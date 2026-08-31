package main

import (
	"context"
	"fmt"
	"io"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/match/fold"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/orderlog"
)

// This file is issue #323: --rebuild's own orchestration. store.RebuildProjections
// (internal/store/projections.go) is the atomic delete-then-rewrite primitive;
// everything here is folding a match's own order log fresh and shaping the
// result into that primitive's inputs — the same split run.go/bundle.go
// already draw between "what the database holds" and "what a fold computes."

// statusActive is matches.status' own "races the tick" value (migration
// 00001's CHECK) — the one status --rebuild refuses without --include-active.
const statusActive = "active"

// statusFinished is the status --all rebuilds by default (issue #323:
// "Restrict to status='finished' by default").
const statusFinished = "finished"

// rebuildOne rebuilds exactly one match, named by matchID. includeActive
// allows rebuilding a match whose status is "active" — refused by default,
// named or not, since rebuilding an active match races the round tick
// (issue #323) regardless of whether it was reached via --match or --all.
func rebuildOne(ctx context.Context, s *store.Store, matchID game.MatchID, includeActive bool, stdout, stderr io.Writer) int {
	if !reportRebuild(ctx, s, matchID, includeActive, stdout, stderr) {
		return 1
	}
	return 0
}

// rebuildAll rebuilds every match whose status is "finished", plus every
// "active" match too when includeActive is set — issue #323's own scope
// rule for --all. statuses is built once and passed to ListMatchIDsByStatus
// as a single call (rather than one call per status, concatenated), so the
// result is one globally id-ordered list, not two separately-ordered lists
// stitched together. Matches are processed in that order, one at a time,
// and the first failure stops the run: each match's own rebuild is already
// atomic on its own (it cannot leave that match half-written), but there is
// no reason to keep going once one has failed, and continuing silently past
// a failure is exactly the kind of gate that "passes" having done less than
// it reported.
func rebuildAll(ctx context.Context, s *store.Store, includeActive bool, stdout, stderr io.Writer) int {
	statuses := []string{statusFinished}
	if includeActive {
		statuses = append(statuses, statusActive)
	}

	ids, err := store.New(s.Pool()).ListMatchIDsByStatus(ctx, statuses)
	if err != nil {
		logLine(stderr, "cmd/replay: list matches to rebuild: %v", err)
		return 1
	}

	if len(ids) == 0 {
		logLine(stdout, "cmd/replay: no matches to rebuild")
		return 0
	}

	for _, matchID := range ids {
		if !reportRebuild(ctx, s, matchID, includeActive, stdout, stderr) {
			return 1
		}
	}
	return 0
}

// reportRebuild is rebuildOne/rebuildAll's shared per-match body: rebuild
// matchID and print exactly one line to stdout (success) or stderr
// (failure), returning whether it succeeded. Centralising this here is what
// keeps the two callers' log wording — and any future change to it — in
// one place.
func reportRebuild(ctx context.Context, s *store.Store, matchID game.MatchID, includeActive bool, stdout, stderr io.Writer) bool {
	n, err := rebuildMatch(ctx, s, matchID, includeActive)
	if err != nil {
		logLine(stderr, "cmd/replay: rebuild match %s: %v", matchID, err)
		return false
	}
	logLine(stdout, "cmd/replay: rebuilt match %s (%d events written)", matchID, n)
	return true
}

// rebuildMatch is rebuildOne/rebuildAll's shared body: load the match and
// its order log, fold it fresh (the null-sink fold, RFC §7.4 — events are
// data here, never dispatched), and hand the result to
// store.RebuildProjections as one atomic delete-then-rewrite. It returns
// the number of events written, for the caller's own confirmation line.
//
// The status check here is not made redundant by rebuildAll's own
// ListMatchIDsByStatus filter: status can change between that listing query
// and this match's own turn in the loop, and rebuildOne's --match path has
// no prior filtering at all. It is a point-in-time check, not a lock —
// closing the remaining race against a concurrent writer is the round
// tick's own job once M4 builds it (RFC §8: "one Tick() guarded by
// SELECT ... FOR UPDATE"), the same guard every other future writer of
// these two tables will need to take too.
func rebuildMatch(ctx context.Context, s *store.Store, matchID game.MatchID, includeActive bool) (int, error) {
	seed, cfg, meta, err := s.LoadMatch(ctx, matchID)
	if err != nil {
		return 0, fmt.Errorf("load match: %w", err)
	}
	if meta.Status == statusActive && !includeActive {
		return 0, fmt.Errorf("match is active; rebuilding it races the round tick — pass --include-active to override")
	}

	rows, err := store.New(s.Pool()).ListOrdersForMatch(ctx, matchID)
	if err != nil {
		return 0, fmt.Errorf("list orders: %w", err)
	}

	log, err := orderlog.Decode(matchID, rows)
	if err != nil {
		return 0, fmt.Errorf("decode order log: %w", err)
	}

	// throughRound is the highest round actually present in the order log,
	// not cfg.Rounds: a finished match's log covers every round up to
	// cfg.Rounds and the two coincide, but an active match rebuilt with
	// --include-active may have orders for only its first few rounds, and
	// fold.Fold (== FoldThrough at cfg.Rounds) would reject that log as
	// "missing round N" for every round the match has not played yet.
	// FoldThrough itself handles an empty log (throughRound left at its
	// zero value) as the lobby case, before ever checking this bound — see
	// its own doc comment (internal/match/fold/fold.go).
	var throughRound game.RoundNumber
	for round := range log {
		if round > throughRound {
			throughRound = round
		}
	}

	players := len(meta.Seats)
	_, events, err := fold.FoldThrough(seed, cfg, players, log, throughRound)
	if err != nil {
		return 0, fmt.Errorf("fold: %w", err)
	}

	// eventsByRound groups Fold's own flat, round-major []Event by round —
	// RebuildProjections' own required shape (internal/store/projections.go),
	// since events' primary key (match_id, round, seq) assigns seq per round,
	// not across the whole match.
	eventsByRound := make(map[game.RoundNumber][]game.Event)
	for _, e := range events {
		eventsByRound[e.Round] = append(eventsByRound[e.Round], e)
	}

	// submittedByRound is match_summary's own "which seats have submitted"
	// cache (match_summary.sql's own comment), reconstructed from the order
	// log's `source` column: a seat whose order was human- or bot-sourced
	// submitted; a `default` order is the round's own fallback, standing in
	// for a seat that did not. Every round present in rows gets a map entry
	// — initialised before the source check, never only on a non-default
	// row — so a round where every seat fell back to `default` (a full
	// autopilot handover) still gets its match_summary row, with an empty
	// submitted_seats, exactly as the live round tick's own unconditional
	// per-round UpsertSummary call would have written it; a round entirely
	// absent from rows is genuinely absent from match_summary too, since it
	// never happened. rows already arrive ordered by (round, seat)
	// (ListOrdersForMatch's own ORDER BY), so appending in row order needs
	// no separate sort to land each round's seats ascending.
	submittedByRound := make(map[game.RoundNumber][]game.SeatID)
	for _, r := range rows {
		if _, ok := submittedByRound[r.Round]; !ok {
			submittedByRound[r.Round] = []game.SeatID{}
		}
		if r.Source != string(store.SourceDefault) {
			submittedByRound[r.Round] = append(submittedByRound[r.Round], r.Seat)
		}
	}

	if err := s.RebuildProjections(ctx, matchID, eventsByRound, submittedByRound); err != nil {
		return 0, fmt.Errorf("rebuild projections: %w", err)
	}

	return len(events), nil
}
