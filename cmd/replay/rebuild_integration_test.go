//go:build integration

package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
)

// This file is issue #323's real-Postgres acceptance criteria for the
// --rebuild CLI wiring itself: rebuild.go's own fold-then-write path,
// end to end through run(), plus --all's status scope rule and the
// single-match active-match guard. The atomic primitive it calls
// (store.RebuildProjections) has its own exhaustive byte-identical/
// corruption/atomicity/outbox coverage in internal/store's own integration
// suite — this file only proves the CLI wires it up correctly.

// setMatchStatus updates matchID's own status column directly — there is
// no status-transition write API yet (the round tick, M4, owns that); every
// test here that needs a match in a status CreateMatch itself never
// produces (anything but "lobby") sets it this way, the same shape
// TestClearProjectionsTouchesOnlyItsOwnMatch's neighbours use raw SQL for
// setup a store method doesn't yet exist for.
func setMatchStatus(t *testing.T, s *store.Store, matchID game.MatchID, status string) {
	t.Helper()
	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE matches SET status = $1 WHERE id = $2`, status, matchID,
	); err != nil {
		t.Fatalf("set match %s status to %q: %v", matchID, status, err)
	}
}

func countEventRows(t *testing.T, s *store.Store, matchID game.MatchID) int {
	t.Helper()
	var n int
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE match_id = $1`, matchID,
	).Scan(&n); err != nil {
		t.Fatalf("count events for match %s: %v", matchID, err)
	}
	return n
}

func hasEventKind(t *testing.T, s *store.Store, matchID game.MatchID, kind game.EventKind) bool {
	t.Helper()
	var n int
	if err := s.Pool().QueryRow(context.Background(),
		// events.kind stores the bare EventKind ordinal as a decimal string
		// (WriteEvents' own encoding, internal/store/projections.go), never
		// EventKind.String()'s display name.
		`SELECT count(*) FROM events WHERE match_id = $1 AND kind = $2`, matchID, strconv.Itoa(int(kind)),
	).Scan(&n); err != nil {
		t.Fatalf("query events by kind for match %s: %v", matchID, err)
	}
	return n > 0
}

// TestIntegrationRunRebuildMatchPopulatesEventsAndMatchSummary is #323's own happy
// path through the CLI: a finished match whose events/match_summary are
// still empty (nothing but this issue writes either table yet) gets them
// populated by --rebuild, from the same fold every other cmd/replay mode
// runs.
func TestIntegrationRunRebuildMatchPopulatesEventsAndMatchSummary(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	setMatchStatus(t, s, matchID, "finished")

	if n := countEventRows(t, s, matchID); n != 0 {
		t.Fatalf("events row count before --rebuild = %d, want 0 (nothing has written it yet)", n)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--rebuild) = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), string(matchID)) {
		t.Errorf("stdout = %q, want it to mention the rebuilt match", stdout.String())
	}

	// Fails closed: non-empty, and containing a known event kind — the same
	// EventLoitering fact TestReplayByteIdenticalAcrossRunsRealMatch already
	// verifies testFixture's idle-order log produces (LoiteringStreak == 11
	// implies the streak crossed EventLoitering's own 2-round firing point).
	if n := countEventRows(t, s, matchID); n == 0 {
		t.Fatal("events row count after --rebuild = 0, want > 0")
	}
	if !hasEventKind(t, s, matchID, game.EventLoitering) {
		t.Fatal("no EventLoitering row after --rebuild, want at least one from testFixture's idle-order log")
	}

	var summaryRounds int
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM match_summary WHERE match_id = $1`, matchID,
	).Scan(&summaryRounds); err != nil {
		t.Fatalf("count match_summary rows: %v", err)
	}
	cfg, _, _, _ := testFixture()
	if summaryRounds != cfg.Rounds {
		t.Fatalf("match_summary row count after --rebuild = %d, want %d (one per round)", summaryRounds, cfg.Rounds)
	}
}

// TestIntegrationRunRebuildAllOnlyTouchesFinishedMatchesByDefault is #323's own scope
// rule: "--all touches only status='finished' matches unless an explicit
// flag says otherwise" — asserted with one active match present.
func TestIntegrationRunRebuildAllOnlyTouchesFinishedMatchesByDefault(t *testing.T) {
	dsn, s := openReplayStore(t)
	finished := seedFullReplayMatch(t, s)
	active := seedFullReplayMatch(t, s)
	setMatchStatus(t, s, finished, "finished")
	setMatchStatus(t, s, active, "active")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--all", "--rebuild"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--all --rebuild) = %d, stderr = %s", code, stderr.String())
	}

	if n := countEventRows(t, s, finished); n == 0 {
		t.Fatal("finished match has 0 events after --all --rebuild, want it rebuilt")
	}
	if n := countEventRows(t, s, active); n != 0 {
		t.Fatalf("active match has %d events after --all --rebuild (no --include-active), want 0 (untouched)", n)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--db", dsn, "--all", "--rebuild", "--include-active"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--all --rebuild --include-active) = %d, stderr = %s", code, stderr.String())
	}
	if n := countEventRows(t, s, active); n == 0 {
		t.Fatal("active match still has 0 events after --all --rebuild --include-active, want it rebuilt")
	}
}

// TestIntegrationRunRebuildSingleActiveMatchRequiresIncludeActive asserts the same
// scope rule applies to a --match named explicitly, not only to --all —
// rebuilding an active match races the round tick regardless of how the
// match was selected (issue #323, cmd/replay/doc.go's own reasoning).
func TestIntegrationRunRebuildSingleActiveMatchRequiresIncludeActive(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	setMatchStatus(t, s, matchID, "active")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("run(--rebuild) on an active match with no --include-active succeeded, want a refusal")
	}
	if !strings.Contains(stderr.String(), "active") {
		t.Errorf("stderr = %q, want it to mention the match being active", stderr.String())
	}
	if n := countEventRows(t, s, matchID); n != 0 {
		t.Fatalf("events row count after a refused --rebuild = %d, want 0", n)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--db", dsn, "--match", string(matchID), "--rebuild", "--include-active"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--rebuild --include-active) on an active match = %d, stderr = %s", code, stderr.String())
	}
	if n := countEventRows(t, s, matchID); n == 0 {
		t.Fatal("events row count after --rebuild --include-active = 0, want > 0")
	}
}

// TestIntegrationRunRebuildActiveMatchWithPartialOrderLogSucceeds is a regression test
// for a code-review finding on this PR: rebuildMatch used to fold.Fold
// unconditionally, which folds through cfg.Rounds and errors on any round
// missing from the log — always, for a genuinely in-progress match, which
// has orders only through its current round. The fix folds through the
// order log's own highest round instead. seedFullReplayMatch's own fixture
// always covers every round, so the earlier active-match test above cannot
// catch this — this test deletes the later rounds' orders to build a
// genuinely partial log before rebuilding it.
func TestIntegrationRunRebuildActiveMatchWithPartialOrderLogSucceeds(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)

	const playedThrough = 3
	if _, err := s.Pool().Exec(context.Background(),
		`DELETE FROM orders WHERE match_id = $1 AND round > $2`, matchID, playedThrough,
	); err != nil {
		t.Fatalf("truncate order log to round %d: %v", playedThrough, err)
	}
	setMatchStatus(t, s, matchID, "active")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild", "--include-active"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--rebuild --include-active) on a match played through round %d = %d, stderr = %s", playedThrough, code, stderr.String())
	}

	var summaryRounds int
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM match_summary WHERE match_id = $1`, matchID,
	).Scan(&summaryRounds); err != nil {
		t.Fatalf("count match_summary rows: %v", err)
	}
	if summaryRounds != playedThrough {
		t.Fatalf("match_summary row count after rebuilding a partial log = %d, want %d (only the rounds actually played)", summaryRounds, playedThrough)
	}
}

// TestIntegrationRunRebuildRoundWithAllDefaultOrdersStillGetsSummaryRow is a
// regression test for a second code-review finding: submittedByRound used
// to gain a map key only for a round with at least one non-default order,
// so a round where every seat fell back to the round's default order (a
// full autopilot handover, GDD §18) silently lost its match_summary row on
// rebuild — contradicting the live round tick's own unconditional
// per-round UpsertSummary call. testFixture's own log sources every order
// as human, so this test overwrites one round's orders to "default"
// directly.
func TestIntegrationRunRebuildRoundWithAllDefaultOrdersStillGetsSummaryRow(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	cfg, _, _, _ := testFixture()

	const defaultRound = 3
	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE orders SET source = 'default' WHERE match_id = $1 AND round = $2`, matchID, defaultRound,
	); err != nil {
		t.Fatalf("mark round %d all-default: %v", defaultRound, err)
	}
	setMatchStatus(t, s, matchID, "finished")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--rebuild) = %d, stderr = %s", code, stderr.String())
	}

	var summaryRounds int
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM match_summary WHERE match_id = $1`, matchID,
	).Scan(&summaryRounds); err != nil {
		t.Fatalf("count match_summary rows: %v", err)
	}
	if summaryRounds != cfg.Rounds {
		t.Fatalf("match_summary row count after --rebuild = %d, want %d (one per round, including the all-default one)", summaryRounds, cfg.Rounds)
	}

	var submitted []game.SeatID
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT submitted_seats FROM match_summary WHERE match_id = $1 AND round = $2`, matchID, defaultRound,
	).Scan(&submitted); err != nil {
		t.Fatalf("query match_summary for round %d: %v", defaultRound, err)
	}
	if len(submitted) != 0 {
		t.Fatalf("submitted_seats for the all-default round = %v, want empty", submitted)
	}
}

// queryLastSeenRound reads back one seat's match_players.last_seen_round —
// TestIntegrationRunRebuildRecomputesLastSeenRoundAcrossAGap's own oracle read.
func queryLastSeenRound(t *testing.T, s *store.Store, matchID game.MatchID, seat game.SeatID) int32 {
	t.Helper()
	var got int32
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT last_seen_round FROM match_players WHERE match_id = $1 AND seat = $2`, matchID, seat,
	).Scan(&got); err != nil {
		t.Fatalf("query last_seen_round for seat %d: %v", seat, err)
	}
	return got
}

// TestIntegrationRunRebuildRecomputesLastSeenRoundAcrossAGap is issue #409's own
// acceptance criterion end to end: "cmd/replay --rebuild also recomputes
// match_players.last_seen_round ... over a fixture whose human-submission
// pattern is not just 'every round' ... so the LEAST clamp and a
// multi-round backlog are both actually exercised."
//
// seedFullReplayMatch's own fixture (testFixture) submits every round as a
// human order for every seat, which only exercises D52's steady-state
// formula — cursor = round-1 with no clamp ever engaged. This test starts
// from that fixture, then overwrites seat 0's rounds 2-4 to source =
// 'default' (the same technique
// TestIntegrationRunRebuildRoundWithAllDefaultOrdersStillGetsSummaryRow already uses
// to build a non-uniform source pattern), leaving seat 0 with a genuine
// 3-round gap in its human submissions while seat 1 stays at steady state —
// the two expected values below are computed by hand against RFC §7.2's own
// formula (corrected by D52), independently of lastSeenRoundsFromOrders,
// the production code under test.
func TestIntegrationRunRebuildRecomputesLastSeenRoundAcrossAGap(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	cfg, _, _, _ := testFixture()

	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE orders SET source = 'default' WHERE match_id = $1 AND seat = 0 AND round BETWEEN 2 AND 4`,
		matchID,
	); err != nil {
		t.Fatalf("open a 3-round gap in seat 0's human submissions: %v", err)
	}
	setMatchStatus(t, s, matchID, "finished")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--rebuild) = %d, stderr = %s", code, stderr.String())
	}

	// Seat 0: human rounds 1, 5, 6, ..., cfg.Rounds (2-4 skipped). Folding
	// D52's cursor = LEAST(cursor+1, round-1) from 0: round 1 -> 0; round 5
	// -> min(0+1, 4) = 1; every round after that advances the cursor by
	// exactly one more, landing cfg.Rounds-5 rounds later. For
	// cfg.Rounds = 15 that is 1 + (15-5) = 11 — three short of the 14 a
	// gap-blind rebuild (a bare MAX(round)-1, D16's rejected shortcut) would
	// produce, matching the gap's own width per D52's own invariant
	// (round-1-cursor stays constant at 3 once no further round is skipped).
	wantSeat0 := int32(1 + (cfg.Rounds - 5))
	if got := queryLastSeenRound(t, s, matchID, 0); got != wantSeat0 {
		t.Fatalf("seat 0 last_seen_round after --rebuild = %d, want %d", got, wantSeat0)
	}

	// Seat 1 never had its source touched: human every round, steady state,
	// cursor = cfg.Rounds - 1.
	wantSeat1 := int32(cfg.Rounds - 1)
	if got := queryLastSeenRound(t, s, matchID, 1); got != wantSeat1 {
		t.Fatalf("seat 1 last_seen_round after --rebuild = %d, want %d", got, wantSeat1)
	}
}

// TestIntegrationRunRebuildOverwritesStaleLastSeenRound is RebuildLastSeenRounds' own
// doc comment made concrete: unlike events/match_summary, this table is
// never cleared before being rewritten, so a stale value already on the row
// (here simulating drift from whatever corrupted it) has to be overwritten
// explicitly rather than surviving a rebuild that happens to compute the
// same 0 a fresh row would already have.
func TestIntegrationRunRebuildOverwritesStaleLastSeenRound(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	setMatchStatus(t, s, matchID, "finished")

	const stale = 12345
	if _, err := s.Pool().Exec(context.Background(),
		`UPDATE match_players SET last_seen_round = $1 WHERE match_id = $2 AND seat = 0`, stale, matchID,
	); err != nil {
		t.Fatalf("corrupt last_seen_round: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--rebuild) = %d, stderr = %s", code, stderr.String())
	}

	cfg, _, _, _ := testFixture()
	if got, want := queryLastSeenRound(t, s, matchID, 0), int32(cfg.Rounds-1); got != want {
		t.Fatalf("seat 0 last_seen_round after --rebuild = %d, want %d (the stale %d value must not survive)", got, want, stale)
	}
}

// TestIntegrationRunRebuildMatchWithEmptyOrderLogSucceeds is a regression test for a
// CodeRabbit review finding on this PR: rebuildMatch called
// fold.FoldThrough unconditionally, with throughRound left at its zero
// value for an empty order log. FoldThrough's own throughRound < 1 check
// runs before its empty-log short-circuit (fold.go), so 0 hit "round 0 is
// invalid" instead of the lobby-match short-circuit its doc comment
// describes — rebuilding a match with no orders yet always failed. The fix
// skips the fold entirely for an empty log, since there is nothing to fold.
func TestIntegrationRunRebuildMatchWithEmptyOrderLogSucceeds(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)

	if _, err := s.Pool().Exec(context.Background(),
		`DELETE FROM orders WHERE match_id = $1`, matchID,
	); err != nil {
		t.Fatalf("clear order log: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--match", string(matchID), "--rebuild"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--rebuild) on a match with an empty order log = %d, stderr = %s", code, stderr.String())
	}

	if n := countEventRows(t, s, matchID); n != 0 {
		t.Fatalf("events row count after rebuilding an empty order log = %d, want 0", n)
	}
	var summaryRounds int
	if err := s.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM match_summary WHERE match_id = $1`, matchID,
	).Scan(&summaryRounds); err != nil {
		t.Fatalf("count match_summary rows: %v", err)
	}
	if summaryRounds != 0 {
		t.Fatalf("match_summary row count after rebuilding an empty order log = %d, want 0", summaryRounds)
	}
}
