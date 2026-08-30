package fold

import (
	"fmt"
	"time"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
	"github.com/garnizeh/cinzal/internal/rules"
)

// Fold is state = fold(Resolve, initial(seed, cfg), orderLog) (RFC §7.1).
// It reconstructs the complete match state by replaying all resolved rounds
// from a seed, config, and order log. Fold is pure — it takes no Effects, no
// *Store, no context.Context — and returns events as data; the caller
// decides what to do with them (RFC §7.4).
//
// Fold validates the log structure on entry: missing rounds, invalid round
// numbers, and missing seats all return errors. An empty log returns
// initial(seed, cfg, players) with no events and no error. Folding the same
// {seed, cfg, log} twice produces byte-identical state and event sequence
// (RFC §16.2), asserted by tests comparing JSON serializations.
//
// Fold lives in this subpackage, not in top-level internal/match, to keep
// internal/match's exported API free of rules.MatchState types (D49). The
// parent package imports this child only for its own internal tick; cmd/replay
// may import it directly for its own needs.
func Fold(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) (rules.MatchState, []game.Event, error) {
	// Validate config
	if err := cfg.Validate(players); err != nil {
		return rules.MatchState{}, nil, err
	}

	// Initialize state from seed and config
	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		return rules.MatchState{}, nil, err
	}

	// Handle empty log — match in lobby is a legitimate state
	if len(log) == 0 {
		return s, []game.Event{}, nil
	}

	// Reject any round number below 1 in the log — a round key of 0 or
	// negative is structurally invalid, not merely absent, and must be
	// named explicitly rather than silently ignored by the 1..cfg.Rounds
	// loop below (#319 acceptance criterion: "a round number below 1...
	// returns an error naming the round"). Scanned by lowest offending
	// round, never map iteration order, so the reported round is
	// deterministic (RFC §6.3: no map-range order).
	invalidFound := false
	var lowestInvalid game.RoundNumber
	for round := range log {
		if round < 1 && (!invalidFound || round < lowestInvalid) {
			invalidFound = true
			lowestInvalid = round
		}
	}
	if invalidFound {
		return rules.MatchState{}, nil, fmt.Errorf("order log contains invalid round %d (rounds start at 1)", lowestInvalid)
	}

	// Reject any round number above cfg.Rounds in the log, scanning every
	// key rather than walking forward from cfg.Rounds+1 and stopping at the
	// first gap — a sparse log (e.g. only cfg.Rounds+2 present, cfg.Rounds+1
	// absent) would otherwise slip past that walk and have its out-of-range
	// order silently dropped. Same deterministic-lowest-round selection as
	// the round<1 scan above.
	beyondFound := false
	var lowestBeyond game.RoundNumber
	for round := range log {
		if round > game.RoundNumber(cfg.Rounds) && (!beyondFound || round < lowestBeyond) {
			beyondFound = true
			lowestBeyond = round
		}
	}
	if beyondFound {
		return rules.MatchState{}, nil, fmt.Errorf("order log contains round %d beyond cfg.Rounds (%d)", lowestBeyond, cfg.Rounds)
	}

	// Accumulate events across all rounds
	var allEvents []game.Event

	// Get the list of expected seats (0..players-1)
	expectedSeats := make([]game.SeatID, players)
	for i := 0; i < players; i++ {
		expectedSeats[i] = game.SeatID(i)
	}

	// Loop through rounds 1..cfg.Rounds
	for round := game.RoundNumber(1); round <= game.RoundNumber(cfg.Rounds); round++ {
		// Check if round exists in log
		orders, hasRound := log[round]
		if !hasRound {
			// A log with a missing round is an error — fold does not skip
			return rules.MatchState{}, nil, fmt.Errorf("order log missing round %d", round)
		}

		// Check that all expected seats are present in the orders for this round
		// (Fold enforces completeness; Resolve handles live absences with defaults)
		for _, seat := range expectedSeats {
			if _, hasSeat := orders[seat]; !hasSeat {
				return rules.MatchState{}, nil, fmt.Errorf("order log round %d missing seat %d", round, seat)
			}
		}

		// Resolve this round with its orders
		var events []game.Event
		s, events, err = rules.Resolve(s, orders, cfg, rules.NewRNG(seed, int(round)))
		if err != nil {
			return rules.MatchState{}, nil, err
		}

		allEvents = append(allEvents, events...)
	}

	// Fails closed: assert that we have actual results, not a pathological log
	// that returned zero state. The test will fail if allEvents is empty or
	// state.Round is 0 when it should not be.
	if len(allEvents) == 0 {
		// Empty events from a valid full fold is suspicious — if we ran 15
		// rounds, we should have events. If the log was empty, we already
		// returned above. This is a sign of a broken log that silently
		// folded to round 0.
		return rules.MatchState{}, nil, fmt.Errorf("fold produced no events (log may be truncated or empty)")
	}

	return s, allEvents, nil
}

// FoldMeasured wraps Fold with RFC-001 §7.3's own instrumentation (D45): a
// timer covering everything Fold does — before it is invoked to when it
// returns, so initial() and every Resolve call in the fold count as one
// number, matching what §7.3's own arithmetic table treats as one fold and
// what a caller actually experiences as latency — and, on success, one
// Observe call recording that duration plus the estimated allocation into
// opsmetrics.Default.
//
// FoldMeasured lives here rather than in top-level internal/match for the
// same reason Fold does (D49): it returns a rules.MatchState, and
// internal/match's own exported surface must stay free of that type so the
// fog gate's direct-import check (scripts/check-fog-boundary.sh) stays
// congruent with the property it enforces. internal/match's own tick (M4)
// imports this package for exactly this function; cmd/replay will too, once
// #322 builds it. cmd/simulate cannot import internal/match/fold at all —
// the simulate-dependency gate (#199) restricts it to rules, bots, game,
// telemetry, opsmetrics — so it calls opsmetrics.Default.Observe directly
// around its own per-match sequence of Resolve calls instead (D45); see
// cmd/simulate/driver.go's RunMatch.
//
// A failed fold records nothing: an error means there is no duration or
// allocation figure worth attributing to "one fold," and Observe-ing a
// number for a fold that never completed would silently inflate the
// duration/allocation-share statistics with failure-path noise no operator
// asked to see.
func FoldMeasured(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) (rules.MatchState, []game.Event, error) {
	start := time.Now()
	s, events, err := Fold(seed, cfg, players, log)
	if err == nil {
		opsmetrics.Default.Observe(time.Since(start), opsmetrics.EstimateFoldBytes(len(log)))
	}
	return s, events, err
}
