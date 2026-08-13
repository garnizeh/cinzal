package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// confrontTestGraph builds a fixed graph covering every pushback shape this
// file's tests need:
//
//	0 - 1 - 2 - 3 - 4        a line, for "traversed enough" / "traversed 1,
//	                         pushed 2" pushback
//	5 - 6, 5 - 7, 5 - 8      a hub (node 5) with three leaf neighbors (each
//	                         connects only back to 5), for the stationary
//	                         random-hop cases where a second hop is always
//	                         boxed in
//	9 - 10                   a boxed pair: 9's only neighbor is 10, and
//	                         10's only neighbor is 9 — an Evasive stationary
//	                         loser's second hop has nowhere to go but back
//	11                       isolated — no edges at all, for a stationary
//	                         loser's *first* hop having nowhere to go
//	12 - 13, 12 Alley        for the ambush bonus
//	14 - 15, 16, 17          a second hub whose leaves each have a further
//	                         escape (18, 19, 20) besides the hub itself, for
//	                         a stationary Evasive loser's genuine second hop
//	15 - 18, 16 - 19, 17 - 20
func confrontTestGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1}},
		{ID: 1, Edges: []game.NodeID{0, 2}},
		{ID: 2, Edges: []game.NodeID{1, 3}},
		{ID: 3, Edges: []game.NodeID{2, 4}},
		{ID: 4, Edges: []game.NodeID{3}},
		{ID: 5, Edges: []game.NodeID{6, 7, 8}},
		{ID: 6, Edges: []game.NodeID{5}},
		{ID: 7, Edges: []game.NodeID{5}},
		{ID: 8, Edges: []game.NodeID{5}},
		{ID: 9, Edges: []game.NodeID{10}},
		{ID: 10, Edges: []game.NodeID{9}},
		{ID: 11, Edges: nil},
		{ID: 12, Type: game.NodeAlley, Edges: []game.NodeID{13}},
		{ID: 13, Edges: []game.NodeID{12}},
		{ID: 14, Edges: []game.NodeID{15, 16, 17}},
		{ID: 15, Edges: []game.NodeID{14, 18}},
		{ID: 16, Edges: []game.NodeID{14, 19}},
		{ID: 17, Edges: []game.NodeID{14, 20}},
		{ID: 18, Edges: []game.NodeID{15}},
		{ID: 19, Edges: []game.NodeID{16}},
		{ID: 20, Edges: []game.NodeID{17}},
	}}
}

// confrontTestState returns a MatchState over confrontTestGraph with n
// seats, positioned at start (seat i at node start[i]), Balance 20,
// otherwise zero — a baseline every test overrides from.
func confrontTestState(start ...game.NodeID) MatchState {
	players := make([]Player, len(start))
	for i, pos := range start {
		players[i] = Player{Seat: game.SeatID(i), Balance: 20, Position: pos}
	}
	return MatchState{Round: 5, Graph: confrontTestGraph(), Players: players}
}

// confrontWalks builds walks for seats, each seeded with the given Path.
func confrontWalks(paths map[game.SeatID][]game.NodeID) map[game.SeatID]*seatWalk {
	walks := make(map[game.SeatID]*seatWalk, len(paths))
	for seat, path := range paths {
		prev := path[0]
		if len(path) > 1 {
			prev = path[len(path)-2]
		}
		walks[seat] = &seatWalk{Previous: prev, Path: append([]game.NodeID(nil), path...)}
	}
	return walks
}

// --- pure helpers ---

func TestStanceModifier(t *testing.T) {
	cases := []struct {
		stance game.Stance
		want   int
	}{
		{game.StanceAggressive, 1},
		{game.StanceNeutral, 0},
		{game.StanceEvasive, -1},
	}
	for _, c := range cases {
		if got := stanceModifier(c.stance); got != c.want {
			t.Errorf("stanceModifier(%s) = %d, want %d", c.stance, got, c.want)
		}
	}
}

func TestHasShivDiscard(t *testing.T) {
	yes := game.Order{Items: []game.ItemDiscard{{Item: game.ItemPoliceBand}, {Item: game.ItemShiv}}}
	no := game.Order{Items: []game.ItemDiscard{{Item: game.ItemPoliceBand}}}

	if !hasShivDiscard(yes) {
		t.Error("hasShivDiscard(declares Shiv) = false, want true")
	}
	if hasShivDiscard(no) {
		t.Error("hasShivDiscard(no Shiv) = true, want false")
	}
}

func TestLowestInfamy(t *testing.T) {
	infamy := map[game.SeatID]int{0: 5, 1: 2, 2: 9}
	if got := lowestInfamy(infamy, []game.SeatID{0, 1, 2}); got != 2 {
		t.Errorf("lowestInfamy() = %d, want 2", got)
	}
}

// TestDetermineOutcomeUniqueWinner is the ordinary two-participant case.
func TestDetermineOutcomeUniqueWinner(t *testing.T) {
	seats := []game.SeatID{0, 1}
	totals := map[game.SeatID]int{0: 9, 1: 7}
	winner, tie := determineOutcome(seats, totals)
	if tie || winner != 0 {
		t.Errorf("determineOutcome() = (%d, %v), want (0, false)", winner, tie)
	}
}

// TestDetermineOutcomeWholeFreeForAllTiesWhenTopIsShared is the acceptance
// criterion behind GDD §15's "on a tie, nobody wins": in a 4-way melee where
// two participants share the highest total and two score lower, the whole
// confrontation ties — "highest total wins" never resolved to a single
// winner, so the two lower scorers are not "losers" either.
func TestDetermineOutcomeWholeFreeForAllTiesWhenTopIsShared(t *testing.T) {
	seats := []game.SeatID{0, 1, 2, 3}
	totals := map[game.SeatID]int{0: 9, 1: 9, 2: 5, 3: 3}
	_, tie := determineOutcome(seats, totals)
	if !tie {
		t.Error("determineOutcome() tie = false, want true when two of four share the top total")
	}
}

func TestLosersExcept(t *testing.T) {
	got := losersExcept([]game.SeatID{0, 1, 2, 3}, 2)
	want := []game.SeatID{0, 1, 3}
	if len(got) != len(want) {
		t.Fatalf("losersExcept() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("losersExcept() = %v, want %v", got, want)
		}
	}
}

func TestNeighborsExcludingFiltersSinkholeAndExcludeList(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1, 2, 3}},
		{ID: 1},
		{ID: 2, SinkholeRounds: 2},
		{ID: 3},
	}}

	got := neighborsExcluding(g, 0, 3)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("neighborsExcluding() = %v, want [1] (2 Sinkholed, 3 excluded)", got)
	}
}

func TestRemoveOneItemRemovesSingleInstance(t *testing.T) {
	items := []game.ItemID{game.ItemMuscle, game.ItemShiv, game.ItemMuscle}
	got := removeOneItem(items, game.ItemMuscle)
	want := []game.ItemID{game.ItemShiv, game.ItemMuscle}
	if len(got) != len(want) {
		t.Fatalf("removeOneItem() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("removeOneItem() = %v, want %v", got, want)
		}
	}
}

func TestFixPathTailCollapsesDuplicate(t *testing.T) {
	walk := &seatWalk{Path: []game.NodeID{0, 1, 2}}
	fixPathTail(walk, 1)
	want := []game.NodeID{0, 1}
	if len(walk.Path) != len(want) || walk.Path[0] != 0 || walk.Path[1] != 1 {
		t.Errorf("fixPathTail() Path = %v, want %v", walk.Path, want)
	}
}

func TestFixPathTailKeepsDistinctEntry(t *testing.T) {
	walk := &seatWalk{Path: []game.NodeID{0, 2}}
	fixPathTail(walk, 1)
	want := []game.NodeID{0, 1}
	if len(walk.Path) != len(want) || walk.Path[0] != 0 || walk.Path[1] != 1 {
		t.Errorf("fixPathTail() Path = %v, want %v", walk.Path, want)
	}
}

func TestHaltMovementClearsRouteAndAction(t *testing.T) {
	validated := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 2}, PushingOn: game.PushingOn{Steps: 2}, Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}
	haltMovement(validated, 0)
	o := validated[0]
	if o.Route != nil || o.PushingOn.Steps != 0 || o.Action.Kind != game.ActionNothing {
		t.Errorf("haltMovement() order = %+v, want Route nil, PushingOn zero, Action Nothing", o)
	}
}

// --- confrontationTotal ---

// TestConfrontationTotalSumsEveryTerm builds a participant with every
// modifier active at once and checks the sum against the D6 roll the same
// seed/round/seq would produce on its own — pinning the formula's
// composition without hardcoding a magic total.
func TestConfrontationTotalSumsEveryTerm(t *testing.T) {
	s := confrontTestState(12)
	s.Players[0].Infamy = 9 // Legend: tier bonus 3
	s.Players[0].Items = []game.ItemID{game.ItemMuscle}
	o := game.Order{
		Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 6}, // +1 stance, +2 stake (6/3, capped)
		Items:  []game.ItemDiscard{{Item: game.ItemShiv}},                 // +3, first confrontation
	}
	walk := &seatWalk{}

	seed := testSeed(9)
	roll := NewRNG(seed, int(s.Round)).Next(PurposeConfrontD6, 6) + 1

	r := NewRNG(seed, int(s.Round))
	// infamy 9 (tier 3), lowest 9 too -> underestimated +1
	got := confrontationTotal(&s, 0, 12, o, 9, 9, walk, r)

	want := roll + 3 /* tier */ + 1 /* stance */ + 1 /* ambush: Aggressive+Alley */ + 3 /* Shiv */ + 1 /* Muscle */ + 2 /* stake cap */ + 1 /* underestimated */
	if got != want {
		t.Errorf("confrontationTotal() = %d, want %d (roll %d + modifiers)", got, want, roll)
	}
	if !walk.ShivFired {
		t.Error("confrontationTotal() left ShivFired false after a declared Shiv fired")
	}
}

// TestConfrontationTotalShivFiresOnlyOnce checks the second call for the
// same seat's walk this round gets no further Shiv bonus, and consumes no
// extra draw for it — ShivFired already gates that at the call site, this
// pins the gate itself.
func TestConfrontationTotalShivFiresOnlyOnce(t *testing.T) {
	s := confrontTestState(0)
	o := game.Order{Items: []game.ItemDiscard{{Item: game.ItemShiv}}}
	walk := &seatWalk{}
	seed := testSeed(1)

	replay := NewRNG(seed, int(s.Round))
	roll1 := replay.Next(PurposeConfrontD6, 6) + 1
	roll2 := replay.Next(PurposeConfrontD6, 6) + 1

	r := NewRNG(seed, int(s.Round))
	first := confrontationTotal(&s, 0, 0, o, 0, 0, walk, r)
	second := confrontationTotal(&s, 0, 0, o, 0, 0, walk, r)

	// infamy 0 (tier bonus 0), Neutral stance, no stake, always "lowest" (+1).
	if want := roll1 + 3 + 1; first != want {
		t.Errorf("first confrontationTotal() = %d, want %d (Shiv fires)", first, want)
	}
	if want := roll2 + 1; second != want {
		t.Errorf("second confrontationTotal() = %d, want %d (Shiv already spent this round)", second, want)
	}
	if !walk.ShivFired {
		t.Fatal("walk.ShivFired = false after first call, want true")
	}
}

// --- pushback table (GDD §15) ---

// TestPushbackTraversedEnoughWalksBackAlongOwnRoute covers the ordinary
// case for both stances: a seat that moved several nodes this round falls
// back 1 (Neutral) or 2 (Evasive) along its own Path, drawing nothing.
func TestPushbackTraversedEnoughWalksBackAlongOwnRoute(t *testing.T) {
	cases := []struct {
		name    string
		evasive bool
		want    game.NodeID
	}{
		{"non-evasive falls back 1", false, 2},
		{"evasive falls back 2", true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := confrontTestState(3)
			walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {0, 1, 2, 3}})
			r := NewRNG(testSeed(1), int(s.Round))

			pushback(&s, confrontation{Node: 3, Seats: []game.SeatID{0}}, 0, c.evasive, walks, r)

			if s.Players[0].Position != c.want {
				t.Errorf("Position = %d, want %d", s.Players[0].Position, c.want)
			}
			if got := r.Consumed(PurposePushbackHop); got != 0 {
				t.Errorf("pushback.hop consumed = %d, want 0 (a mover never draws)", got)
			}
		})
	}
}

// TestPushbackTraversedOneEvasivePushedTwoStopsAtStart is the GDD §15 row:
// "traversed only 1 node, pushed 2: stops at their starting node."
func TestPushbackTraversedOneEvasivePushedTwoStopsAtStart(t *testing.T) {
	s := confrontTestState(1)
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {0, 1}})
	r := NewRNG(testSeed(1), int(s.Round))

	pushback(&s, confrontation{Node: 1, Seats: []game.SeatID{0}}, 0, true, walks, r)

	if s.Players[0].Position != 0 {
		t.Errorf("Position = %d, want 0 (the route is exhausted, not extrapolated)", s.Players[0].Position)
	}
	if got := r.Consumed(PurposePushbackHop); got != 0 {
		t.Errorf("pushback.hop consumed = %d, want 0", got)
	}
}

// TestPushbackStationaryNeutralConsumesOneHop is RFC §6.4's worked case: "a
// stationary Neutral loser consumes 1."
func TestPushbackStationaryNeutralConsumesOneHop(t *testing.T) {
	s := confrontTestState(5)
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {5}})
	r := NewRNG(testSeed(1), int(s.Round))

	pushback(&s, confrontation{Node: 5, Seats: []game.SeatID{0}}, 0, false, walks, r)

	if got := r.Consumed(PurposePushbackHop); got != 1 {
		t.Errorf("pushback.hop consumed = %d, want 1", got)
	}
	neighbors := map[game.NodeID]bool{6: true, 7: true, 8: true}
	if !neighbors[s.Players[0].Position] {
		t.Errorf("Position = %d, want one of 5's neighbors", s.Players[0].Position)
	}
}

// TestPushbackStationaryEvasiveConsumesTwoHops is RFC §6.4's worked case:
// "a stationary Evasive loser consumes 2." The second hop must never land
// back on the confrontation node.
func TestPushbackStationaryEvasiveConsumesTwoHops(t *testing.T) {
	s := confrontTestState(14)
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {14}})
	r := NewRNG(testSeed(1), int(s.Round))

	pushback(&s, confrontation{Node: 14, Seats: []game.SeatID{0}}, 0, true, walks, r)

	if got := r.Consumed(PurposePushbackHop); got != 2 {
		t.Errorf("pushback.hop consumed = %d, want 2", got)
	}
	if s.Players[0].Position == 14 {
		t.Error("Position = 14 (the confrontation node), want anywhere else — re-entry is forbidden")
	}
}

// TestPushbackStationaryEvasiveBoxedInSecondHopStopsEarly uses the 9-10
// boxed pair: hop one is forced onto 10, whose only neighbor is 9 (the
// confrontation node) — excluded — so the second hop has no legal
// destination and must not be drawn at all (RFC §6.4's lazy-draw rule).
func TestPushbackStationaryEvasiveBoxedInSecondHopStopsEarly(t *testing.T) {
	s := confrontTestState(9)
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {9}})
	r := NewRNG(testSeed(1), int(s.Round))

	pushback(&s, confrontation{Node: 9, Seats: []game.SeatID{0}}, 0, true, walks, r)

	if got := r.Consumed(PurposePushbackHop); got != 1 {
		t.Errorf("pushback.hop consumed = %d, want 1 (second hop boxed in, not drawn)", got)
	}
	if s.Players[0].Position != 10 {
		t.Errorf("Position = %d, want 10 (the only legal first hop)", s.Players[0].Position)
	}
}

// TestPushbackStationaryBoxedInFirstHopStaysPut is the general "any hop has
// no legal destination: the loop stops early" rule applied to the first
// hop itself — an isolated node has nowhere to go at all.
func TestPushbackStationaryBoxedInFirstHopStaysPut(t *testing.T) {
	s := confrontTestState(11)
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {11}})
	r := NewRNG(testSeed(1), int(s.Round))

	pushback(&s, confrontation{Node: 11, Seats: []game.SeatID{0}}, 0, false, walks, r)

	if got := r.Consumed(PurposePushbackHop); got != 0 {
		t.Errorf("pushback.hop consumed = %d, want 0", got)
	}
	if s.Players[0].Position != 11 {
		t.Errorf("Position = %d, want 11 (displaced as far as the graph allows: nowhere)", s.Players[0].Position)
	}
}

// --- resolveConfrontations: decisive outcomes ---

// dominantOrder and weakOrder are engineered so the dominant seat's TOTAL
// beats the weak seat's TOTAL under every possible pair of D6 rolls (1-6
// each): dominant's floor (1 + 7 modifiers = 8) exceeds weak's ceiling
// (6 + 1 modifier = 7). Used wherever a test needs a guaranteed decisive
// outcome without searching for a seed. The 7 modifiers include the Legend
// tier bonus of 3, so a caller must also set the dominant seat's Infamy to
// 9 — every caller below does — or the floor drops to 5 and the outcome
// becomes seed-dependent.
func dominantOrder() game.Order {
	return game.Order{Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 6}}
}

func weakOrder() game.Order {
	return game.Order{Stance: game.StanceOrder{Stance: game.StanceNeutral}}
}

// TestResolveConfrontationsDecisiveOneOnOne exercises the full Winner/Loser
// path at an Alley node (so the dominant seat's ambush bonus applies): +2
// Infamy and half the loser's forfeited stake for the winner; -1 Infamy,
// the stake loss, and a 1-node pushback for the loser; one EventConfrontation
// naming both.
func TestResolveConfrontationsDecisiveOneOnOne(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[1].Balance = 10
	validated := map[game.SeatID]game.Order{
		0: dominantOrder(),
		1: {Stance: game.StanceOrder{Stance: game.StanceNeutral, Stake: 3}},
	}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {13, 12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	events := resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if want := ApplyInfamyDelta(9, InfamyGainConfrontationWin); s.Players[0].Infamy != want {
		t.Errorf("winner Infamy = %d, want %d", s.Players[0].Infamy, want)
	}
	if want := ApplyInfamyDelta(0, InfamyLossConfrontationLoss); s.Players[1].Infamy != want {
		t.Errorf("loser Infamy = %d, want %d", s.Players[1].Infamy, want)
	}
	if want := 10 - 3; s.Players[1].Balance != want {
		t.Errorf("loser Balance = %d, want %d (stake 3 lost)", s.Players[1].Balance, want)
	}
	if want := 20 + 3/2; s.Players[0].Balance != want {
		t.Errorf("winner Balance = %d, want %d (collects half the loser's forfeited stake)", s.Players[0].Balance, want)
	}
	if s.Players[1].Position != 13 {
		t.Errorf("loser Position = %d, want 13 (falls back 1 node along its own route)", s.Players[1].Position)
	}
	if s.Players[0].Position != 12 {
		t.Errorf("winner Position = %d, want 12 (holds the node)", s.Players[0].Position)
	}
	if o := validated[1]; o.Route != nil || o.Action.Kind != game.ActionNothing {
		t.Errorf("loser's order = %+v, want halted (Route nil, Action Nothing)", o)
	}

	if len(events) != 1 {
		t.Fatalf("events = %v, want exactly 1 EventConfrontation", events)
	}
	ev := events[0]
	if ev.Kind != game.EventConfrontation || ev.Seat != 0 || ev.Target != 1 || ev.Node != 12 {
		t.Errorf("event = %+v, want Kind Confrontation, Seat 0, Target 1, Node 12", ev)
	}
}

// TestResolveConfrontationsBrokeEvasiveLoserNoDebt is the acceptance
// criterion verbatim: "A broke Evasive loser: cargo forfeit, winner takes
// the partial payment, no Debt, no lease surrendered, no flag."
func TestResolveConfrontationsBrokeEvasiveLoserNoDebt(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[1].Balance = 2 // less than the Cr$4 shakedown cost
	s.Players[1].Cargo = &game.CarriedCargo{Bound: false}
	validated := map[game.SeatID]game.Order{
		0: dominantOrder(),
		1: {Stance: game.StanceOrder{Stance: game.StanceEvasive}},
	}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if s.Players[1].Balance != 0 {
		t.Errorf("loser Balance = %d, want 0 (paid what little it had, floored)", s.Players[1].Balance)
	}
	if s.Players[1].Flagged {
		t.Error("loser Flagged = true, want false — the shakedown must never trigger Debt")
	}
	if len(s.Players[1].Posts) != 0 {
		t.Error("loser Posts changed, want untouched — no lease surrendered")
	}
	if s.Players[1].Cargo != nil {
		t.Error("loser still carrying cargo, want forfeit (couldn't pay the shakedown in full)")
	}
	if s.Players[0].Cargo == nil {
		t.Error("winner Cargo = nil, want the forfeit loose crate taken (empty slot, only eligible loser)")
	}
}

// TestResolveConfrontationsEvasivePaysShakedownKeepsCargo is the mirror
// case: balance covers the Cr$4 fee in full, so the cargo is protected.
func TestResolveConfrontationsEvasivePaysShakedownKeepsCargo(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[1].Balance = 20
	s.Players[1].Cargo = &game.CarriedCargo{Bound: false}
	validated := map[game.SeatID]game.Order{
		0: dominantOrder(),
		1: {Stance: game.StanceOrder{Stance: game.StanceEvasive}},
	}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if s.Players[1].Cargo == nil {
		t.Error("loser lost its cargo, want kept — the shakedown was paid in full")
	}
	if s.Players[0].Cargo != nil {
		t.Error("winner took cargo that was protected by a paid shakedown")
	}
	if got, want := s.Players[1].Balance, 20-cfg.ShakedownCost; got != want {
		t.Errorf("loser Balance = %d, want %d (paid the full shakedown)", got, want)
	}
}

// TestResolveConfrontationsFullSlotLeavesCargoBound is the acceptance
// criterion verbatim: "A winner with a full cargo slot leaves the crate
// bound at the node, not loose."
func TestResolveConfrontationsFullSlotLeavesCargoBound(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 99} // winner's slot already full
	s.Players[1].Cargo = &game.CarriedCargo{Bound: true, Contract: 5}
	s.Players[1].Contracts = []Contract{{ID: 5, Origin: 3, Destination: 7}}
	validated := map[game.SeatID]game.Order{0: dominantOrder(), 1: weakOrder()}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if s.Players[1].Cargo != nil {
		t.Error("loser still carrying cargo, want dropped (winner's slot was full)")
	}
	if len(s.Graph.Cargo) != 1 {
		t.Fatalf("Graph.Cargo = %v, want exactly 1 dropped crate", s.Graph.Cargo)
	}
	dropped := s.Graph.Cargo[0]
	if !dropped.Bound || dropped.Origin != 3 || dropped.Destination != 7 || dropped.Node != 12 {
		t.Errorf("dropped cargo = %+v, want Bound at node 12 with the original contract's Origin/Destination", dropped)
	}
}

// TestResolveConfrontationsDeadlinePauseFiresOncePerContract checks the
// acceptance criterion: a contract's DeadlinePauseUsed flag, once already
// set, leaves ExpiresRound untouched on a second loss — whether or not the
// cargo was kept (here it is forfeit and dropped, same as any other loss).
func TestResolveConfrontationsDeadlinePauseFiresOncePerContract(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[1].Cargo = &game.CarriedCargo{Bound: true, Contract: 5}
	s.Players[1].Contracts = []Contract{{ID: 5, Origin: 3, Destination: 7, ExpiresRound: 10, DeadlinePauseUsed: true}}
	validated := map[game.SeatID]game.Order{0: dominantOrder(), 1: weakOrder()}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if got := s.Players[1].Contracts[0]; got.ExpiresRound != 10 || !got.DeadlinePauseUsed {
		t.Errorf("contract = %+v, want ExpiresRound unchanged at 10, DeadlinePauseUsed still true", got)
	}
}

// TestResolveConfrontationsDeadlinePauseAppliesOnFirstLoss is the positive
// case: a contract that has never used its pause gets ExpiresRound bumped
// by exactly one and DeadlinePauseUsed set, whether or not the cargo was
// kept — here it is forfeit and dropped, same as any other loss.
func TestResolveConfrontationsDeadlinePauseAppliesOnFirstLoss(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[1].Cargo = &game.CarriedCargo{Bound: true, Contract: 5}
	s.Players[1].Contracts = []Contract{{ID: 5, Origin: 3, Destination: 7, ExpiresRound: 10}}
	validated := map[game.SeatID]game.Order{0: dominantOrder(), 1: weakOrder()}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if got := s.Players[1].Contracts[0]; got.ExpiresRound != 11 || !got.DeadlinePauseUsed {
		t.Errorf("contract = %+v, want ExpiresRound 11, DeadlinePauseUsed true", got)
	}
}

// TestApplyDeadlinePauseIsIdempotent unit-tests the pause guard directly,
// independent of the whole confrontation pipeline.
func TestApplyDeadlinePauseIsIdempotent(t *testing.T) {
	p := &Player{
		Cargo:     &game.CarriedCargo{Bound: true, Contract: 5},
		Contracts: []Contract{{ID: 5, ExpiresRound: 10}},
	}
	applyDeadlinePause(p)
	if !p.Contracts[0].DeadlinePauseUsed || p.Contracts[0].ExpiresRound != 11 {
		t.Fatalf("after first pause: %+v, want ExpiresRound 11, DeadlinePauseUsed true", p.Contracts[0])
	}
	applyDeadlinePause(p)
	if p.Contracts[0].ExpiresRound != 11 {
		t.Errorf("after second pause: ExpiresRound = %d, want unchanged at 11", p.Contracts[0].ExpiresRound)
	}
}

// --- resolveConfrontations: ties ---

// TestResolveConfrontationsTieRevertsWithNoPenalty finds a seed where two
// identically-configured seats roll the same D6 (guaranteed identical
// totals) and checks GDD §15's tie outcome: both revert to the node they
// held before this step, no stake, cargo, or Infamy change, and their
// route/action are halted.
func TestResolveConfrontationsTieRevertsWithNoPenalty(t *testing.T) {
	seed, _ := findEqualD6Seed(t, 6)

	s := confrontTestState(2, 2)
	s.Players[0].Infamy, s.Players[1].Infamy = 4, 4
	o := game.Order{Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 3}}
	validated := map[game.SeatID]game.Order{0: o, 1: o}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {1, 2}, 1: {3, 2}})
	cfg := legalTestConfig()
	r := NewRNG(seed, 6)

	events := resolveConfrontations(&s, []confrontation{{Node: 2, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if s.Players[0].Position != 1 {
		t.Errorf("seat 0 Position = %d, want 1 (the node it came from)", s.Players[0].Position)
	}
	if s.Players[1].Position != 3 {
		t.Errorf("seat 1 Position = %d, want 3 (the node it came from)", s.Players[1].Position)
	}
	if s.Players[0].Balance != 20 || s.Players[1].Balance != 20 {
		t.Errorf("balances = %d, %d, want unchanged at 20, 20 (a tie never deducts a stake)", s.Players[0].Balance, s.Players[1].Balance)
	}
	if s.Players[0].Infamy != 4 || s.Players[1].Infamy != 4 {
		t.Errorf("infamy = %d, %d, want unchanged at 4, 4", s.Players[0].Infamy, s.Players[1].Infamy)
	}
	for _, seat := range []game.SeatID{0, 1} {
		if o := validated[seat]; o.Route != nil || o.Action.Kind != game.ActionNothing {
			t.Errorf("seat %d order = %+v, want halted", seat, o)
		}
	}
	if len(events) != 1 || events[0].Seat != 0 || events[0].Target != 1 {
		t.Errorf("events = %+v, want one Confrontation entry naming both seats", events)
	}
}

// findEqualD6Seed searches small integer seeds for one where two identical,
// back-to-back PurposeConfrontD6 draws (the shape two identically-modified
// participants produce) come out equal — the fixture two-seat tie tests
// need, mirroring movement_test.go's findScavengeSeed.
func findEqualD6Seed(t *testing.T, round int) ([32]byte, int) {
	t.Helper()
	for i := range 1000 {
		seed := seedFromInt(i)
		r := NewRNG(seed, round)
		a := r.Next(PurposeConfrontD6, 6)
		b := r.Next(PurposeConfrontD6, 6)
		if a == b {
			return seed, a + 1
		}
	}
	t.Fatalf("no seed found producing two equal confront.d6 draws within 1000 tries")
	return [32]byte{}, 0
}

// --- resolveConfrontations: Muscle loss (D14) ---

func TestResolveConfrontationsMuscleLossOnDecisiveLoss(t *testing.T) {
	s := confrontTestState(12, 12)
	s.Players[0].Infamy = 9
	s.Players[0].Items = []game.ItemID{game.ItemMuscle}
	s.Players[1].Items = []game.ItemID{game.ItemMuscle}
	validated := map[game.SeatID]game.Order{0: dominantOrder(), 1: weakOrder()}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 12, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	found0 := false
	for _, it := range s.Players[0].Items {
		found0 = found0 || it == game.ItemMuscle
	}
	if !found0 {
		t.Error("winner lost Muscle, want kept")
	}
	for _, it := range s.Players[1].Items {
		if it == game.ItemMuscle {
			t.Error("loser still holds Muscle, want lost (D14: every non-winner loses it)")
		}
	}
}

func TestResolveConfrontationsTieNeverLosesMuscle(t *testing.T) {
	seed, _ := findEqualD6Seed(t, 6)

	s := confrontTestState(2, 2)
	s.Players[0].Items = []game.ItemID{game.ItemMuscle}
	s.Players[1].Items = []game.ItemID{game.ItemMuscle}
	o := game.Order{Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 3}}
	validated := map[game.SeatID]game.Order{0: o, 1: o}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {1, 2}, 1: {3, 2}})
	cfg := legalTestConfig()
	r := NewRNG(seed, 6)

	resolveConfrontations(&s, []confrontation{{Node: 2, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	for _, seat := range []game.SeatID{0, 1} {
		hasIt := false
		for _, it := range s.Players[seat].Items {
			hasIt = hasIt || it == game.ItemMuscle
		}
		if !hasIt {
			t.Errorf("seat %d lost Muscle on a tie, want kept (GDD §15: nobody loses on a tie)", seat)
		}
	}
}

// --- resolveConfrontations: melee (3+) ---

// TestResolveConfrontationsMeleeLosersDrawPushbackInSeatOrder is RFC §6.5's
// own worked case: three or more losers draw pushback.hop in seat order,
// before any of those draws are consumed. All three losers here are
// stationary (1 draw each if Neutral), so the total consumed count pins the
// ordering claim indirectly: exactly 3 pushback.hop draws, one per loser.
func TestResolveConfrontationsMeleeLosersDrawPushbackInSeatOrder(t *testing.T) {
	s := confrontTestState(5, 5, 5, 5)
	s.Players[0].Infamy = 9
	validated := map[game.SeatID]game.Order{
		0: dominantOrder(),
		1: weakOrder(),
		2: weakOrder(),
		3: weakOrder(),
	}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {5}, 1: {5}, 2: {5}, 3: {5}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	events := resolveConfrontations(&s, []confrontation{{Node: 5, Seats: []game.SeatID{0, 1, 2, 3}}}, validated, walks, cfg, r)

	if got := r.Consumed(PurposeConfrontD6); got != 4 {
		t.Errorf("confront.d6 consumed = %d, want 4 (one per participant)", got)
	}
	if got := r.Consumed(PurposePushbackHop); got != 3 {
		t.Errorf("pushback.hop consumed = %d, want 3 (one stationary Neutral loser each)", got)
	}
	if len(events) != 3 {
		t.Fatalf("events = %v, want 3 (one per loser, winner named in each)", events)
	}
	for i, ev := range events {
		if ev.Seat != 0 || ev.Target != game.SeatID(i+1) {
			t.Errorf("events[%d] = %+v, want Seat 0, Target %d", i, ev, i+1)
		}
	}
	for _, seat := range []game.SeatID{1, 2, 3} {
		if s.Players[seat].Position == 5 {
			t.Errorf("loser seat %d still at the confrontation node, want pushed back", seat)
		}
	}
}

// --- resolveConfrontations: crossing correction ---

// TestResolveConfrontationsCrossingSnapsBothToTheSameNode builds a genuine
// mid-edge crossing: advance() (movement.go) would already have carried
// both seats to their own (swapped) destinations before this runs. The
// dominant, Aggressive seat's own origin is where GDD §15a resolves the
// fight; the weak seat's destination already was that node (the "through"
// side), so only the dominant seat needs correcting — and, since it wins,
// its remaining route is halted despite winning (the one case where "route
// continues" doesn't literally hold).
func TestResolveConfrontationsCrossingSnapsBothToTheSameNode(t *testing.T) {
	s := confrontTestState(1, 0) // advance() already moved seat 0: 0->1, seat 1: 1->0
	s.Players[0].Infamy = 9
	o0 := dominantOrder()
	o0.Route = []game.NodeID{1, 2} // a further step planned past the crossing
	validated := map[game.SeatID]game.Order{0: o0, 1: weakOrder()}
	// Path already recorded advance()'s raw (uncorrected) result for both.
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {0, 1}, 1: {1, 0}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	resolveConfrontations(&s, []confrontation{{Node: 0, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if s.Players[0].Position != 0 {
		t.Errorf("winner Position = %d, want 0 (the fight's own node, corrected back from its raw destination 1)", s.Players[0].Position)
	}
	if o := validated[0]; o.Route != nil {
		t.Errorf("winner's remaining route = %v, want halted — it no longer starts from the winner's corrected position", o.Route)
	}
}

// --- resolveConfrontations: third-party displacement ---

// TestResolveConfrontationsPushbackIntoOccupiedNodeTriggersNoSecondFight is
// the acceptance criterion verbatim: "Displacement into an occupied node
// triggers no second confrontation, and both players are mutually visible."
// Since resolveConfrontations only ever processes the pending list handed
// to it for this one movement step, a pushback landing on an uninvolved
// seat's node simply cannot recurse — this pins that no extra event or
// panic results, and both end up positioned at the same node.
func TestResolveConfrontationsPushbackIntoOccupiedNodeTriggersNoSecondFight(t *testing.T) {
	s := confrontTestState(3, 3, 2) // seat 2 already sits at node 2, the loser's pushback target
	s.Players[0].Infamy = 9
	validated := map[game.SeatID]game.Order{0: dominantOrder(), 1: weakOrder()}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {3}, 1: {2, 3}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), int(s.Round))

	events := resolveConfrontations(&s, []confrontation{{Node: 3, Seats: []game.SeatID{0, 1}}}, validated, walks, cfg, r)

	if len(events) != 1 {
		t.Fatalf("events = %v, want exactly 1 — pushback must not trigger a second confrontation on its own", events)
	}
	if s.Players[1].Position != 2 {
		t.Errorf("loser Position = %d, want 2 (its own prior node, already occupied by uninvolved seat 2)", s.Players[1].Position)
	}
	if s.Players[2].Position != 2 {
		t.Errorf("uninvolved seat 2 Position = %d, want 2 (displacement must not move it)", s.Players[2].Position)
	}
}
