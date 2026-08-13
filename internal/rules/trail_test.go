package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// trailTestGraph is the fixed graph every test in this file builds on:
//
//	0 (Warehouse) - 1 (Alley) - 2 (Alley) - 3 (Border)
//	                  |
//	                  4 (Alley)
//
// Node 1 is deliberately not adjacent to node 3 (traversal-sight isolation),
// and node 4 is deliberately adjacent only to node 1 (post-sight isolation:
// a seat positioned at 3 has no sight source that reaches node 4 except a
// post staked directly on node 1's neighbourhood).
func trailTestGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2, 4}},
		{ID: 2, Type: game.NodeAlley, Edges: []game.NodeID{1, 3}},
		{ID: 3, Type: game.NodeBorder, Edges: []game.NodeID{2}},
		{ID: 4, Type: game.NodeAlley, Edges: []game.NodeID{1}},
	}}
}

// trailTestState returns a MatchState over trailTestGraph with one Player
// per position, seated in order, LastEndNode seeded to the same starting
// position (matching initial.go's own "no previous round yet" seeding) so a
// test that doesn't care about Loitering never accidentally triggers it.
func trailTestState(round game.RoundNumber, positions ...game.NodeID) MatchState {
	players := make([]Player, len(positions))
	for i, pos := range positions {
		players[i] = Player{Seat: game.SeatID(i), Position: pos, LastEndNode: pos, Fog: make([]game.FogState, 5)}
	}
	return MatchState{Round: round, Graph: trailTestGraph(), Players: players}
}

// trailTestWalks returns one stationary seatWalk per position — the
// baseline every test overrides for a seat whose Path this round actually
// traverses more than its own starting node.
func trailTestWalks(positions ...game.NodeID) map[game.SeatID]*seatWalk {
	walks := make(map[game.SeatID]*seatWalk, len(positions))
	for i, pos := range positions {
		walks[game.SeatID(i)] = &seatWalk{Previous: pos, Path: []game.NodeID{pos}}
	}
	return walks
}

// TestWriteTrailPostSightOwnNodeOnly is the acceptance criterion verbatim:
// "A post yields sight of its node and nothing adjacent" (GDD §7.2). Seat 0
// ends the round at node 3, with no sight source reaching node 1's
// neighbourhood except a post staked directly on node 1. A confrontation at
// node 1 (post's own node) must reach seat 0's archive; an identical one at
// node 4 (post's neighbour) must not.
func TestWriteTrailPostSightOwnNodeOnly(t *testing.T) {
	s := trailTestState(5, 3, 0) // seat 0 at node 3, seat 1 at node 0 (unused)
	s.Players[0].Posts = []game.NodeID{1}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	walks := trailTestWalks(3, 0)
	roundEvents := []game.Event{
		{Kind: game.EventConfrontation, Round: 5, Node: 1, Seat: 1, Target: 0},
		{Kind: game.EventConfrontation, Round: 5, Node: 4, Seat: 1, Target: 0},
	}

	writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

	if s.Players[0].Archive.Sight[1].Count() != 1 {
		t.Errorf("Archive.Sight[1] = %v, want the post's own node observed this round", s.Players[0].Archive.Sight[1])
	}
	if s.Players[0].Archive.Sight[4].Count() != 0 {
		t.Errorf("Archive.Sight[4] = %v, want the post's neighbour NOT observed", s.Players[0].Archive.Sight[4])
	}
	if n := countTrailAt(s.Players[0].Archive.Trail, 4); n != 0 {
		t.Errorf("Trail has %d entries at node 4, want 0 — post sight must not reach a neighbour", n)
	}
	if n := countTrailAt(s.Players[0].Archive.Trail, 1); n != 1 {
		t.Errorf("Trail has %d entries at node 1, want 1 — post sight must reach its own node", n)
	}
}

// TestWriteTrailTraversedNodeNoLogForTraverser is the acceptance criterion
// verbatim: "Nodes merely traversed become Known and produce no log for the
// traverser" (GDD §7.2: "Passing through is not looking around"). Seat 0
// travels 0 -> 1 -> 2 -> 3, ending at node 3. Node 1 is not adjacent to
// node 3 (trailTestGraph's own construction), so seat 0's own sight this
// round never reaches it, even though seat 0 is the one who generated the
// fresh-tracks fact there for anyone else watching.
func TestWriteTrailTraversedNodeNoLogForTraverser(t *testing.T) {
	s := trailTestState(5, 3)
	walks := trailTestWalks(3)
	walks[0].Path = []game.NodeID{0, 1, 2, 3}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	events := writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	if !hasFreshTracksAt(events, 1) {
		t.Errorf("events = %+v, want an EventFreshTracks at node 1 — someone did pass through it", events)
	}
	if s.Players[0].Archive.Sight[1].Count() != 0 {
		t.Errorf("Archive.Sight[1] = %v, want 0 — the traverser has no sight of a node it only passed through", s.Players[0].Archive.Sight[1])
	}
	if n := countTrailAt(s.Players[0].Archive.Trail, 1); n != 0 {
		t.Errorf("Trail has %d entries at node 1 for the traverser, want 0", n)
	}
	// Node 2 IS adjacent to the ending node 3, so it is legitimately sighted.
	if s.Players[0].Archive.Sight[2].Count() != 1 {
		t.Errorf("Archive.Sight[2] = %v, want 1 — node 2 is adjacent to the ending node", s.Players[0].Archive.Sight[2])
	}
}

// TestWriteTrailSurveilTwoStepSight covers GDD §7.2's Surveil source:
// "everything within 2 steps of your position, this round." Node 4 is 2
// graph steps from node 0 (0-1-4) but not adjacent to it, so only Surveil
// — not the plain ending-position rule — can put it in sight.
func TestWriteTrailSurveilTwoStepSight(t *testing.T) {
	s := trailTestState(5, 0)
	walks := trailTestWalks(0)
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionSurveil}},
	}
	roundEvents := []game.Event{
		{Kind: game.EventConfrontation, Round: 5, Node: 4, Seat: 0, Target: 0},
	}

	writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

	if s.Players[0].Archive.Sight[4].Count() != 1 {
		t.Errorf("Archive.Sight[4] = %v, want 1 — node 4 is within 2 steps under Surveil", s.Players[0].Archive.Sight[4])
	}
	if s.Players[0].Fog[4] != game.FogInSight {
		t.Errorf("Fog[4] = %v, want FogInSight", s.Players[0].Fog[4])
	}
	if s.Players[0].Archive.Sight[3].Count() != 0 {
		t.Errorf("Archive.Sight[3] = %v, want 0 — node 3 is 3 steps away, outside Surveil's radius", s.Players[0].Archive.Sight[3])
	}
}

// TestWriteTrailPoliceBandSight covers GDD §7.2/§12's Police Band source: a
// declared discard grants full sight of one chosen node this round,
// wherever the seat actually ends up.
func TestWriteTrailPoliceBandSight(t *testing.T) {
	s := trailTestState(5, 0)
	walks := trailTestWalks(0)
	validated := map[game.SeatID]game.Order{
		0: {Items: []game.ItemDiscard{{Item: game.ItemPoliceBand, Target: 3}}},
	}

	writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	if s.Players[0].Archive.Sight[3].Count() != 1 {
		t.Errorf("Archive.Sight[3] = %v, want 1 — Police Band's declared node", s.Players[0].Archive.Sight[3])
	}
}

// TestWriteTrailLoiteringEscalation is the acceptance criterion verbatim:
// "Loitering fires on the correct round for each escalation level" (GDD
// §9.1): silent on the 1st consecutive round, a sight-gated trace on the
// 2nd, a global (Event-only, no per-seat archive write — RFC §9.1 row 5)
// escalation from the 3rd onward. Seat 0 sits still at node 2 for four
// simulated rounds, taking no action each time.
func TestWriteTrailLoiteringEscalation(t *testing.T) {
	s := trailTestState(1, 2)
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}

	wantStreak := []int{1, 2, 3, 4}
	for i, round := range []game.RoundNumber{1, 2, 3, 4} {
		s.Round = round
		walks := trailTestWalks(2)

		events := writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

		if got := s.Players[0].LoiteringStreak; got != wantStreak[i] {
			t.Fatalf("round %d: LoiteringStreak = %d, want %d", round, got, wantStreak[i])
		}

		fired := hasEvent(events, game.EventLoitering, 0)
		archived := countTrailAtRound(s.Players[0].Archive.Trail, 2, round) > 0

		switch round {
		case 1:
			if fired {
				t.Errorf("round 1: EventLoitering fired, want silence (streak 1)")
			}
		case 2:
			if !fired {
				t.Errorf("round 2: EventLoitering did not fire, want the sight-gated trace")
			}
			if !archived {
				t.Errorf("round 2: no Archive.Trail entry at node 2, want the sight-gated trace archived (seat has sight of its own ending node)")
			}
		case 3, 4:
			if !fired {
				t.Errorf("round %d: EventLoitering did not fire, want the global escalation", round)
			}
			if archived {
				t.Errorf("round %d: Archive.Trail gained a new entry at node 2, want none — 3+ is Anchor-only (RFC §9.1 row 5), not SeatArchive", round)
			}
		}
	}
}

// TestWriteTrailVanishExemptsWhenInfamyReduced and its sibling below are the
// acceptance criterion verbatim: "A Vanish that reduced Infamy exempts; a
// Vanish at Infamy 0 does not" (GDD §9.1).
func TestWriteTrailVanishExemptsWhenInfamyReduced(t *testing.T) {
	s := trailTestState(2, 2)
	s.Players[0].LoiteringStreak = 1 // already loitering once, entering this round
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionVanish}}}
	walks := trailTestWalks(2)

	writeTrail(&s, validated, bySeat(s), walks, map[game.SeatID]bool{0: true}, nil, globalEventContext{})

	if got := s.Players[0].LoiteringStreak; got != 0 {
		t.Errorf("LoiteringStreak = %d, want 0 — a qualifying Vanish exempts this round entirely", got)
	}
}

func TestWriteTrailVanishAtZeroDoesNotExempt(t *testing.T) {
	s := trailTestState(2, 2)
	s.Players[0].LoiteringStreak = 1
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionVanish}}}
	walks := trailTestWalks(2)

	writeTrail(&s, validated, bySeat(s), walks, map[game.SeatID]bool{0: false}, nil, globalEventContext{})

	if got := s.Players[0].LoiteringStreak; got != 2 {
		t.Errorf("LoiteringStreak = %d, want 2 — a Vanish that did not reduce Infamy does not exempt", got)
	}
}

// TestWriteTrailVanishSuppressesFreshTracksNotLoitering is the acceptance
// criterion verbatim: "Vanish suppresses 'fresh tracks' and never suppresses
// a Loitering entry" (GDD §9.1). Seat 0 travels 2 -> 1 -> 2 (ending back
// within 1 step of last round, no qualifying action other than a
// non-qualifying Vanish at Infamy 0) and declares Vanish: node 1's would-be
// fresh-tracks entry must be entirely absent, but the Loitering streak must
// still advance and fire at its own escalation.
func TestWriteTrailVanishSuppressesFreshTracksNotLoitering(t *testing.T) {
	s := trailTestState(2, 2)
	s.Players[0].LoiteringStreak = 1
	walks := trailTestWalks(2)
	walks[0].Path = []game.NodeID{2, 1, 2}
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionVanish}}}

	events := writeTrail(&s, validated, bySeat(s), walks, map[game.SeatID]bool{0: false}, nil, globalEventContext{})

	if hasFreshTracksAt(events, 1) {
		t.Errorf("events = %+v, want no EventFreshTracks at node 1 — Vanish suppresses it", events)
	}
	if got := s.Players[0].LoiteringStreak; got != 2 {
		t.Errorf("LoiteringStreak = %d, want 2 — Vanish never suppresses Loitering", got)
	}
	if !hasEvent(events, game.EventLoitering, 0) {
		t.Errorf("events = %+v, want EventLoitering to have fired (streak reached 2)", events)
	}
}

// TestWriteTrailArchiveRate is the acceptance criterion verbatim: "A node
// watched 6 rounds with traffic on 4 reports 4/6; a node watched with no
// traffic reports 0/N, not absence" (RFC §9.2, §16.1's Archive row). Seat 0
// holds a post at node 1, guaranteeing sight of it every round regardless
// of where seat 0 itself ends up; seat 1 generates a confrontation there in
// 4 of 6 rounds.
func TestWriteTrailArchiveRate(t *testing.T) {
	s := trailTestState(1, 3, 2) // seat 0 far away at 3, seat 1 at 2 (adjacent to 1)
	s.Players[0].Posts = []game.NodeID{1}
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	traffic := map[game.RoundNumber]bool{1: true, 2: false, 3: true, 4: true, 5: false, 6: true}

	for round := game.RoundNumber(1); round <= 6; round++ {
		s.Round = round
		walks := trailTestWalks(3, 2)
		var roundEvents []game.Event
		if traffic[round] {
			roundEvents = []game.Event{{Kind: game.EventConfrontation, Round: round, Node: 1, Seat: 1, Target: 0}}
		}
		writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})
	}

	if got := s.Players[0].Archive.Sight[1].Count(); got != 6 {
		t.Errorf("Archive.Sight[1] observed rounds = %d, want 6", got)
	}
	if got := countTrailAt(s.Players[0].Archive.Trail, 1); got != 4 {
		t.Errorf("Trail entries at node 1 = %d, want 4 (traffic rounds only)", got)
	}

	// Node 4: never anything there, never even sighted — an absence, not
	// an honest zero, since seat 0 never had sight of it at all.
	if got := s.Players[0].Archive.Sight[4].Count(); got != 0 {
		t.Errorf("Archive.Sight[4] = %d, want 0 (never sighted, distinct from an honest zero)", got)
	}
}

// TestWriteTrailRainSuppressesFreshTracksOnly and its Obscured-vs-honest-
// zero siblings below cover D13: Rain (GDD §14.2) erases exactly the
// "fresh tracks" kind, nothing else, and only where erasing it actually
// erases something.
func TestWriteTrailRainSuppressesFreshTracksOnly(t *testing.T) {
	s := trailTestState(4, 3, 2) // round 4: EventDeck[0] must be Rain; seat 1 ends at node 2
	s.Graph.EventDeck = []EventCardID{EventRain}
	s.Players[0].Posts = []game.NodeID{1}
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	walks := trailTestWalks(3, 2)
	// Node 1 gets BOTH a confrontation (survives Rain) and fresh tracks
	// from seat 1 passing through it on the way to node 2.
	walks[1].Path = []game.NodeID{0, 1, 2}
	roundEvents := []game.Event{{Kind: game.EventConfrontation, Round: 4, Node: 1, Seat: 1, Target: 1}}

	events := writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

	if hasFreshTracksAt(events, 1) {
		t.Errorf("events = %+v, want no EventFreshTracks at node 1 — Rain records none anywhere this round (D13)", events)
	}
	if s.Players[0].Archive.Obscured[1].Count() != 0 {
		t.Errorf("Archive.Obscured[1] = %v, want 0 — a real confrontation entry survived Rain", s.Players[0].Archive.Obscured[1])
	}
	if s.Players[0].Archive.Sight[1].Count() != 1 {
		t.Errorf("Archive.Sight[1] = %v, want 1", s.Players[0].Archive.Sight[1])
	}
	found := false
	for _, te := range s.Players[0].Archive.Trail {
		if te.Node == 1 && te.Kind == game.EventFreshTracks {
			t.Errorf("Trail contains a FreshTracks entry at node 1, want it removed by Rain")
		}
		if te.Node == 1 && te.Kind == game.EventConfrontation {
			found = true
		}
	}
	if !found {
		t.Errorf("Trail has no Confrontation entry at node 1, want the non-fresh-tracks entry unaffected by Rain")
	}
}

// TestWriteTrailFreshTracksExcludesRevisitedEndingNode is a regression test
// for a CodeRabbit finding on this PR: a there-and-back route can revisit
// its own final node earlier in the same round's path (Path == {2, 1, 2}),
// and that earlier visit must not be mistaken for "passing through" — the
// seat ends at node 2, so node 2 is a sight source, never a fresh-tracks
// one, however many times the path touches it first.
func TestWriteTrailFreshTracksExcludesRevisitedEndingNode(t *testing.T) {
	s := trailTestState(5, 2)
	walks := trailTestWalks(2)
	walks[0].Path = []game.NodeID{2, 1, 2}
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}

	events := writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	if hasFreshTracksAt(events, 2) {
		t.Errorf("events = %+v, want no EventFreshTracks at node 2 — it is the ending node, revisited or not", events)
	}
	if !hasFreshTracksAt(events, 1) {
		t.Errorf("events = %+v, want an EventFreshTracks at node 1 — it was genuinely passed through", events)
	}
}

func TestWriteTrailRainObscuresNodeWhoseOnlyEntryWasFreshTracks(t *testing.T) {
	s := trailTestState(4, 3) // round 4: EventDeck[0] must be Rain
	s.Graph.EventDeck = []EventCardID{EventRain}
	s.Players[0].Posts = []game.NodeID{1}
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
	walks := trailTestWalks(3)
	walks[0].Path = []game.NodeID{0, 1, 2, 3} // seat 0 itself passes through node 1

	writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	if got := s.Players[0].Archive.Obscured[1].Count(); got != 1 {
		t.Errorf("Archive.Obscured[1] = %d, want 1 — the only entry there (fresh tracks) was erased by Rain", got)
	}
	if got := s.Players[0].Archive.Sight[1].Count(); got != 0 {
		t.Errorf("Archive.Sight[1] = %d, want 0 — an obscured round must not also count as an honest zero", got)
	}
}

func TestWriteTrailNoTrafficIsHonestZeroNotObscured(t *testing.T) {
	s := trailTestState(4, 3) // round 4: Rain is active, but node 1 sees no traffic at all
	s.Graph.EventDeck = []EventCardID{EventRain}
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
	walks := trailTestWalks(3)

	writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	if got := s.Players[0].Archive.Sight[2].Count(); got != 1 {
		t.Errorf("Archive.Sight[2] = %d, want 1 — genuinely no traffic is an honest zero", got)
	}
	if got := s.Players[0].Archive.Obscured[2].Count(); got != 0 {
		t.Errorf("Archive.Obscured[2] = %d, want 0 — Rain has nothing to erase where nothing happened", got)
	}
}

// TestWriteTrailBlackoutObscuresEveryEntryKind is GDD §14.2's Blackout:
// "nobody generates trail entries" — broader than Rain's fresh-tracks-only
// suppression (D13), erasing any entry kind, read from the next-round
// modifier a prior round's card set (MatchState.NextRound.Blackout, live
// state, not a deck peek).
func TestWriteTrailBlackoutObscuresEveryEntryKind(t *testing.T) {
	s := trailTestState(5, 3)
	s.NextRound.Blackout = true
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
	walks := trailTestWalks(3)
	roundEvents := []game.Event{{Kind: game.EventCargoTaken, Round: 5, Node: 3, Seat: 0}}

	events := writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

	if got := s.Players[0].Archive.Obscured[3].Count(); got != 1 {
		t.Errorf("Archive.Obscured[3] = %d, want 1 — Blackout erases a cargo-taken entry too, not just fresh tracks", got)
	}
	if got := s.Players[0].Archive.Sight[3].Count(); got != 0 {
		t.Errorf("Archive.Sight[3] = %d, want 0 — an obscured round must not also count as an honest zero", got)
	}
	if n := countTrailAt(s.Players[0].Archive.Trail, 3); n != 0 {
		t.Errorf("Trail has %d entries at node 3, want 0", n)
	}
	for _, e := range events {
		if e.Kind == game.EventFreshTracks {
			t.Errorf("events = %+v, want no EventFreshTracks — Blackout suppresses the returned stream too", events)
		}
	}
}

// TestWriteTrailBlackoutCapsSightToOwnNodeOnly is GDD §14.2's Blackout's
// other half: "nobody has sight beyond their own node" — no exception for
// a held post, unlike an ordinary round (TestWriteTrailPostSightOwnNodeOnly
// above already covers the ordinary, unrestricted post-sight case).
func TestWriteTrailBlackoutCapsSightToOwnNodeOnly(t *testing.T) {
	s := trailTestState(5, 3, 0) // seat 0 at node 3, with a post far away at node 1
	s.NextRound.Blackout = true
	s.Players[0].Posts = []game.NodeID{1}
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	walks := trailTestWalks(3, 0)
	roundEvents := []game.Event{{Kind: game.EventConfrontation, Round: 5, Node: 1, Seat: 1, Target: 0}}

	writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

	if got := s.Players[0].Archive.Sight[1].Count(); got != 0 {
		t.Errorf("Archive.Sight[1] = %d, want 0 — Blackout overrides even a held post's own-node sight", got)
	}
	if got := s.Players[0].Archive.Obscured[1].Count(); got != 0 {
		t.Errorf("Archive.Obscured[1] = %d, want 0 — node 1 is outside sight entirely, not an erased-but-sighted round", got)
	}
}

// TestWriteTrailFestivalSuppressesOwnEntryAtDrawnNode is GDD §14.2's
// Festival: "leaves no trace" for the seat ending this round at the
// already-drawn node (ctx.festivalNode) — never populates Obscured either
// (D13: the same shape as Vanish and Distracted Guard, a designed evasion,
// not a world-level erasure), so the node reads an honest, non-Obscured
// zero to any other watching seat.
func TestWriteTrailFestivalSuppressesOwnEntryAtDrawnNode(t *testing.T) {
	s := trailTestState(5, 3, 3) // both seats end at node 3, the drawn Festival node
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	walks := trailTestWalks(3, 3)
	roundEvents := []game.Event{{Kind: game.EventCargoTaken, Round: 5, Node: 3, Seat: 0}}
	ctx := globalEventContext{live: true, card: EventFestival, festivalNode: 3}

	writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, ctx)

	if n := countTrailAt(s.Players[1].Archive.Trail, 3); n != 0 {
		t.Errorf("seat 1's Trail has %d entries at node 3, want 0 — Festival suppresses seat 0's own cargo-taken entry", n)
	}
	if got := s.Players[1].Archive.Sight[3].Count(); got != 1 {
		t.Errorf("Archive.Sight[3] = %d, want 1 — an honest zero, not an absence", got)
	}
	if got := s.Players[1].Archive.Obscured[3].Count(); got != 0 {
		t.Errorf("Archive.Obscured[3] = %d, want 0 — Festival never populates Obscured (D13)", got)
	}
}

// TestWriteTrailCargoTakenNamingGate covers GDD §7.3 row 1's Infamy >= 3
// name gate, both sides of the boundary.
func TestWriteTrailCargoTakenNamingGate(t *testing.T) {
	for _, tc := range []struct {
		infamy int
		named  bool
	}{
		{2, false},
		{3, true},
	} {
		s := trailTestState(5, 0)
		s.Players[0].Infamy = tc.infamy
		validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
		walks := trailTestWalks(0)
		roundEvents := []game.Event{{Kind: game.EventCargoTaken, Round: 5, Node: 0, Seat: 0}}

		writeTrail(&s, validated, bySeat(s), walks, nil, roundEvents, globalEventContext{})

		te := findTrailEntry(t, s.Players[0].Archive.Trail, 0, game.EventCargoTaken)
		gotNamed := te.Actor != nil
		if gotNamed != tc.named {
			t.Errorf("infamy %d: named = %v, want %v", tc.infamy, gotNamed, tc.named)
		}
	}
}

// TestWriteTrailDecoyMatchesRealCargoTaken is D12's own indistinguishability
// requirement: a Decoy discard produces the identical TrailEntry shape as a
// real pickup, gated by the same Infamy >= 3 rule, naming only the planter.
func TestWriteTrailDecoyMatchesRealCargoTaken(t *testing.T) {
	s := trailTestState(5, 1) // seat 0 ends at node 1, so node 2 (adjacent) is in sight
	s.Players[0].Infamy = 3
	validated := map[game.SeatID]game.Order{
		0: {Items: []game.ItemDiscard{{Item: game.ItemDecoy, Target: 2}}},
	}
	walks := trailTestWalks(1)

	writeTrail(&s, validated, bySeat(s), walks, nil, nil, globalEventContext{})

	te := findTrailEntry(t, s.Players[0].Archive.Trail, 2, game.EventCargoTaken)
	if te.Actor == nil || *te.Actor != 0 {
		t.Errorf("Decoy entry Actor = %v, want the planter (seat 0)", te.Actor)
	}
}

// countTrailAt counts trail entries recorded for node, across every kind.
func countTrailAt(trail []game.StampedTrailEntry, node game.NodeID) int {
	n := 0
	for _, te := range trail {
		if te.Node == node {
			n++
		}
	}
	return n
}

// countTrailAtRound counts trail entries recorded for node in exactly
// round, distinguishing a fresh write this round from an earlier round's
// entry the cumulative archive is still carrying.
func countTrailAtRound(trail []game.StampedTrailEntry, node game.NodeID, round game.RoundNumber) int {
	n := 0
	for _, te := range trail {
		if te.Node == node && te.Round == round {
			n++
		}
	}
	return n
}

// hasFreshTracksAt reports whether events contains an EventFreshTracks at
// node.
func hasFreshTracksAt(events []game.Event, node game.NodeID) bool {
	for _, e := range events {
		if e.Kind == game.EventFreshTracks && e.Node == node {
			return true
		}
	}
	return false
}

// findTrailEntry returns the single archived entry of kind at node, failing
// the test if there is not exactly one.
func findTrailEntry(t *testing.T, trail []game.StampedTrailEntry, node game.NodeID, kind game.EventKind) game.TrailEntry {
	t.Helper()
	var found []game.TrailEntry
	for _, te := range trail {
		if te.Node == node && te.Kind == kind {
			found = append(found, te.TrailEntry)
		}
	}
	if len(found) != 1 {
		t.Fatalf("Trail has %d entries of kind %v at node %d, want exactly 1", len(found), kind, node)
	}
	return found[0]
}
