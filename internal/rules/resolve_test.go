package rules

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// resolveTestState mirrors legalTestView's fixed graph (legal_test.go),
// extended with a live Graph.Nodes so it can stand as full MatchState:
//
//	0 (Warehouse, Known) --- 1 (Alley, Known) --- 2 (Border, Known) --- 3 (Alley, Hidden)
//	 \
//	  4 (Alley, Rumoured — no edges disclosed)
//
// Two seats: seat 0 starts at node 0, seat 1 at node 2 — far enough apart
// that the movement stubs (still no-ops in this issue) never need to
// interact for these tests to hold.
func resolveTestState() MatchState {
	fog := []game.FogState{game.FogKnown, game.FogKnown, game.FogKnown, game.FogHidden, game.FogRumoured}
	return MatchState{
		Round: 4,
		Graph: Graph{
			Nodes: []Node{
				{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1, 4}},
				{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
				{ID: 2, Type: game.NodeBorder, Edges: []game.NodeID{1, 3}},
				{ID: 3, Type: game.NodeAlley, Edges: []game.NodeID{2}},
				{ID: 4, Type: game.NodeAlley, Edges: []game.NodeID{0}},
			},
		},
		Players: []Player{
			{Seat: 0, Balance: 20, Position: 0, Fog: append([]game.FogState(nil), fog...)},
			{Seat: 1, Balance: 20, Position: 2, Fog: append([]game.FogState(nil), fog...)},
		},
	}
}

// TestResolveIsDeterministic is RFC §16.2's headline invariant:
// resolve(s, o) == resolve(s, o), byte-identical, on the same inputs.
func TestResolveIsDeterministic(t *testing.T) {
	s := resolveTestState()
	cfg := legalTestConfig()
	orders := map[game.SeatID]game.Order{}

	next1, events1, err1 := Resolve(s, orders, cfg, NewRNG(testSeed(1), int(s.Round)+1))
	next2, events2, err2 := Resolve(s, orders, cfg, NewRNG(testSeed(1), int(s.Round)+1))

	if err1 != nil || err2 != nil {
		t.Fatalf("Resolve() errors = %v, %v, want nil, nil", err1, err2)
	}
	if !reflect.DeepEqual(next1, next2) {
		t.Fatalf("Resolve() produced different states on identical inputs:\n%+v\n%+v", next1, next2)
	}
	if !reflect.DeepEqual(events1, events2) {
		t.Fatalf("Resolve() produced different events on identical inputs:\n%+v\n%+v", events1, events2)
	}
}

// TestResolveDoesNotMutateInput is the acceptance criterion verbatim:
// "Resolve never mutates its argument; the returned state is new and the
// input is byte-identical afterwards." s is deep-copied before the call so
// the comparison cannot be fooled by Resolve and the test sharing the same
// backing arrays.
func TestResolveDoesNotMutateInput(t *testing.T) {
	s := resolveTestState()
	before := s.clone()

	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1}},
	}

	if _, _, err := Resolve(s, orders, legalTestConfig(), NewRNG(testSeed(2), int(s.Round)+1)); err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(s, before) {
		t.Fatalf("Resolve() mutated its input:\ngot  %+v\nwant %+v", s, before)
	}
}

// TestResolveFreezesFlaggedAndEvasiveStepPenalty locks RFC §6.6/D5's rule:
// both counters are consumed from the entry snapshot and reset on the live
// Player at the same top-of-Resolve point the snapshot is taken — never
// left set for a step later in the pipeline to see.
func TestResolveFreezesFlaggedAndEvasiveStepPenalty(t *testing.T) {
	s := resolveTestState()
	s.Players[0].Flagged = true
	s.Players[1].EvasiveStepPenalty = true

	next, _, err := Resolve(s, nil, legalTestConfig(), NewRNG(testSeed(3), int(s.Round)+1))
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	if next.Players[0].Flagged {
		t.Error("Players[0].Flagged still true after Resolve, want reset to false")
	}
	if next.Players[1].EvasiveStepPenalty {
		t.Error("Players[1].EvasiveStepPenalty still true after Resolve, want reset to false")
	}
}

// TestResolveIncrementsRound checks Round advances by exactly one per call
// — MatchState's own comment defines Round as "the last round Resolve
// folded in".
func TestResolveIncrementsRound(t *testing.T) {
	s := resolveTestState()
	next, _, err := Resolve(s, nil, legalTestConfig(), NewRNG(testSeed(4), int(s.Round)+1))
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if next.Round != s.Round+1 {
		t.Errorf("next.Round = %d, want %d", next.Round, s.Round+1)
	}
}

// TestMovementStepsAtLeastOneWhenEveryRouteEmpty is the acceptance
// criterion verbatim: "Collision evaluation runs on a round where every
// route is empty" (RFC §6.7, GDD §15 — a table where nobody moves still
// resolves). movementSteps is Resolve's movement-loop bound; asserting it
// directly proves the loop executes (and so calls detectCollisions) at
// least once even with nothing to move, without needing to instrument the
// still-stub movement functions themselves.
func TestMovementStepsAtLeastOneWhenEveryRouteEmpty(t *testing.T) {
	validated := map[game.SeatID]game.Order{
		0: {},
		1: {Route: nil},
	}
	if got := movementSteps(validated); got < 1 {
		t.Errorf("movementSteps(all empty) = %d, want >= 1", got)
	}
}

// TestMovementStepsTracksLongestRoutePlusPushingOn checks the bound also
// grows with the longest submitted route or Pushing On extension — not
// just that the floor works.
func TestMovementStepsTracksLongestRoutePlusPushingOn(t *testing.T) {
	validated := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 2}},
		1: {Route: []game.NodeID{1}, PushingOn: game.PushingOn{Steps: 2}},
	}
	if got, want := movementSteps(validated), 3; got != want {
		t.Errorf("movementSteps() = %d, want %d", got, want)
	}
}

// TestResolvePipelineOrderStructural is the acceptance criterion verbatim:
// "the pipeline order is asserted structurally, not just behaviourally —
// a stub test that each step ran, in order, once." It scans resolve.go's
// source text for the Resolve function body and asserts every RFC §6.7
// pipeline step's call appears exactly once, in the documented order —
// the same source-scanning approach ordering_test.go's
// TestOnlyOrderingFileUsesSortSlice already uses for its own structural
// guarantee in this package.
//
// This is a fail-closed gate (CLAUDE.md): if the file can't be read or the
// Resolve function can't be found, that is a broken check, not a passing
// one.
func TestResolvePipelineOrderStructural(t *testing.T) {
	data, err := os.ReadFile("resolve.go")
	if err != nil {
		t.Fatalf("ReadFile(resolve.go): %v", err)
	}
	src := string(data)

	start := strings.Index(src, "func Resolve(")
	if start < 0 {
		t.Fatal("resolve.go has no func Resolve( — nothing to check")
	}
	body := resolveFunctionBody(t, src[start:])

	steps := []string{
		"validate(",
		"advance(",
		"detectCrossings(",
		"detectCollisions(",
		"resolveConfrontations(",
		"resolveActions(",
		"resolveDeliveries(",
		"resolveAddons(",
		"writeTrail(",
		"globalEvent(",
		"incident(",
		"pressure(",
		"upkeep(",
	}

	cursor := 0
	for _, step := range steps {
		idx := strings.Index(body[cursor:], step)
		if idx < 0 {
			t.Fatalf("Resolve() body does not call %s after position %d — pipeline steps must appear in RFC §6.7's documented order", step, cursor)
		}
		cursor += idx + len(step)

		if n := strings.Count(body, step); n != 1 {
			t.Errorf("Resolve() body calls %s %d times, want exactly 1", step, n)
		}
	}
}

// resolveFunctionBody isolates fromResolve — src starting at "func
// Resolve(" — down to the closing brace that matches Resolve's own
// opening one, by counting braces rather than searching for the next
// "func" keyword: a doc comment between Resolve and its neighbour can
// legitimately mention another step's name (as resetRoundFlags's does,
// for upkeep), which a text-based "next func" cut would wrongly fold into
// Resolve's own body and double-count.
func resolveFunctionBody(t *testing.T, fromResolve string) string {
	t.Helper()

	open := strings.Index(fromResolve, "{")
	if open < 0 {
		t.Fatal("func Resolve( has no opening brace — nothing to check")
	}

	depth := 0
	for i, r := range fromResolve[open:] {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return fromResolve[:open+i+1]
			}
		}
	}

	t.Fatal("func Resolve( never closes — nothing to check")
	return ""
}
