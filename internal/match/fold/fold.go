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
//
// Fold is FoldThrough bounded at cfg.Rounds — "the whole match" is simply
// "through its own last round." See FoldThrough's own doc for why the round
// bound and cfg itself (with cfg.Rounds unchanged) have to travel separately:
// Resolve, Legal and the incident/market/add-on rules all branch on
// int(round) >= cfg.Rounds for final-round behaviour (Ledger's final-round
// reject, the last market refresh, the last Unstable Sector draw, whether
// Phase 2/3 stage a round that will never be played) — so cfg.Rounds must
// stay the match's true length even when a caller only wants an
// intermediate round's state, or that intermediate dump would silently
// disagree with what the live match actually did at that round.
func Fold(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) (rules.MatchState, []game.Event, error) {
	return FoldThrough(seed, cfg, players, log, game.RoundNumber(cfg.Rounds))
}

// FoldThrough is Fold's own loop, parameterised by how far to fold rather
// than always running to cfg.Rounds. It exists for cmd/replay's --round N
// (issue #322): "dump state at round N" needs the state as it stood after
// round N resolved, for any finished match's N < cfg.Rounds, not only the
// terminal state Fold itself returns.
//
// throughRound is the last round to resolve, not a second cfg.Rounds — cfg
// is passed through unmodified, and every final-round-gated rule inside
// Resolve keeps comparing against the match's real cfg.Rounds throughout,
// exactly as it would resolving the whole match. Substituting a shortened
// cfg (Rounds: int(throughRound)) instead of adding this parameter would
// silently fire the final-round branches early — prepareNextRound would
// stop staging Phase 2/3 for round N+1, nextUnstableSector would return nil
// a round early, the Ledger's final-round reject would trip on a round that
// is not actually final — producing a dump that is not what the real match
// looked like at round N. This is why the null sink is still "the same
// fold," not a second implementation: the round-by-round Resolve loop below
// is Fold's own, run to a caller-chosen bound instead of always to the end.
//
// throughRound must be in [1, cfg.Rounds]; 0 (an empty log's "no rounds
// played yet") is handled separately, above the loop, matching Fold's own
// empty-log short-circuit. throughRound > cfg.Rounds is rejected naming
// cfg.Rounds — the match's actual last round — rather than silently
// clamping, per #322's acceptance criterion.
func FoldThrough(seed [32]byte, cfg game.Config, players int, log rules.OrderLog, throughRound game.RoundNumber) (rules.MatchState, []game.Event, error) {
	// Validate config
	if err := cfg.Validate(players); err != nil {
		return rules.MatchState{}, nil, err
	}

	if throughRound < 1 {
		return rules.MatchState{}, nil, fmt.Errorf("fold: round %d is invalid (rounds start at 1)", throughRound)
	}
	if int(throughRound) > cfg.Rounds {
		return rules.MatchState{}, nil, fmt.Errorf("fold: round %d is beyond the match's last round (%d)", throughRound, cfg.Rounds)
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
	// named explicitly rather than silently ignored by the 1..throughRound
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
	// the round<1 scan above. Checked against cfg.Rounds, not throughRound —
	// this validates the log's own structural well-formedness, independent
	// of how far this particular call folds it.
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

	// Accumulate events across every round folded
	var allEvents []game.Event

	// Get the list of expected seats (0..players-1)
	expectedSeats := make([]game.SeatID, players)
	for i := 0; i < players; i++ {
		expectedSeats[i] = game.SeatID(i)
	}

	// Loop through rounds 1..throughRound. cfg travels unmodified into every
	// Resolve call — see this function's own doc for why that matters.
	for round := game.RoundNumber(1); round <= throughRound; round++ {
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
		// Empty events from a valid fold through at least one round is
		// suspicious — if we ran any rounds, we should have events. If the
		// log was empty, we already returned above. This is a sign of a
		// broken log that silently folded to round 0.
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
// imports this package for exactly this function, to keep the production
// dashboard's duration/allocation-share population scoped to real
// gameplay. cmd/replay (#322) imports this package too, but calls Fold and
// FoldThrough directly rather than FoldMeasured: an ad hoc replay run —
// from a developer's laptop, at any time, against any match, possibly
// re-run many times over the same match while debugging — is not a live
// fold this dashboard's own "what does a real match on this deployment
// cost" question is about, and Observe-ing one would mix that population
// with samples that have nothing to do with it. cmd/simulate cannot import
// internal/match/fold at all — the simulate-dependency gate (#199)
// restricts it to rules, bots, game, telemetry, opsmetrics — so it calls
// opsmetrics.Default.Observe directly around its own per-match sequence of
// Resolve calls instead (D45); see cmd/simulate/driver.go's RunMatch.
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
