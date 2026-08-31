package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/match/fold"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/orderlog"
)

// run is main's entire implementation, factored out so it can be driven
// directly from a test with a controlled argv and captured stdout/stderr,
// instead of only through a subprocess — the same shape cmd/simulate's own
// run/runWithDeps split already uses.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbFlag := fs.String("db", "", "database connection string (e.g. $DATABASE_URL); required with --match, mutually exclusive with --bundle")
	matchFlag := fs.String("match", "", "match ID to load from the database; required with --db, mutually exclusive with --bundle")
	bundleFlag := fs.String("bundle", "", "path to a replay bundle (RFC §15.4); mutually exclusive with --db/--match")
	roundFlag := fs.Int("round", 0, "dump state as it stood after this round resolved; defaults to the match's last round")
	seatFlag := fs.Int("seat", -1, "print this seat's fog-filtered PlayerView instead of the full match state")
	exportBundleFlag := fs.String("export-bundle", "", "write a replay bundle for --match to this path and exit, instead of dumping state")
	rebuildFlag := fs.Bool("rebuild", false, "delete and regenerate events/match_summary for --match (or every finished match with --all) from a fresh fold, instead of dumping state; requires --db")
	allFlag := fs.Bool("all", false, "with --rebuild, rebuild every eligible match instead of one named by --match")
	includeActiveFlag := fs.Bool("include-active", false, "with --rebuild, also allow rebuilding a match whose status is active (default: refuse — rebuilding an active match races the round tick)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	usingBundle := *bundleFlag != ""
	usingAll := *allFlag
	usingDB := *dbFlag != "" || *matchFlag != "" || usingAll

	if usingBundle && usingDB {
		logLine(stderr, "cmd/replay: --bundle cannot be combined with --db/--match")
		return 1
	}
	if !usingBundle && !usingDB {
		logLine(stderr, "cmd/replay: either --bundle, or both --db and --match, are required")
		return 1
	}
	if usingDB && *dbFlag == "" {
		logLine(stderr, "cmd/replay: --db is required")
		return 1
	}
	if usingDB && !usingAll && *matchFlag == "" {
		logLine(stderr, "cmd/replay: --match is required unless --all is given")
		return 1
	}
	if usingAll && *matchFlag != "" {
		logLine(stderr, "cmd/replay: --all cannot be combined with --match")
		return 1
	}
	if usingAll && !*rebuildFlag {
		logLine(stderr, "cmd/replay: --all requires --rebuild")
		return 1
	}
	if *includeActiveFlag && !*rebuildFlag {
		logLine(stderr, "cmd/replay: --include-active only applies with --rebuild")
		return 1
	}
	if *rebuildFlag && usingBundle {
		logLine(stderr, "cmd/replay: --rebuild cannot be used with --bundle (rebuild writes to the database named by --db)")
		return 1
	}
	if *rebuildFlag && *exportBundleFlag != "" {
		logLine(stderr, "cmd/replay: --rebuild cannot be combined with --export-bundle")
		return 1
	}
	if *rebuildFlag && *seatFlag != -1 {
		logLine(stderr, "cmd/replay: --seat has no effect with --rebuild")
		return 1
	}
	if *rebuildFlag && *roundFlag != 0 {
		logLine(stderr, "cmd/replay: --round has no effect with --rebuild — a rebuild always regenerates the whole match")
		return 1
	}
	if *exportBundleFlag != "" && !usingDB {
		logLine(stderr, "cmd/replay: --export-bundle requires --db and --match")
		return 1
	}
	if *roundFlag != 0 && *roundFlag < 1 {
		logLine(stderr, "cmd/replay: --round must be >= 1")
		return 1
	}

	ctx := context.Background()

	if *rebuildFlag {
		s, err := store.Open(ctx, store.Config{DSN: *dbFlag})
		if err != nil {
			logLine(stderr, "cmd/replay: connect: %v", err)
			return 1
		}
		defer s.Close()

		if usingAll {
			return rebuildAll(ctx, s, *includeActiveFlag, stdout, stderr)
		}
		return rebuildOne(ctx, s, game.MatchID(*matchFlag), *includeActiveFlag, stdout, stderr)
	}

	if usingBundle {
		seed, cfg, log, players, err := readBundle(*bundleFlag)
		if err != nil {
			logLine(stderr, "cmd/replay: %v", err)
			return 1
		}
		return foldAndDump(stdout, stderr, seed, cfg, players, log, *roundFlag, *seatFlag)
	}

	s, err := store.Open(ctx, store.Config{DSN: *dbFlag})
	if err != nil {
		logLine(stderr, "cmd/replay: connect: %v", err)
		return 1
	}
	defer s.Close()

	matchID := game.MatchID(*matchFlag)

	if *exportBundleFlag != "" {
		b, err := exportBundleFromDB(ctx, s.Pool(), matchID)
		if err != nil {
			logLine(stderr, "cmd/replay: %v", err)
			return 1
		}
		if err := writeBundle(*exportBundleFlag, b); err != nil {
			logLine(stderr, "cmd/replay: %v", err)
			return 1
		}
		return 0
	}

	seed, cfg, meta, err := s.LoadMatch(ctx, matchID)
	if err != nil {
		logLine(stderr, "cmd/replay: load match %s: %v", matchID, err)
		return 1
	}
	players := len(meta.Seats)

	log, err := orderlog.Load(ctx, s.Pool(), matchID)
	if err != nil {
		logLine(stderr, "cmd/replay: load order log for match %s: %v", matchID, err)
		return 1
	}

	return foldAndDump(stdout, stderr, seed, cfg, players, log, *roundFlag, *seatFlag)
}

// foldAndDump runs FoldThrough — the same fold, with events discarded, per
// RFC §7.4's null sink — and prints exactly one of two things: the full
// rules.MatchState (the default, a developer's view — see cmd/replay/doc.go
// for the build-tag question this answers), or one seat's fog-filtered
// game.PlayerView when --seat names one. round == 0 means "unset": fold
// through the match's own last round, cfg.Rounds.
func foldAndDump(stdout, stderr io.Writer, seed [32]byte, cfg game.Config, players int, log rules.OrderLog, round, seat int) int {
	// -1 is the documented "no seat" default (full-state dump, below).
	// Anything lower than that is not a meaningful seat value and must be
	// rejected here rather than falling through to the `else` branch below,
	// which would otherwise silently emit the full, un-fogged MatchState for
	// an explicitly invalid --seat instead of erroring.
	if seat < -1 {
		logLine(stderr, "cmd/replay: --seat must be >= -1")
		return 1
	}

	throughRound := game.RoundNumber(cfg.Rounds)
	if round != 0 {
		throughRound = game.RoundNumber(round)
	}

	state, _, err := fold.FoldThrough(seed, cfg, players, log, throughRound)
	if err != nil {
		logLine(stderr, "cmd/replay: fold: %v", err)
		return 1
	}

	var payload any
	if seat >= 0 {
		if seat >= players {
			logLine(stderr, "cmd/replay: --seat %d is out of range for a %d-player match", seat, players)
			return 1
		}
		payload = rules.ProjectView(state, game.SeatID(seat), cfg)
	} else {
		payload = state
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		logLine(stderr, "cmd/replay: encode dump: %v", err)
		return 1
	}
	data = append(data, '\n')
	if _, err := stdout.Write(data); err != nil {
		logLine(stderr, "cmd/replay: write dump: %v", err)
		return 1
	}
	return 0
}

// logLine writes one diagnostic line to w, discarding the write error
// deliberately — a failure to write a stderr line isn't actionable here,
// and every call site already reports its own error (or none) through
// run's return code, not through this line landing. Mirrors cmd/simulate's
// own logLine (run.go).
func logLine(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
