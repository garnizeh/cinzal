package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is issue #321: the writers for the two derived projections
// (RFC-001 §7.1-7.3) and the discipline that keeps them derived — "these are
// caches with no invalidation problem, because they are never read as
// truth." WriteEvents and UpsertSummary are the round tick's own write path
// (M4, immediately after Resolve() returns); ClearProjections is
// cmd/replay --rebuild's first step (M4), clearing both tables for one
// match before a fresh fold regenerates them identically.
//
// Nothing in this file reads events or match_summary — see
// projections_test.go's TestProjectionReadsAreAllowlisted for the source-
// level test that keeps it that way: any future SELECT against either table
// has to be added to testdata/projections-read-allowlist.txt, in a reviewed
// diff naming its caller, the same discipline
// scripts/bots-isolation-allowlist.txt already holds internal/bots to.

// WriteEvents inserts round's events for matchID in exactly Resolve's own
// order (RFC-001 §7.1: "state = fold(Resolve, initial(seed, cfg),
// orderLog)" — events is that fold's own trace, materialised). seq is the
// event's index within events, not a database sequence: a SERIAL would let a
// --rebuild produce different sequence numbers than the original write,
// which breaks byte-identical replay before that acceptance criterion even
// starts. Calling WriteEvents twice for the same (matchID, round) without an
// intervening ClearProjections violates events' primary key
// (match_id, round, seq) and returns the resulting unique-violation error —
// the tick's own caller is responsible for calling ClearProjections first on
// any re-run (a --rebuild), never this function.
//
// events.kind stores EventKind's own iota ordinal, as a bare decimal
// string — never game.EventKind.String()'s display name, which is free to
// change for debug/UI text and is not the wire contract here. The repository
// already carries the rule that a new EventKind constant must be appended at
// the end of the iota block (internal/game/event.go): a mid-block insertion
// shifts every later ordinal, and since this column stores that ordinal
// directly, a reordering silently relabels every historical events.kind row
// already written, with no error anywhere. events is rebuildable — never
// read as authority (this file's own header comment) — so the only recovery
// this hazard needs is "run cmd/replay --rebuild over every match," and that
// recovery is what makes the bare integer affordable here in the first
// place, unlike orders.payload's enum fields, which D44/D47 encode as a
// frozen string table specifically because orders.payload has no such
// rebuild to fall back on.
func (s *Store) WriteEvents(ctx context.Context, matchID game.MatchID, round game.RoundNumber, events []game.Event) error {
	if len(events) == 0 {
		return nil
	}

	params := make([]InsertEventsBatchParams, len(events))
	for i, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("store: write events (match %s, round %d, seq %d): encode payload: %w", matchID, round, i, err)
		}
		params[i] = InsertEventsBatchParams{
			MatchID: matchID,
			Round:   round,
			Seq:     int32(i),
			Kind:    strconv.Itoa(int(e.Kind)),
			Payload: payload,
		}
	}

	// InsertEventsBatch.Exec's own doc (batch.go) closes the underlying
	// pgx.BatchResults itself once the callback loop finishes; nothing here
	// closes it again.
	var firstErr error
	New(s.pool).InsertEventsBatch(ctx, params).Exec(func(i int, err error) {
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("store: write events (match %s, round %d, seq %d): %w", matchID, round, i, err)
		}
	})
	return firstErr
}

// UpsertSummary writes match's round-scoped "which seats have submitted"
// cache (RFC-001 §7.2's match_summary — roster data, not the game view; see
// queries/match_summary.sql's own comment). Idempotent for a (match_id,
// round) pair: a second call for the same pair replaces submitted rather
// than adding a row, and updated_at always advances to the call's own write
// time — UpsertMatchSummary's ON CONFLICT ... SET updated_at = excluded.*
// takes updated_at's DEFAULT now() from the row Postgres would have
// inserted, not the row already on disk, so every call — insert or
// conflict — gets a fresh timestamp.
func (s *Store) UpsertSummary(ctx context.Context, matchID game.MatchID, round game.RoundNumber, submitted []game.SeatID) error {
	_, err := New(s.pool).UpsertMatchSummary(ctx, UpsertMatchSummaryParams{
		MatchID:        matchID,
		Round:          round,
		SubmittedSeats: submitted,
	})
	if err != nil {
		return fmt.Errorf("store: upsert summary (match %s, round %d): %w", matchID, round, err)
	}
	return nil
}

// ClearProjections removes every events and match_summary row for matchID —
// cmd/replay --rebuild's first step (M4) — and touches nothing else: no
// other match's rows, and no other table. Both deletes run inside one
// transaction (the same shape CreateMatch's own match+seats write already
// uses, matches.go) so a failure partway — a lost connection between the
// two deletes — never leaves one table cleared and the other still holding
// the match's stale projection.
func (s *Store) ClearProjections(ctx context.Context, matchID game.MatchID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: clear projections (match %s): begin transaction: %w", matchID, err)
	}
	// Rollback after a successful Commit is a documented no-op (pgx.Tx); the
	// error is discarded deliberately, matching CreateMatch's own precedent
	// (matches.go) — nothing actionable to do with a rollback failing after
	// the transaction is already resolved either way.
	defer func() { _ = tx.Rollback(ctx) }()

	q := New(tx)
	if err := q.DeleteEventsByMatch(ctx, matchID); err != nil {
		return fmt.Errorf("store: clear projections (match %s): delete events: %w", matchID, err)
	}
	if err := q.DeleteMatchSummaryByMatch(ctx, matchID); err != nil {
		return fmt.Errorf("store: clear projections (match %s): delete match_summary: %w", matchID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: clear projections (match %s): commit: %w", matchID, err)
	}
	return nil
}
