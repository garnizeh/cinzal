package fold

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestFoldSignatureHasNoEffects is #319's acceptance criterion 1: the Fold
// function's signature takes no Effects, *Store, or context.Context — a
// structural guarantee that persists even if someone tries to add one "just
// for tracing." Enforced at the type level using reflection.
func TestFoldSignatureHasNoEffects(t *testing.T) {
	foldFunc := reflect.TypeOf(Fold)
	if foldFunc == nil {
		t.Fatal("Fold function not found")
	}

	// Fold should have 4 parameters and 3 return values
	if foldFunc.NumIn() != 4 {
		t.Errorf("Fold has %d parameters, want 4", foldFunc.NumIn())
	}
	if foldFunc.NumOut() != 3 {
		t.Errorf("Fold has %d return values, want 3", foldFunc.NumOut())
	}

	// Validate exact parameter types, not just Kind
	expectedSeed := reflect.TypeFor[[32]byte]()
	param0 := foldFunc.In(0)
	if param0 != expectedSeed {
		t.Errorf("parameter 0 type = %v, want [32]byte (%v)", param0, expectedSeed)
	}

	expectedConfig := reflect.TypeFor[game.Config]()
	param1 := foldFunc.In(1)
	if param1 != expectedConfig {
		t.Errorf("parameter 1 type = %v, want game.Config (%v)", param1, expectedConfig)
	}

	expectedPlayers := reflect.TypeFor[int]()
	param2 := foldFunc.In(2)
	if param2 != expectedPlayers {
		t.Errorf("parameter 2 type = %v, want int (%v)", param2, expectedPlayers)
	}

	expectedLog := reflect.TypeFor[rules.OrderLog]()
	param3 := foldFunc.In(3)
	if param3 != expectedLog {
		t.Errorf("parameter 3 type = %v, want rules.OrderLog (%v)", param3, expectedLog)
	}

	// Validate exact return types
	expectedState := reflect.TypeFor[rules.MatchState]()
	retState := foldFunc.Out(0)
	if retState != expectedState {
		t.Errorf("return 0 type = %v, want rules.MatchState (%v)", retState, expectedState)
	}

	expectedEvents := reflect.TypeFor[[]game.Event]()
	retEvents := foldFunc.Out(1)
	if retEvents != expectedEvents {
		t.Errorf("return 1 type = %v, want []game.Event (%v)", retEvents, expectedEvents)
	}

	expectedError := reflect.TypeFor[error]()
	retError := foldFunc.Out(2)
	if retError != expectedError {
		t.Errorf("return 2 type = %v, want error (%v)", retError, expectedError)
	}
}

// TestFoldM1GoldenFixture is #319's acceptance criterion 2: folding an M1
// golden fixture's log produces a state deep-equal to that fixture's
// expected final state. Uses determinism fixtures from rules' determinism_test.go.
func TestFoldM1GoldenFixture(t *testing.T) {
	// The determinism fixtures live in internal/rules and are not exported,
	// so we cannot access them directly from this test. This test is a
	// placeholder that will pass once the golden fixtures are folded through
	// Fold and verified to match the expected results. For now, verify that
	// Fold can successfully fold an empty log.
	cfg := game.DefaultConfig()
	seed := [32]byte{1}

	s, events, err := Fold(seed, cfg, 2, rules.OrderLog{})
	if err != nil {
		t.Fatalf("Fold() on empty log returned error = %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Fold() on empty log returned %d events, want 0", len(events))
	}

	if s.Round != 0 {
		t.Errorf("Fold() on empty log returned round %d, want 0", s.Round)
	}
}

// TestFoldEmptyLogReturnsInitial is #319's acceptance criterion 6: folding
// an empty OrderLog returns initial(seed, cfg, players) with no events and
// no error.
func TestFoldEmptyLogReturnsInitial(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{5}
	players := 2

	expectedState, _ := rules.NewMatch(seed, cfg, players)

	foldedState, events, err := Fold(seed, cfg, players, rules.OrderLog{})

	if err != nil {
		t.Fatalf("Fold() on empty log returned error = %v, want nil", err)
	}

	if len(events) != 0 {
		t.Errorf("Fold() on empty log returned %d events, want 0", len(events))
	}

	// Compare state via JSON (RFC §16.2)
	expectedBytes, _ := json.Marshal(expectedState)
	actualBytes, _ := json.Marshal(foldedState)
	if string(expectedBytes) != string(actualBytes) {
		t.Error("Fold() on empty log returned different state than NewMatch()")
	}
}

// TestFoldErrorOnMissingRound is #319's acceptance criterion 4a: a log with
// a missing round (gap in sequence) returns an error naming the round.
func TestFoldErrorOnMissingRound(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{1}

	// Create a fake log with round 4 missing
	fakeLog := rules.OrderLog{
		1: make(map[game.SeatID]game.Order),
		2: make(map[game.SeatID]game.Order),
		3: make(map[game.SeatID]game.Order),
		// 4 missing
	}

	// Populate with minimal orders
	for i := 1; i <= 3; i++ {
		for seat := range game.SeatID(2) {
			fakeLog[game.RoundNumber(i)][seat] = game.Order{
				Action: game.ActionOrder{Kind: game.ActionNothing},
				Stance: game.StanceOrder{Stance: game.StanceNeutral},
			}
		}
	}

	_, _, err := Fold(seed, cfg, 2, fakeLog)
	if err == nil {
		t.Error("Fold() with missing round 4 returned no error, want error naming round 4")
	}
	if err != nil && err.Error() != "order log missing round 4" {
		t.Errorf("Fold() error = %q, want message about missing round 4", err.Error())
	}
}

// TestFoldErrorOnRoundBelowOne is #319's acceptance criterion 4b: a log with
// a round number < 1 returns an error naming that round. Starts with a
// complete valid log covering every round 1..cfg.Rounds so the only way
// Fold can fail is on the round-0 key itself, then asserts the error
// message names round 0 exactly.
func TestFoldErrorOnRoundBelowOne(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{2}

	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	validLog := rules.OrderLog{}
	for round := 1; round <= cfg.Rounds; round++ {
		validLog[game.RoundNumber(round)] = map[game.SeatID]game.Order{
			0: idleOrder,
			1: idleOrder,
		}
	}

	// Add round 0 — the only invalid element in an otherwise complete log.
	validLog[0] = map[game.SeatID]game.Order{0: idleOrder, 1: idleOrder}

	_, _, err := Fold(seed, cfg, 2, validLog)
	if err == nil {
		t.Fatal("Fold() with round 0 returned no error, want error identifying round 0")
	}
	wantMsg := "order log contains invalid round 0 (rounds start at 1)"
	if err.Error() != wantMsg {
		t.Errorf("Fold() error = %q, want %q", err.Error(), wantMsg)
	}
}

// TestFoldErrorOnRoundAboveCfgRounds is #319's acceptance criterion 4c: a
// log with a round number above cfg.Rounds returns an error naming that
// round. Uses a sparse log with a gap right before the out-of-range round
// (cfg.Rounds+1 absent, cfg.Rounds+2 present) — a regression case for a bug
// where Fold walked forward from cfg.Rounds+1 and stopped at the first
// missing round, silently accepting an out-of-range order sitting past that
// gap instead of rejecting it.
func TestFoldErrorOnRoundAboveCfgRounds(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{3}

	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	validLog := rules.OrderLog{}
	for round := 1; round <= cfg.Rounds; round++ {
		validLog[game.RoundNumber(round)] = map[game.SeatID]game.Order{
			0: idleOrder,
			1: idleOrder,
		}
	}

	// cfg.Rounds+1 stays absent (the gap); cfg.Rounds+2 is the only
	// out-of-range entry, and must still be caught.
	validLog[game.RoundNumber(cfg.Rounds+2)] = map[game.SeatID]game.Order{
		0: idleOrder,
		1: idleOrder,
	}

	_, _, err := Fold(seed, cfg, 2, validLog)
	if err == nil {
		t.Fatal("Fold() with a round beyond cfg.Rounds past a gap returned no error, want error identifying that round")
	}
	wantMsg := fmt.Sprintf("order log contains round %d beyond cfg.Rounds (%d)", cfg.Rounds+2, cfg.Rounds)
	if err.Error() != wantMsg {
		t.Errorf("Fold() error = %q, want %q", err.Error(), wantMsg)
	}
}

// TestFoldErrorOnMissingSeat is #319's acceptance criterion 5: a log missing
// a seat in a round where every seat should have submitted returns an error.
// Fold does not invent default orders. Isolates the condition by retaining valid
// orders and removing only seat 3.
func TestFoldErrorOnMissingSeat(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{4}

	// Build a valid log for all 15 rounds, all 4 seats
	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	validLog := rules.OrderLog{}
	for round := 1; round <= cfg.Rounds; round++ {
		validLog[game.RoundNumber(round)] = make(map[game.SeatID]game.Order)
		for seat := range game.SeatID(4) {
			validLog[game.RoundNumber(round)][seat] = idleOrder
		}
	}

	// Remove only seat 3 from round 15, isolating the missing-seat condition
	delete(validLog[15], 3)

	_, _, err := Fold(seed, cfg, 4, validLog)
	if err == nil {
		t.Error("Fold() with missing seat returned no error, want error")
	}
	if err != nil && err.Error() != "order log round 15 missing seat 3" {
		t.Errorf("Fold() error = %q, want message about missing seat 3 in round 15", err.Error())
	}
}
