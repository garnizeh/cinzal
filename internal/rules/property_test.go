package rules

import (
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is issue #79's property invariant suite: RFC §16.2's seven
// GDD-grounded state invariants (the determinism/refold/RNG-accounting rows
// of that same list are #78's suite, out of scope here), driven by
// randomised orders across randomised seeds and player counts, asserted
// after every round a match plays.
//
// scripts/check-rules-purity.sh explicitly excludes test files from its
// import ban — "testing itself imports os, time and flag. A test using it
// does not make Resolve impure" — so math/rand/v2 is deliberately used
// below rather than repurposing the match's own seeded *RNG: order
// generation stays on a completely separate stream, so it can never
// perturb the RNG consumption count #78's suite already guards.

// --- order generation ---

// randomPropertyOrder builds a bounded, mostly-legal order for seat against
// s's live state: a random walk along real edges within this round's step
// allowance, occasionally an action at the ending node, occasionally an
// Aggressive stance with a small stake, occasionally Pushing On when the
// route ends Hidden. It does not need to be exhaustively Legal-proof —
// validate() (validate.go) already falls back safely to the absence
// default on anything illegal — it only needs to explore real reachable
// states: movement, cargo, stakes, confrontations, deliveries.
func randomPropertyOrder(s MatchState, seat game.SeatID, cfg game.Config, gen *mathrand.Rand) game.Order {
	v := Project(s, seat)
	maxSteps := Steps(v, cfg)

	route := make([]game.NodeID, 0, maxSteps)
	current := v.You.Position
	for range maxSteps {
		view, known := v.Nodes[current]
		if !known || len(view.Edges) == 0 {
			break
		}
		next := view.Edges[gen.IntN(len(view.Edges))]
		route = append(route, next)
		current = next
		if _, stillKnown := v.Nodes[current]; !stillKnown {
			break // arrived on a Hidden node — must be the route's last entry
		}
	}

	o := game.Order{Round: game.RoundNumber(int(s.Round) + 1), Route: route}

	switch gen.IntN(4) {
	case 0:
		o.Stance = game.StanceOrder{Stance: game.StanceAggressive, Stake: gen.IntN(7)}
	case 1:
		o.Stance = game.StanceOrder{Stance: game.StanceEvasive}
	default:
		o.Stance = game.StanceOrder{Stance: game.StanceNeutral}
	}

	if _, endsHidden := v.Nodes[current]; !endsHidden {
		if gen.IntN(2) == 0 {
			o.PushingOn = game.PushingOn{Steps: gen.IntN(3)}
		}
	} else if gen.IntN(3) == 0 {
		if nv, known := v.Nodes[current]; known {
			switch nv.Type {
			case game.NodeBorder:
				o.Action = game.ActionOrder{Kind: game.ActionDeliver}
			case game.NodeBlackMarket:
				o.Action = game.ActionOrder{Kind: game.ActionDeal}
			case game.NodeWarehouse:
				o.Action = game.ActionOrder{Kind: game.ActionPickup}
			default:
				o.Action = game.ActionOrder{Kind: game.ActionStakePost}
			}
		}
	}

	return o
}

// --- invariants (RFC §16.2) ---

// checkPropertyInvariants asserts RFC §16.2's state invariants against s, a
// state just returned by Resolve. cargo <= 1 per player is not checked here
// at all — it is structural (game.Player.Cargo is a single pointer, not a
// slice; see TestPropertyCargoCapIsStructural) — and credit conservation is
// a static source-scan (TestPropertyCreditConservationSourcesAreExhaustive),
// not a per-round runtime check, since no game.Event carries a Cr$ amount
// to reconcile against live here.
func checkPropertyInvariants(s MatchState, cfg game.Config) error {
	postCap, hasCap := cfg.PostCapByPlayers[len(s.Players)]

	for _, p := range s.Players {
		if p.Balance < 0 {
			return fmt.Errorf("seat %d: balance = %d, want >= 0 (GDD §13)", p.Seat, p.Balance)
		}

		if int(p.Position) < 0 || int(p.Position) >= len(s.Graph.Nodes) {
			return fmt.Errorf("seat %d: position = %d, out of range [0,%d) — not a valid node", p.Seat, p.Position, len(s.Graph.Nodes))
		}

		if int(p.Position) >= len(p.Fog) {
			return fmt.Errorf("seat %d: Fog has %d entries, want at least %d to cover its own live Position %d — a Fog too short to even name the seat's own node is itself a malformed state, not a pass", p.Seat, len(p.Fog), int(p.Position)+1, p.Position)
		}
		if p.Fog[p.Position] < game.FogKnown {
			return fmt.Errorf("seat %d: standing on node %d at fog tier %s, want >= Known — GDD §7.2: \"a visited node becomes Known permanently\", so no seat can ever end a round on a node it could not reach", p.Seat, p.Position, p.Fog[p.Position])
		}

		if hasCap && len(p.Posts) > postCap {
			return fmt.Errorf("seat %d: holds %d posts, want <= cap %d (GDD §10.3)", p.Seat, len(p.Posts), postCap)
		}
	}

	return nil
}

// TestPropertyCargoCapIsStructural is RFC §16.2's "cargo <= 1 per player"
// row: game.Player.Cargo (state.go) is a single *game.CarriedCargo, not a
// slice, so holding two pieces of cargo at once is not a rule the engine
// enforces at runtime — it is a shape the type does not have. This pins
// that the field stays that way rather than silently widening into a
// slice, which would turn a structural guarantee into an unchecked one.
func TestPropertyCargoCapIsStructural(t *testing.T) {
	var p Player
	if p.Cargo != nil {
		t.Fatal("Player{}.Cargo is not nil — the zero value must mean \"carrying nothing\"")
	}

	field, ok := reflect.TypeFor[Player]().FieldByName("Cargo")
	if !ok {
		t.Fatal(`Player has no "Cargo" field`)
	}
	if field.Type.Kind() != reflect.Pointer {
		t.Fatalf("Player.Cargo is a %s, want a pointer — \"cargo <= 1 per player\" depends on this being structurally impossible to violate, not runtime-checked", field.Type.Kind())
	}
}

// TestCheckPropertyInvariantsCatchesAViolation is the property suite's own
// self-check: a hand-built state that violates each invariant in turn must
// be reported, not silently accepted — a property check nobody can prove
// fires is a check that could vacuously always pass.
func TestCheckPropertyInvariantsCatchesAViolation(t *testing.T) {
	cfg := legalTestConfig()
	base := MatchState{
		Graph:   Graph{Nodes: []Node{{ID: 0}, {ID: 1}}},
		Players: []Player{{Seat: 0, Fog: []game.FogState{game.FogKnown, game.FogKnown}}},
	}
	if err := checkPropertyInvariants(base, cfg); err != nil {
		t.Fatalf("checkPropertyInvariants(well-formed base) = %v, want nil", err)
	}

	t.Run("negative balance", func(t *testing.T) {
		s := base
		s.Players = []Player{{Seat: 0, Balance: -1, Fog: base.Players[0].Fog}}
		if err := checkPropertyInvariants(s, cfg); err == nil {
			t.Error("checkPropertyInvariants(negative balance) = nil, want an error")
		}
	})

	t.Run("position out of range", func(t *testing.T) {
		s := base
		s.Players = []Player{{Seat: 0, Position: 99, Fog: base.Players[0].Fog}}
		if err := checkPropertyInvariants(s, cfg); err == nil {
			t.Error("checkPropertyInvariants(position out of range) = nil, want an error")
		}
	})

	t.Run("standing on an unreachable Hidden node", func(t *testing.T) {
		s := base
		s.Players = []Player{{Seat: 0, Position: 1, Fog: []game.FogState{game.FogKnown, game.FogHidden}}}
		if err := checkPropertyInvariants(s, cfg); err == nil {
			t.Error("checkPropertyInvariants(standing on a Hidden node) = nil, want an error")
		}
	})

	t.Run("posts over the cap", func(t *testing.T) {
		s := base
		// PostCapByPlayers has no entry for a 1-player match (only
		// 2-5) — a second player is needed so hasCap is actually true.
		postCap := cfg.PostCapByPlayers[2]
		posts := make([]game.NodeID, postCap+1)
		s.Players = []Player{
			{Seat: 0, Posts: posts, Fog: base.Players[0].Fog},
			{Seat: 1, Fog: base.Players[0].Fog},
		}
		if err := checkPropertyInvariants(s, cfg); err == nil {
			t.Error("checkPropertyInvariants(posts over cap) = nil, want an error")
		}
	})
}

// --- credit conservation (RFC §16.2: "sum(credits in play) changes only
// via defined sources") ---

// creditMutatingFiles is the data the acceptance criterion asks for: every
// non-test file in this package permitted to mutate a Player's Balance,
// with the GDD-cited category it belongs to — deliveries, boons, stakes
// won, shakedowns collected (issue #79's source list) and leases, items,
// stakes, gate fees, penalties, Payroll Day, police (GDD §10.2's sink
// list, "clean and one-directional"). Built from an exhaustive grep of
// every `.Balance` mutation site in the package at the time this was
// written — see each entry's comment for what it actually is.
var creditMutatingFiles = map[string]string{
	// Sources
	"deliveries.go": "deliveries — GDD §10.2: \"money comes in from contracts\"",
	"confront.go":   "stakes won and shakedowns collected (winner side) / stakes and shakedowns paid (loser side) — GDD §15",
	"movement.go":   "boons — Scavenging's Cr$3 find on a Hidden-node roll of 4-5 (GDD §9.1)",
	"actions.go":    "boons (Shipping Boom's +Cr$5) and items (Deal's purchase cost) — GDD §14.2, §12",
	"events.go":     "boons (Fence's Windfall, New Boss, Bounty) and penalties (Currency Slide's -25%) — GDD §14.2",

	// Sinks
	"addons.go":    "leases — the Ledger's purchase cost, GDD §5.1, §9.5",
	"debt.go":      "penalties, gate fees, and lease costs that could not be paid in full — GDD §13's Debt cascade, the shared path for every Debt-eligible payment",
	"incidents.go": "items — Open Doors' pre-declared purchase, GDD §14.3, D14 §4",
}

// TestPropertyCreditConservationSourcesAreExhaustive is RFC §16.2's credit-
// conservation invariant, checked structurally rather than at runtime:
// game.Event carries no Cr$ amount on any kind (event.go), so there is no
// way to reconcile a per-round delta against the event stream the way the
// other invariants read live state. What can be checked, and is the
// meaningful half of "changes only via defined sources": that
// creditMutatingFiles above is the *complete* set of files that ever touch
// a Player's Balance. A scan in the same fail-closed, file-level style
// TestOnlyOrderingFileUsesSortSlice (ordering_test.go) already uses in this
// package — any file outside this list that starts mutating Balance is
// exactly the silent, undocumented ninth source/sink this invariant exists
// to catch.
func TestPropertyCreditConservationSourcesAreExhaustive(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}

	balanceMutation := regexp.MustCompile(`\.Balance\s*(?:\+\+|--|[+\-]?=[^=])`)

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}

		if !balanceMutation.Match(data) {
			continue
		}
		if _, allowed := creditMutatingFiles[name]; !allowed {
			t.Errorf("%s mutates a Player's Balance but is not in creditMutatingFiles — either it belongs to one of GDD §10.2's named sources/sinks (add it, with a citation) or it is a genuine new source of money materialising or vanishing outside the defined list", name)
		}
	}

	if scanned == 0 {
		t.Fatal("scanned 0 non-test .go files in internal/rules — the check ran over nothing, which is not the same as passing")
	}

	// The reverse direction: every entry in the allowlist must still be
	// real, or the data has drifted from the code without anyone noticing
	// (e.g. a file rename, or Balance mutations moving out of a file
	// entirely).
	for file := range creditMutatingFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("creditMutatingFiles names %s, but it could not be read: %v", file, err)
			continue
		}
		if !balanceMutation.Match(data) {
			t.Errorf("creditMutatingFiles names %s as a Balance mutator, but it no longer contains one — this entry is stale", file)
		}
	}
}

// --- the randomised sweep ---

// propertySweepWeight is one player count's share of the sweep, weighted
// inversely to NewMatch's own cost: procedural map generation (RFC §6.1's
// rejection-and-retry loop) gets substantially more expensive as player
// count rises — measured at roughly 2ms, 6ms, 40ms and 325ms per full
// match construction at 2, 3, 4 and 5 players respectively — so an equal
// split would spend nearly all its wall-clock budget on 5-player matches
// alone. The four counts total exactly 1000, issue #79's own acceptance
// floor ("≥ 1000 randomised matches at 2/3/4/5 players"), and every count
// still gets a real, non-trivial sample.
type propertySweepWeight struct {
	players    int
	iterations int
}

var propertySweepWeights = []propertySweepWeight{
	{players: 2, iterations: 500},
	{players: 3, iterations: 300},
	{players: 4, iterations: 150},
	{players: 5, iterations: 50},
}

// TestPropertyInvariantsHoldAcrossRandomisedMatches is issue #79's
// headline acceptance criterion: every RFC §16.2 state invariant, asserted
// after every round, over >= 1000 randomised matches at 2/3/4/5 players.
// Each player count's sweep runs as its own parallel subtest so the
// 5-player group's much higher per-match cost does not serialise behind
// the cheaper ones.
func TestPropertyInvariantsHoldAcrossRandomisedMatches(t *testing.T) {
	cfg := game.DefaultConfig()

	for _, w := range propertySweepWeights {
		t.Run(fmt.Sprintf("%dp", w.players), func(t *testing.T) {
			t.Parallel()

			for i := range w.iterations {
				matchSeed := seedFromInt(w.players*1_000_000 + i)
				gen := mathrand.New(mathrand.NewPCG(uint64(w.players), uint64(i)))

				s, err := NewMatch(matchSeed, cfg, w.players)
				if err != nil {
					t.Fatalf("NewMatch(seed=%x, players=%d) error = %v", matchSeed, w.players, err)
				}

				var orderLog []string

				for round := 1; round <= cfg.Rounds; round++ {
					orders := make(map[game.SeatID]game.Order, w.players)
					for seat := range w.players {
						orders[game.SeatID(seat)] = randomPropertyOrder(s, game.SeatID(seat), cfg, gen)
					}
					orderLog = append(orderLog, fmt.Sprintf("round %d: %+v", round, orders))

					next, _, err := Resolve(s, orders, cfg, NewRNG(matchSeed, round))
					if err != nil {
						t.Fatalf("seed=%x players=%d round=%d: Resolve() error = %v", matchSeed, w.players, round, err)
					}

					if err := checkPropertyInvariants(next, cfg); err != nil {
						t.Fatalf("seed=%x players=%d round=%d: invariant violated: %v\n\nreproduce with this exact order log:\n%s",
							matchSeed, w.players, round, err, strings.Join(orderLog, "\n"))
					}

					s = next
				}
			}
		})
	}
}
