package rules

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestSnapshotIsIsolatedFromLaterMutation is the acceptance criterion
// verbatim: "a test mutates the state and asserts the snapshot is
// unchanged." Every field on SeatSnapshot is a plain value (int, bool, or a
// named integer ID), so a value-copying loop is sufficient — this test is
// what makes that claim more than an assertion in a comment.
func TestSnapshotIsIsolatedFromLaterMutation(t *testing.T) {
	s := MatchState{
		Round: 4,
		Players: []Player{
			{Seat: 0, Balance: 10, Infamy: 3, Position: 5, Flagged: true},
			{Seat: 1, Balance: 20, Infamy: 6, Position: 7, EvasiveStepPenalty: true},
		},
	}

	entry := s.Snapshot()

	want := EntrySnapshot{Seats: []SeatSnapshot{
		{Seat: 0, Balance: 10, Infamy: 3, Position: 5, Flagged: true},
		{Seat: 1, Balance: 20, Infamy: 6, Position: 7, EvasiveStepPenalty: true},
	}}
	if !reflect.DeepEqual(entry, want) {
		t.Fatalf("Snapshot() = %+v, want %+v", entry, want)
	}

	// Mutate the live state after the snapshot was taken — every field the
	// snapshot captured, including the two step-allowance booleans.
	s.Players[0].Balance = 999
	s.Players[0].Infamy = 0
	s.Players[0].Position = 0
	s.Players[0].Flagged = false
	s.Players[1].EvasiveStepPenalty = false

	if !reflect.DeepEqual(entry, want) {
		t.Fatalf("later mutation of MatchState reached the snapshot: got %+v, want %+v", entry, want)
	}
}

// TestSnapshotSeatsAreSeatIndexed pins the ordering convention MatchState's
// own comment claims: Snapshot() must not reorder, drop, or map-scramble
// seats — Seats[i] must be Players[i], unchanged.
func TestSnapshotSeatsAreSeatIndexed(t *testing.T) {
	s := MatchState{Players: []Player{
		{Seat: 0}, {Seat: 1}, {Seat: 2},
	}}

	entry := s.Snapshot()
	if len(entry.Seats) != 3 {
		t.Fatalf("len(Seats) = %d, want 3", len(entry.Seats))
	}
	for i, seat := range entry.Seats {
		if seat.Seat != game.SeatID(i) {
			t.Errorf("Seats[%d].Seat = %v, want %v", i, seat.Seat, game.SeatID(i))
		}
	}
}

// TestCrossRoundCountersLiveWhereTheTableSays locks the placement RFC §6.6
// and the issue's acceptance criteria specify: all eight counters exist,
// seven on Player and DeadlinePauseUsed on Contract instead — never on the
// seat, since two seats can hold contracts with the same origin/destination
// pair and must never share or inherit each other's flag (GDD §8.4).
func TestCrossRoundCountersLiveWhereTheTableSays(t *testing.T) {
	// Constructing these by field name is itself the assertion that every
	// counter in RFC §6.6's table exists with the right type — a rename or
	// a dropped field fails this at compile time, not at review.
	_ = Player{
		LastEndNode:          game.NodeID(3),
		LoiteringStreak:      2,
		LooseCrateHeldRounds: 1,
		Flagged:              true,
		EvasiveStepPenalty:   true,
		LastOfferRound:       game.RoundNumber(5),
		ConsecutiveDefaults:  1,
	}
	_ = Contract{DeadlinePauseUsed: true}

	if _, ok := reflect.TypeFor[Player]().FieldByName("DeadlinePauseUsed"); ok {
		t.Fatal("Player must not have a DeadlinePauseUsed field — GDD §8.4 puts the flag on the contract instance, not the seat")
	}
}

// TestGameDoesNotImportRules pins the direction of the dependency the fog
// gate assumes: internal/game is the leaf, internal/rules — home to
// MatchState, the thing the fog boundary hides — is what imports it, never
// the other way around (RFC §6.1, D01). check-fog-boundary.sh and
// check-packages.sh both check this from the other side (render/web can't
// reach rules; game has zero non-stdlib deps, so it already can't reach
// rules either) — this test asserts it directly, from here, since this is
// the file that would notice first if that ever stopped being true.
func TestGameDoesNotImportRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/garnizeh/cinzal/internal/game").Output()
	if err != nil {
		t.Fatalf("go list -deps internal/game: %v", err)
	}

	deps := strings.Fields(string(out))
	if len(deps) == 0 {
		t.Fatal("go list -deps internal/game reported nothing — the check ran over no packages, which is not the same as passing")
	}

	const forbidden = "github.com/garnizeh/cinzal/internal/rules"
	for _, d := range deps {
		if d == forbidden || strings.HasPrefix(d, forbidden+"/") {
			t.Fatalf("internal/game imports %s — the leaf package must import nothing but the standard library (D01)", d)
		}
	}
}

// TestStateTypeGraphHasNoFloatOrMap enforces two of #57's acceptance
// criteria mechanically: "No map is ranged over anywhere in this package"
// and "No float64 anywhere in the type graph." A map field is the
// precondition for the map-ranging hazard RFC §6.3 names — Nodes and
// Players are deliberately ID-indexed slices instead — so catching the
// field is equivalent to catching the hazard before any resolution code
// exists to range over it.
//
// The walk stops at the boundary of package game: game.SeatArchive already
// carries maps (RFC §9.2's per-node RoundSet), by a design accepted in a
// different package under different rules, and re-litigating that here
// would make this test fail for a reason outside #57's scope.
func TestStateTypeGraphHasNoFloatOrMap(t *testing.T) {
	pkgPath := reflect.TypeFor[MatchState]().PkgPath()

	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ == nil || seen[typ] {
			return
		}
		seen[typ] = true

		switch typ.Kind() {
		case reflect.Float32, reflect.Float64:
			t.Errorf("%s is a floating-point type — RFC §6.3 forbids float64 anywhere in the rules type graph", typ)
		case reflect.Map:
			t.Errorf("%s is a map type — RFC §6.3: resolution must never range over a map", typ)
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem())
		case reflect.Struct:
			// Fields belonging to another package obey that package's own
			// rules (see game.SeatArchive above) — do not descend into them.
			if typ.PkgPath() != "" && typ.PkgPath() != pkgPath {
				return
			}
			for f := range typ.Fields() {
				walk(f.Type)
			}
		}
	}

	walk(reflect.TypeFor[MatchState]())
	walk(reflect.TypeFor[EntrySnapshot]())
}

// cloneTestState builds a MatchState with every slice and pointer field
// clone() must deep-copy populated with a distinct value, so a mutation
// through the clone has somewhere real to land if aliasing slipped in.
func cloneTestState() MatchState {
	post := Post{Owner: 1, RoundsRemaining: 3}
	cargo := &game.CarriedCargo{Bound: true, Contract: 7}
	actor := game.SeatID(0)
	target := game.SeatID(1)
	item := game.ItemShiv

	return MatchState{
		Round: 2,
		Graph: Graph{
			Nodes: []Node{
				{
					ID:     0,
					Edges:  []game.NodeID{1, 2},
					Post:   &post,
					Market: []game.ItemID{game.ItemShiv, game.ItemMuscle},
				},
			},
			Cargo:        []Cargo{{Node: 0, Bound: true, Origin: 0, Destination: 1}},
			EventDeck:    []EventCardID{1, 2, 3},
			IncidentDeck: []IncidentCardID{4, 5, 6},
		},
		Players: []Player{
			{
				Seat:      0,
				Cargo:     cargo,
				Contracts: []Contract{{ID: 1, Origin: 0, Destination: 1, Tier: 2}},
				Items:     []game.ItemID{game.ItemDecoy},
				Posts:     []game.NodeID{0},
				Fog:       []game.FogState{game.FogKnown, game.FogRumoured},
				Archive: game.SeatArchive{
					Sight:    map[game.NodeID]game.RoundSet{0: 0b101},
					Obscured: map[game.NodeID]game.RoundSet{1: 0b010},
					Trail: []game.StampedTrailEntry{{
						TrailEntry: game.TrailEntry{
							Kind:   game.EventConfrontation,
							Node:   0,
							Actor:  &actor,
							Target: &target,
							Item:   &item,
						},
						Round: 1,
					}},
				},
			},
		},
	}
}

// TestNodeClone is a focused, direct test of Node.clone() — Edges and
// Market copied, Post reallocated so the clone owns its own lease record.
func TestNodeClone(t *testing.T) {
	post := Post{Owner: 1, RoundsRemaining: 3}
	original := Node{
		ID:     0,
		Edges:  []game.NodeID{1, 2},
		Post:   &post,
		Market: []game.ItemID{game.ItemShiv, game.ItemMuscle},
	}
	want := Node{
		ID:     0,
		Edges:  []game.NodeID{1, 2},
		Post:   &post,
		Market: []game.ItemID{game.ItemShiv, game.ItemMuscle},
	}

	got := original.clone()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Node.clone() = %+v, want a value-equal copy %+v", got, want)
	}
	if got.Post == original.Post {
		t.Fatal("Node.clone().Post aliases the original's Post pointer")
	}

	got.Edges[0] = 99
	got.Edges = append(got.Edges, 100)
	got.Post.RoundsRemaining = 99
	got.Market[0] = game.ItemBoltHole

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("mutating Node.clone()'s result reached the original:\ngot  %+v\nwant %+v", original, want)
	}
}

// TestNodeCloneHandlesNilFields checks the nil-Edges/nil-Post/nil-Market
// case doesn't panic and preserves nil (see TestMatchStateCloneHandlesNilFields
// for why nil vs. empty matters here).
func TestNodeCloneHandlesNilFields(t *testing.T) {
	got := Node{}.clone()
	if !reflect.DeepEqual(got, Node{}) {
		t.Fatalf("Node{}.clone() = %+v, want the zero value", got)
	}
}

// TestGraphClone is a focused, direct test of Graph.clone() — every one of
// its four slice fields copied, and each Node inside Nodes cloned in turn
// rather than shared with the original.
func TestGraphClone(t *testing.T) {
	original := Graph{
		Nodes:        []Node{{ID: 0, Edges: []game.NodeID{1}}},
		Cargo:        []Cargo{{Node: 0, Bound: true, Origin: 0, Destination: 1}},
		EventDeck:    []EventCardID{1, 2},
		IncidentDeck: []IncidentCardID{3, 4},
	}
	want := Graph{
		Nodes:        []Node{{ID: 0, Edges: []game.NodeID{1}}},
		Cargo:        []Cargo{{Node: 0, Bound: true, Origin: 0, Destination: 1}},
		EventDeck:    []EventCardID{1, 2},
		IncidentDeck: []IncidentCardID{3, 4},
	}

	got := original.clone()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Graph.clone() = %+v, want a value-equal copy %+v", got, want)
	}

	got.Nodes[0].Edges[0] = 99
	got.Nodes = append(got.Nodes, Node{ID: 1})
	got.Cargo[0].Node = 99
	got.EventDeck[0] = 99
	got.IncidentDeck[0] = 99

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("mutating Graph.clone()'s result reached the original:\ngot  %+v\nwant %+v", original, want)
	}
}

// TestGraphCloneHandlesNilFields checks the all-nil case doesn't panic and
// preserves nil.
func TestGraphCloneHandlesNilFields(t *testing.T) {
	got := Graph{}.clone()
	if !reflect.DeepEqual(got, Graph{}) {
		t.Fatalf("Graph{}.clone() = %+v, want the zero value", got)
	}
}

// TestPlayerClone is a focused, direct test of Player.clone() — every
// slice, the carried-cargo pointer, and the archive all copied
// independently of the original.
func TestPlayerClone(t *testing.T) {
	cargo := &game.CarriedCargo{Bound: true, Contract: 7}
	archive := game.SeatArchive{Sight: map[game.NodeID]game.RoundSet{0: 0b101}}

	original := Player{
		Seat:      0,
		Cargo:     cargo,
		Contracts: []Contract{{ID: 1, Origin: 0, Destination: 1, Tier: 2}},
		Items:     []game.ItemID{game.ItemDecoy},
		Posts:     []game.NodeID{0},
		Fog:       []game.FogState{game.FogKnown},
		Archive:   archive,
	}
	want := Player{
		Seat:      0,
		Cargo:     &game.CarriedCargo{Bound: true, Contract: 7},
		Contracts: []Contract{{ID: 1, Origin: 0, Destination: 1, Tier: 2}},
		Items:     []game.ItemID{game.ItemDecoy},
		Posts:     []game.NodeID{0},
		Fog:       []game.FogState{game.FogKnown},
		Archive:   game.SeatArchive{Sight: map[game.NodeID]game.RoundSet{0: 0b101}},
	}

	got := original.clone()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Player.clone() = %+v, want a value-equal copy %+v", got, want)
	}
	if got.Cargo == original.Cargo {
		t.Fatal("Player.clone().Cargo aliases the original's Cargo pointer")
	}

	got.Cargo.Contract = 99
	got.Contracts[0].Tier = 99
	got.Items[0] = game.ItemGuardContact
	got.Posts[0] = 99
	got.Fog[0] = game.FogInSight
	got.Archive.Sight[0] = 0b111

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("mutating Player.clone()'s result reached the original:\ngot  %+v\nwant %+v", original, want)
	}
}

// TestPlayerCloneHandlesNilFields checks the all-nil/all-zero case doesn't
// panic and preserves nil.
func TestPlayerCloneHandlesNilFields(t *testing.T) {
	got := Player{}.clone()
	if !reflect.DeepEqual(got, Player{}) {
		t.Fatalf("Player{}.clone() = %+v, want the zero value", got)
	}
}

// TestMatchStateCloneIsIndependentOfOriginal is the deep-copy half of
// Resolve's own acceptance criterion (#67: "Resolve never mutates its
// argument"), tested directly against clone() rather than only indirectly
// through TestResolveDoesNotMutateInput — the stub pipeline in this issue
// never actually writes to most of these fields (Post, Market, Contracts,
// Items, Fog, Archive), so a bug in clone() aliasing any of them would not
// yet be caught by a Resolve-level test alone. TestNodeClone, TestGraphClone,
// TestPlayerClone, and TestCloneArchive elsewhere in this file already
// cover each nested clone() in isolation; this is the integration case — the whole
// MatchState, cloned once, mutated everywhere at once — proving
// MatchState.clone() actually wires all four together rather than, say,
// forgetting to call Graph.clone() at all. This mutates every nested
// slice, map, and pointer on the clone and asserts the original — compared
// against a fresh, unmutated cloneTestState() — is untouched.
func TestMatchStateCloneIsIndependentOfOriginal(t *testing.T) {
	original := cloneTestState()
	want := cloneTestState()

	got := original.clone()

	// Mutate every nested field reachable from the clone.
	got.Graph.Nodes[0].Edges[0] = 99
	got.Graph.Nodes[0].Edges = append(got.Graph.Nodes[0].Edges, 100)
	got.Graph.Nodes[0].Post.RoundsRemaining = 99
	got.Graph.Nodes[0].Market[0] = game.ItemBoltHole
	got.Graph.Cargo[0].Node = 99
	got.Graph.EventDeck[0] = 99
	got.Graph.IncidentDeck[0] = 99

	got.Players[0].Cargo.Contract = 99
	got.Players[0].Contracts[0].Tier = 99
	got.Players[0].Items[0] = game.ItemGuardContact
	got.Players[0].Posts[0] = 99
	got.Players[0].Fog[0] = game.FogInSight
	got.Players[0].Archive.Sight[0] = 0b111
	got.Players[0].Archive.Obscured[1] = 0b111
	got.Players[0].Archive.Trail[0].Round = 99
	*got.Players[0].Archive.Trail[0].Actor = 99
	*got.Players[0].Archive.Trail[0].Target = 99
	*got.Players[0].Archive.Trail[0].Item = game.ItemGuardContact

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("mutating the clone reached the original:\ngot  %+v\nwant %+v", original, want)
	}
}

// TestMatchStateCloneEqualsOriginalBeforeMutation checks clone() actually
// copies every value across — a helper that zeroed everything instead of
// aliasing it would pass the independence test above for the wrong
// reason.
func TestMatchStateCloneEqualsOriginalBeforeMutation(t *testing.T) {
	s := cloneTestState()
	got := s.clone()
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("clone() = %+v, want a value-equal copy of %+v", got, s)
	}
}

// TestCloneArchive is a focused, direct test of cloneArchive — the
// MatchState-level tests above already exercise it as part of a full
// Player clone, but game.SeatArchive is the one field in this whole type
// graph with both maps and pointer-carrying slice elements at once
// (game.TrailEntry's Actor/Target/Item), so it gets its own dedicated
// case rather than relying solely on the larger, indirect one.
func TestCloneArchive(t *testing.T) {
	actor := game.SeatID(0)
	target := game.SeatID(1)
	item := game.ItemShiv

	original := game.SeatArchive{
		Sight:    map[game.NodeID]game.RoundSet{0: 0b101, 1: 0b010},
		Obscured: map[game.NodeID]game.RoundSet{2: 0b001},
		Trail: []game.StampedTrailEntry{{
			TrailEntry: game.TrailEntry{
				Kind:   game.EventConfrontation,
				Node:   0,
				Actor:  &actor,
				Target: &target,
				Item:   &item,
			},
			Round: 1,
		}},
	}
	want := game.SeatArchive{
		Sight:    map[game.NodeID]game.RoundSet{0: 0b101, 1: 0b010},
		Obscured: map[game.NodeID]game.RoundSet{2: 0b001},
		Trail: []game.StampedTrailEntry{{
			TrailEntry: game.TrailEntry{
				Kind:   game.EventConfrontation,
				Node:   0,
				Actor:  &actor,
				Target: &target,
				Item:   &item,
			},
			Round: 1,
		}},
	}

	got := cloneArchive(original)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cloneArchive() = %+v, want a value-equal copy %+v", got, want)
	}

	// Mutate every nested map and pointer on the clone; the original must
	// not move.
	got.Sight[0] = 0b111
	got.Sight[99] = 0b1 // a key never in the original — proves the map itself is a new one
	got.Obscured[2] = 0b111
	got.Trail[0].Round = 99
	*got.Trail[0].Actor = 99
	*got.Trail[0].Target = 99
	*got.Trail[0].Item = game.ItemGuardContact

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("mutating cloneArchive's result reached the original:\ngot  %+v\nwant %+v", original, want)
	}
}

// TestCloneArchiveHandlesNilFields checks the nil case doesn't panic and
// preserves nil rather than substituting an empty map or slice (see
// TestMatchStateCloneHandlesNilFields for why that distinction matters
// here).
func TestCloneArchiveHandlesNilFields(t *testing.T) {
	got := cloneArchive(game.SeatArchive{})
	if !reflect.DeepEqual(got, game.SeatArchive{}) {
		t.Fatalf("cloneArchive(zero value) = %+v, want the zero value", got)
	}
}

// TestMatchStateCloneHandlesNilFields checks clone() does not panic on the
// zero-value/nil case for every field TestMatchStateCloneIsIndependentOfOriginal
// otherwise populates, and preserves nil rather than substituting an empty
// slice or map — both are valid Go zero values but reflect.DeepEqual
// treats nil and empty as distinct, and Snapshot() and every other
// MatchState-shaped value in this package is built and compared the same
// way.
func TestMatchStateCloneHandlesNilFields(t *testing.T) {
	s := MatchState{
		Graph:   Graph{Nodes: []Node{{}}},
		Players: []Player{{}},
	}

	got := s.clone()
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("clone() of an all-nil-fields MatchState = %+v, want %+v", got, s)
	}
}
