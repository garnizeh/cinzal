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
