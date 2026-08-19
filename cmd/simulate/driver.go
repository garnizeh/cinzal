package main

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/telemetry"
)

// MatchResult is one headless match's complete outcome: its final state,
// its full event stream, the order log every seat's order was recorded
// into (RFC §7.1's fold input, not reconstructible any other way once
// haltMovement has cleared a route from the live copy — see
// rules.OrderLog's own doc comment), GDD §16's final scoring, and the GDD
// §22 per-match metric set this whole command exists to produce
// (doc.go) — computed once here, from exactly the (Final, Log, Events,
// cfg) this driver already assembled, rather than left for a later pass
// to recompute from a result that no longer carries what it needs to.
type MatchResult struct {
	Seed    [32]byte
	Players int

	Final   rules.MatchState
	Events  []game.Event
	Log     rules.OrderLog
	Scores  []rules.FinalScoreBreakdown
	Summary telemetry.MatchSummary
}

// RunMatch runs one match headlessly, from seed and cfg, every seat played
// by bot: state = fold(Resolve, initial(seed, cfg), orderLog) (RFC §7.1),
// assembling each seat's view with rules.ProjectView (D27's post-fill,
// promoted here to the one exported helper this driver and internal/match
// both call — see that function's own doc comment) before handing it to
// bot.Decide.
//
// The RNG threading follows D32: Resolve draws from its own *RNG
// (rules.NewRNG(seed, round)), Decide draws from a distinct BotRNG
// (rules.NewBotRNG(seed, seat, round)) — the two streams never mix, and
// this is the only place they meet.
//
// RunMatch fails closed (issue #199's own acceptance criterion): it
// returns an error, never a partial result, when the match does not reach
// cfg.Rounds, when a round did not collect exactly one order per seat, or
// when the recorded order log does not hold exactly cfg.Rounds*players
// orders in total.
func RunMatch(seed [32]byte, cfg game.Config, players int, bot bots.Bot) (MatchResult, error) {
	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		return MatchResult{}, fmt.Errorf("cmd/simulate: RunMatch: %w", err)
	}

	log := make(rules.OrderLog, cfg.Rounds)
	var events []game.Event

	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := game.SeatID(0); int(seat) < players; seat++ {
			v := rules.ProjectView(s, seat, cfg)
			orders[seat] = bot.Decide(v, cfg, rules.NewBotRNG(seed, seat, round))
		}
		if len(orders) != players {
			return MatchResult{}, fmt.Errorf("cmd/simulate: RunMatch: round %d: %d seats produced an order, want %d", round, len(orders), players)
		}
		log[game.RoundNumber(round)] = orders

		next, roundEvents, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
		if err != nil {
			return MatchResult{}, fmt.Errorf("cmd/simulate: RunMatch: round %d: Resolve: %w", round, err)
		}
		s = next
		events = append(events, roundEvents...)
	}

	if err := validateComplete(s, log, cfg, players); err != nil {
		return MatchResult{}, fmt.Errorf("cmd/simulate: RunMatch: %w", err)
	}

	summary, err := telemetry.Match(s, log, events, cfg)
	if err != nil {
		return MatchResult{}, fmt.Errorf("cmd/simulate: RunMatch: telemetry.Match: %w", err)
	}

	return MatchResult{
		Seed:    seed,
		Players: players,
		Final:   s,
		Events:  events,
		Log:     log,
		Scores:  rules.FinalScore(s),
		Summary: summary,
	}, nil
}

// validateComplete is RunMatch's fails-closed check, split out as its own
// pure function so it is unit-testable directly against synthetic
// (state, log) pairs, without having to coax a full match into an
// incomplete shape through real Resolve calls — the shape that acceptance
// criterion has to check is, by construction, unreachable from RunMatch's
// own loop.
func validateComplete(s rules.MatchState, log rules.OrderLog, cfg game.Config, players int) error {
	if s.Round != game.RoundNumber(cfg.Rounds) {
		return fmt.Errorf("match reached round %d, cfg.Rounds is %d — the match did not finish", s.Round, cfg.Rounds)
	}
	if len(log) != cfg.Rounds {
		return fmt.Errorf("order log holds %d round(s), want %d", len(log), cfg.Rounds)
	}

	total := 0
	for round, orders := range log {
		if len(orders) != players {
			return fmt.Errorf("round %d holds %d order(s), want %d (one per seat)", round, len(orders), players)
		}
		total += len(orders)
	}
	if want := cfg.Rounds * players; total != want {
		return fmt.Errorf("order log holds %d order(s) total, want %d (cfg.Rounds x players)", total, want)
	}

	return nil
}

// RunMany runs the same (cfg, players, bot) configuration across many
// seeds, in parallel across DIFFERENT matches — never inside one. RFC §6.3
// hazard 4 is explicit that parallelising a single round is never worth
// the nondeterminism risk; running many independent matches concurrently
// carries none of that risk, because each one owns its own MatchState from
// rules.NewMatch onward and touches nothing another match's goroutine can
// see.
//
// workers bounds how many matches run concurrently; workers <= 0 defaults
// to runtime.GOMAXPROCS(0). Regardless of workers, results are returned in
// seeds' own order, not completion order — a run with workers=1 and a run
// with workers=8 over the same seeds produce byte-identical output in
// identical order (issue #199's own acceptance criterion, stated there as
// GOMAXPROCS=1 vs GOMAXPROCS=8; workers is this package's own knob for the
// same property, independent of the process-wide GOMAXPROCS setting).
//
// RunMany fails closed: the first seed (by seed-set order, not completion
// order) whose RunMatch call errored is returned, wrapped with its index;
// no partial []MatchResult is returned alongside an error.
func RunMany(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(seeds) {
		workers = len(seeds)
	}

	results := make([]MatchResult, len(seeds))
	errs := make([]error, len(seeds))

	indices := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for i := range indices {
				results[i], errs[i] = RunMatch(seeds[i], cfg, players, bot)
			}
		}()
	}
	for i := range seeds {
		indices <- i
	}
	close(indices)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("cmd/simulate: RunMany: seed index %d: %w", i, err)
		}
	}
	return results, nil
}
