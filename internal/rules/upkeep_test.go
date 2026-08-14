package rules

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// upkeepFixture builds a minimal MatchState for upkeep's own tests: round,
// a Graph with numNodes empty nodes (ID == index, no edges — upkeep never
// walks the graph's topology), and the given players.
func upkeepFixture(round game.RoundNumber, numNodes int, players ...Player) MatchState {
	nodes := make([]Node, numNodes)
	for i := range nodes {
		nodes[i].ID = game.NodeID(i)
	}
	return MatchState{
		Round:   round,
		Graph:   Graph{Nodes: nodes},
		Players: players,
	}
}

// --- Step 1: contract deadlines ---------------------------------------

// TestUpkeepContractExpiryAffordablePenaltyNeverTriggersDebt: a seat with
// enough balance to cover the tier's flat penalty pays it in full, the
// contract discards, and nothing past GDD §8.3 fires — no Debt, no Flagged.
func TestUpkeepContractExpiryAffordablePenaltyNeverTriggersDebt(t *testing.T) {
	cfg := legalTestConfig() // DefaultConfig: Tier I penalty Cr$3
	s := upkeepFixture(4, 1, Player{
		Seat:      0,
		Balance:   10,
		Contracts: []Contract{{ID: 0, Tier: 0, ExpiresRound: 4}},
	})

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if got, want := s.Players[0].Balance, 7; got != want {
		t.Errorf("Balance = %d, want %d", got, want)
	}
	if len(s.Players[0].Contracts) != 0 {
		t.Errorf("Contracts = %+v, want empty (discarded)", s.Players[0].Contracts)
	}
	if s.Players[0].Flagged {
		t.Error("Flagged = true, want false — the penalty was fully affordable")
	}
	if events != nil {
		t.Errorf("events = %+v, want nil — an affordable penalty has no public consequence", events)
	}
}

// TestUpkeepContractExpiryUnaffordablePenaltySurrendersFewestRoundsRemainingLease
// is D5's own worked example, numbers and all: seat holds two posts, 1 and 5
// rounds remaining, and a contract deadline it cannot cover in cash expires
// the same round. Step 1's Debt cascade must run — and read "fewest rounds
// remaining" — before step 2's own lease decrement ever touches either
// post, or the wrong post (or both) is lost. Correct behaviour: exactly the
// 1-round post is surrendered; the 5-round post survives, decremented to 4
// by step 2 in the same Upkeep call.
func TestUpkeepContractExpiryUnaffordablePenaltySurrendersFewestRoundsRemainingLease(t *testing.T) {
	cfg := legalTestConfig() // Tier I penalty Cr$3
	s := upkeepFixture(9, 2, Player{
		Seat:      0,
		Balance:   0,
		Infamy:    5,
		Posts:     []game.NodeID{0, 1},
		Contracts: []Contract{{ID: 0, Tier: 0, ExpiresRound: 9}},
	})
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 1}
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 5}

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if s.Graph.Nodes[0].Post != nil {
		t.Error("Graph.Nodes[0].Post != nil, want surrendered — it had fewest rounds remaining (1)")
	}
	if s.Graph.Nodes[1].Post == nil || s.Graph.Nodes[1].Post.RoundsRemaining != 4 {
		t.Errorf("Graph.Nodes[1].Post = %+v, want RoundsRemaining 4 (5, decremented once, untaken by Debt)", s.Graph.Nodes[1].Post)
	}
	if want := []game.NodeID{1}; !reflect.DeepEqual(s.Players[0].Posts, want) {
		t.Errorf("Players[0].Posts = %v, want %v", s.Players[0].Posts, want)
	}
	if !s.Players[0].Flagged {
		t.Error("Flagged = false, want true — the penalty triggered Debt")
	}
	if got, want := s.Players[0].Infamy, 4; got != want {
		t.Errorf("Infamy = %d, want %d (5, -1 from Debt)", got, want)
	}
	want := []game.Event{{Kind: game.EventLeaseExpired, Round: 9, Node: 0, Seat: 0}}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events = %+v, want %+v — exactly one lease lost, not two", events, want)
	}
}

// TestUpkeepContractExpiryDropsItsOwnCarriedCargo: cargo bound to the
// expiring contract is gone entirely (GDD §8.4: "any cargo you were
// carrying for it is gone") — not left on the ground the way a
// confrontation loss works.
func TestUpkeepContractExpiryDropsItsOwnCarriedCargo(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 1, Player{
		Seat:      0,
		Balance:   10,
		Cargo:     &game.CarriedCargo{Bound: true, Contract: 0},
		Contracts: []Contract{{ID: 0, Tier: 0, ExpiresRound: 4}},
	})

	upkeep(&s, cfg, NextRoundModifiers{})

	if s.Players[0].Cargo != nil {
		t.Errorf("Cargo = %+v, want nil", s.Players[0].Cargo)
	}
	if len(s.Graph.Cargo) != 0 {
		t.Errorf("Graph.Cargo = %+v, want empty — a missed deadline destroys the cargo, it doesn't fall at the node", s.Graph.Cargo)
	}
}

// TestUpkeepContractExpiryLeavesOtherContractAndCargoUntouched: a seat's
// second, still-active contract and the cargo it's carrying for that
// (different) contract must survive one slot-mate's own expiry unharmed —
// D5's "each runs the full sequence independently."
func TestUpkeepContractExpiryLeavesOtherContractAndCargoUntouched(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 1, Player{
		Seat:    0,
		Balance: 10,
		Cargo:   &game.CarriedCargo{Bound: true, Contract: 1},
		Contracts: []Contract{
			{ID: 0, Tier: 0, ExpiresRound: 4}, // expires this round
			{ID: 1, Tier: 0, ExpiresRound: 8}, // not yet due
		},
	})

	upkeep(&s, cfg, NextRoundModifiers{})

	if len(s.Players[0].Contracts) != 1 || s.Players[0].Contracts[0].ID != 1 {
		t.Errorf("Contracts = %+v, want only ID 1 to survive", s.Players[0].Contracts)
	}
	if s.Players[0].Cargo == nil || s.Players[0].Cargo.Contract != 1 {
		t.Errorf("Cargo = %+v, want untouched (bound to the surviving contract)", s.Players[0].Cargo)
	}
}

// TestUpkeepContractNotYetDueSurvivesUntouched: ExpiresRound in the future
// means no penalty, no discard, no balance change.
func TestUpkeepContractNotYetDueSurvivesUntouched(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 0, Player{
		Seat:      0,
		Balance:   10,
		Contracts: []Contract{{ID: 0, Tier: 0, ExpiresRound: 5}},
	})

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if len(s.Players[0].Contracts) != 1 {
		t.Errorf("Contracts = %+v, want the one not-yet-due contract untouched", s.Players[0].Contracts)
	}
	if s.Players[0].Balance != 10 {
		t.Errorf("Balance = %d, want unchanged 10", s.Players[0].Balance)
	}
	if events != nil {
		t.Errorf("events = %+v, want nil", events)
	}
}

// --- Step 2: leases ------------------------------------------------------

// TestUpkeepLeaseExpiresAtZeroEmitsPublicEvent: GDD §10.4's "The corner
// went quiet" — the lease-expired anchor (RFC §9.1 row 4), named.
func TestUpkeepLeaseExpiresAtZeroEmitsPublicEvent(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(6, 1, Player{Seat: 0, Posts: []game.NodeID{0}})
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 1}

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if s.Graph.Nodes[0].Post != nil {
		t.Error("Graph.Nodes[0].Post != nil, want expired")
	}
	if len(s.Players[0].Posts) != 0 {
		t.Errorf("Players[0].Posts = %v, want empty", s.Players[0].Posts)
	}
	want := []game.Event{{Kind: game.EventLeaseExpired, Round: 6, Node: 0, Seat: 0}}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events = %+v, want %+v", events, want)
	}
}

// TestUpkeepLeaseNotYetAtZeroSurvivesSilently: an ordinary decrement, no
// event, still held.
func TestUpkeepLeaseNotYetAtZeroSurvivesSilently(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(6, 1, Player{Seat: 0, Posts: []game.NodeID{0}})
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 3}

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if s.Graph.Nodes[0].Post == nil || s.Graph.Nodes[0].Post.RoundsRemaining != 2 {
		t.Errorf("Post = %+v, want RoundsRemaining 2", s.Graph.Nodes[0].Post)
	}
	if events != nil {
		t.Errorf("events = %+v, want nil", events)
	}
}

// TestUpkeepLeaseExpiredBelowZeroFromTorchedStillFires: Torched (GDD
// §14.3, resolveTorched) can push RoundsRemaining below 1 in the same round
// without closing the lease itself — Upkeep step 2 is "the sole place a
// lease transitions to expired" (incidents.go), so it must still fire on a
// lease that Torched already dropped to zero or negative this round, not
// silently skip past it.
func TestUpkeepLeaseExpiredBelowZeroFromTorchedStillFires(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(6, 1, Player{Seat: 0, Posts: []game.NodeID{0}})
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: -1} // Torched: 2 - 3

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if s.Graph.Nodes[0].Post != nil {
		t.Error("Graph.Nodes[0].Post != nil, want expired even though it started this step already below zero")
	}
	want := []game.Event{{Kind: game.EventLeaseExpired, Round: 6, Node: 0, Seat: 0}}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events = %+v, want %+v", events, want)
	}
}

// TestUpkeepLeaseRemovalIsCauseBlindOnTheWire: a lease that expired on its
// own and one surrendered to Debt must produce byte-identical Events (D5,
// RFC §9.1 row 4) — a distinguishable trace would newly disclose that a
// player is in debt, which GDD §5 keeps private.
func TestUpkeepLeaseRemovalIsCauseBlindOnTheWire(t *testing.T) {
	cfg := legalTestConfig()

	natural := upkeepFixture(10, 6, Player{Seat: 0, Posts: []game.NodeID{5}})
	natural.Graph.Nodes[5].Post = &Post{Owner: 0, RoundsRemaining: 1}
	naturalEvents := upkeep(&natural, cfg, NextRoundModifiers{})

	surrendered := upkeepFixture(10, 6, Player{
		Seat:      0,
		Balance:   0,
		Posts:     []game.NodeID{5},
		Contracts: []Contract{{ID: 0, Tier: 0, ExpiresRound: 10}},
	})
	surrendered.Graph.Nodes[5].Post = &Post{Owner: 0, RoundsRemaining: 3}
	surrenderedEvents := upkeep(&surrendered, cfg, NextRoundModifiers{})

	if len(naturalEvents) != 1 || len(surrenderedEvents) != 1 {
		t.Fatalf("want exactly one lease-expiry event from each scenario, got %+v and %+v", naturalEvents, surrenderedEvents)
	}
	if !reflect.DeepEqual(naturalEvents[0], surrenderedEvents[0]) {
		t.Errorf("natural expiry event %+v != debt-surrender event %+v, want identical", naturalEvents[0], surrenderedEvents[0])
	}
}

// --- Step 3: Sinkhole ------------------------------------------------------

// TestUpkeepSinkholeDecrementsSilently: an active Sinkhole ticks down with
// no announcement at any point, zero included (GDD "Upkeep": "it's read
// off the map like any other passable node").
func TestUpkeepSinkholeDecrementsSilently(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(5, 1)
	s.Graph.Nodes[0].SinkholeRounds = 3

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if got := s.Graph.Nodes[0].SinkholeRounds; got != 2 {
		t.Errorf("SinkholeRounds = %d, want 2", got)
	}
	if events != nil {
		t.Errorf("events = %+v, want nil", events)
	}
}

// TestUpkeepSinkholeClearsAtZeroAndStaysThere: the node is passable again,
// and an already-passable node is never decremented negative.
func TestUpkeepSinkholeClearsAtZeroAndStaysThere(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(5, 2)
	s.Graph.Nodes[0].SinkholeRounds = 1
	s.Graph.Nodes[1].SinkholeRounds = 0

	upkeep(&s, cfg, NextRoundModifiers{})

	if got := s.Graph.Nodes[0].SinkholeRounds; got != 0 {
		t.Errorf("Nodes[0].SinkholeRounds = %d, want 0", got)
	}
	if got := s.Graph.Nodes[1].SinkholeRounds; got != 0 {
		t.Errorf("Nodes[1].SinkholeRounds = %d, want still 0, not negative", got)
	}
}

// --- Step 4: next-round modifiers -----------------------------------------

// TestUpkeepNextRoundModifierClearedOnlyWhenActiveEntering is D5's fix for
// the hazard state.go's own NextRound comment names: a modifier already
// active entering the round (entryNextRound) is consumed and cleared;
// one this round's own globalEvent() just set fresh — absent from
// entryNextRound — must survive untouched, or round N+1 never gets to read
// it.
func TestUpkeepNextRoundModifierClearedOnlyWhenActiveEntering(t *testing.T) {
	sectorA := game.SectorOldDocks
	sectorB := game.SectorIronLow

	cases := []struct {
		name  string
		entry NextRoundModifiers
		live  NextRoundModifiers
		want  NextRoundModifiers
	}{
		{
			"Retainer active entering -> cleared",
			NextRoundModifiers{Retainer: true},
			NextRoundModifiers{Retainer: true},
			NextRoundModifiers{Retainer: false},
		},
		{
			"Retainer freshly set this round -> survives",
			NextRoundModifiers{},
			NextRoundModifiers{Retainer: true},
			NextRoundModifiers{Retainer: true},
		},
		{
			"Blackout active entering -> cleared",
			NextRoundModifiers{Blackout: true},
			NextRoundModifiers{Blackout: true},
			NextRoundModifiers{Blackout: false},
		},
		{
			"Blackout freshly set this round -> survives",
			NextRoundModifiers{},
			NextRoundModifiers{Blackout: true},
			NextRoundModifiers{Blackout: true},
		},
		{
			"DockersStrike active entering -> cleared",
			NextRoundModifiers{DockersStrike: true},
			NextRoundModifiers{DockersStrike: true},
			NextRoundModifiers{DockersStrike: false},
		},
		{
			"DockersStrike freshly set this round -> survives",
			NextRoundModifiers{},
			NextRoundModifiers{DockersStrike: true},
			NextRoundModifiers{DockersStrike: true},
		},
		{
			"Scaffolding active entering -> cleared",
			NextRoundModifiers{Scaffolding: &sectorA},
			NextRoundModifiers{Scaffolding: &sectorA},
			NextRoundModifiers{Scaffolding: nil},
		},
		{
			"Scaffolding freshly set this round -> survives",
			NextRoundModifiers{},
			NextRoundModifiers{Scaffolding: &sectorB},
			NextRoundModifiers{Scaffolding: &sectorB},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &MatchState{NextRound: c.live}
			upkeepNextRoundModifiers(s, c.entry)

			got := s.NextRound
			gotScaffolding, wantScaffolding := got.Scaffolding, c.want.Scaffolding
			got.Scaffolding, c.want.Scaffolding = nil, nil
			if got != c.want {
				t.Errorf("NextRound = %+v, want %+v", got, c.want)
			}
			switch {
			case wantScaffolding == nil:
				if gotScaffolding != nil {
					t.Errorf("Scaffolding = %v, want nil", *gotScaffolding)
				}
			case gotScaffolding == nil || *gotScaffolding != *wantScaffolding:
				t.Errorf("Scaffolding = %v, want %v", gotScaffolding, *wantScaffolding)
			}
		})
	}
}

// --- Round 15 -----------------------------------------------------------

// TestResolveRunsUpkeepOnRoundFifteenLikeAnyOtherRound: the GDD §4 phase
// diagram draws no truncated final round, so a lease at 1 round remaining
// entering round 15 must expire during round 15's own Upkeep, before final
// scoring (GDD §16) ever reads the resulting state — D5's own settled
// answer to "does the last round run Upkeep at all."
func TestResolveRunsUpkeepOnRoundFifteenLikeAnyOtherRound(t *testing.T) {
	s := resolveTestState()
	s.Round = 14 // Resolve increments to 15
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 1}
	s.Players[0].Posts = []game.NodeID{0}
	cfg := legalTestConfig()

	next, events, err := Resolve(s, nil, cfg, NewRNG(testSeed(5), 15))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if next.Round != 15 {
		t.Fatalf("next.Round = %d, want 15", next.Round)
	}
	if next.Graph.Nodes[0].Post != nil {
		t.Error("Graph.Nodes[0].Post != nil, want expired during round 15's own Upkeep")
	}
	if len(next.Players[0].Posts) != 0 {
		t.Errorf("Players[0].Posts = %v, want empty", next.Players[0].Posts)
	}

	found := false
	for _, e := range events {
		if e.Kind == game.EventLeaseExpired && e.Round == 15 && e.Node == 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %+v, want a round-15 EventLeaseExpired for node 0", events)
	}
}

// --- Negative guards: what Upkeep must never touch --------------------

// TestUpkeepNeverClearsFlaggedOrEvasiveStepPenalty guards against either
// counter migrating back into Upkeep in a future refactor (D5 Consequences)
// — both are consumed and reset at the top of Resolve (resetRoundFlags),
// never here, because only that point can tell "already used" apart from
// "just set for next round" (D5's own worked example).
func TestUpkeepNeverClearsFlaggedOrEvasiveStepPenalty(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 0, Player{Seat: 0, Flagged: true, EvasiveStepPenalty: true})

	upkeep(&s, cfg, NextRoundModifiers{})

	if !s.Players[0].Flagged {
		t.Error("Flagged = false, want still true — Upkeep must never clear it")
	}
	if !s.Players[0].EvasiveStepPenalty {
		t.Error("EvasiveStepPenalty = false, want still true — Upkeep must never clear it")
	}
}

// TestUpkeepNeverMutatesLastOfferRound: the Contact Cooldown needs no
// per-round action at all (GDD "Upkeep") — LastOfferRound is written once,
// at offer/decline, and read as a difference against the current round.
func TestUpkeepNeverMutatesLastOfferRound(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 0, Player{Seat: 0, LastOfferRound: 7})

	upkeep(&s, cfg, NextRoundModifiers{})

	if got := s.Players[0].LastOfferRound; got != 7 {
		t.Errorf("LastOfferRound = %d, want unchanged 7", got)
	}
}

// TestUpkeepNeverMutatesLooseCrateHeldRounds: the heat tick fires inside
// writeTrail, earlier in the same pipeline (GDD "Upkeep") — Upkeep must
// never touch it.
func TestUpkeepNeverMutatesLooseCrateHeldRounds(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 0, Player{Seat: 0, LooseCrateHeldRounds: 3})

	upkeep(&s, cfg, NextRoundModifiers{})

	if got := s.Players[0].LooseCrateHeldRounds; got != 3 {
		t.Errorf("LooseCrateHeldRounds = %d, want unchanged 3", got)
	}
}

// TestUpkeepEmptyStateIsANoOp: D11's own guarantee that no suppression
// branch is needed — every step iterates state that only exists if the
// owning subsystem is enabled, so a match with none of it produces no
// events and panics on nothing.
func TestUpkeepEmptyStateIsANoOp(t *testing.T) {
	cfg := legalTestConfig()
	s := upkeepFixture(4, 3, Player{Seat: 0}, Player{Seat: 1})

	events := upkeep(&s, cfg, NextRoundModifiers{})

	if events != nil {
		t.Errorf("events = %+v, want nil", events)
	}
}
