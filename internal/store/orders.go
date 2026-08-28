package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
)

// Source is orders.source's three legal values (migration 00001's
// CHECK (source IN ('human', 'bot', 'default'))). RFC-001 §8.2 derives
// Autopilot from this column directly — "seat has a user_id AND the last
// two rounds both recorded source <> 'human'" — so the column's contents
// are a game fact, not bookkeeping, and a caller states which one applies
// rather than the type defaulting to any single value.
type Source string

const (
	// SourceHuman is a real player's own submission. RFC §8.2: a human
	// resubmission over a default/bot row is what un-derives Autopilot —
	// AppendOrder's ON CONFLICT DO UPDATE flips this column back to
	// 'human' on exactly that resubmission, with no separate code path.
	SourceHuman Source = "human"

	// SourceBot is a bot-generated order — a filler bot's own play (GDD
	// §14.2), or the heuristic that plays a seat while it is on Autopilot
	// (GDD §18). RFC §8.2's derivation deliberately excludes this from the
	// "still human" case: 'bot' counts toward the two-consecutive-absence
	// streak exactly like 'default' does.
	SourceBot Source = "bot"

	// SourceDefault is the round's default order (GDD §18: empty route,
	// Deliver-if-legal-else-Nothing, Evasive, no add-ons), written when a
	// deadline lapses with no submission and no bot fills the seat.
	SourceDefault Source = "default"
)

// Pool exposes s's underlying connection pool as the minimal DBTX surface
// (db.go, sqlc's own generated interface) — what New needs to build a
// *Queries, and what any hand-written raw-SQL caller needs otherwise. This
// is the same shape ratelimit.go's package-level ConsumeRateLimit/
// CleanupRateLimits already take as their own dbtx parameter; the two
// interfaces are structurally identical, kept separate because dbtx
// predates sqlc's generated DBTX (db.go's own comment) rather than as a
// deliberate distinction.
//
// internal/store/orderlog.Load (issue #317) is Pool's first caller outside
// this package: it needs a DBTX to build its own *Queries, and cannot get
// one through a *Store-returning method without becoming reachable from
// wherever *Store already is (see orderlog's own package doc comment for
// why that specific function cannot be a *Store method at all). Exposing
// the pool itself, rather than a pre-built *Queries, keeps this generic
// for any future caller building a *Queries bound to a transaction it
// began itself, not only one bound to the ambient pool.
func (s *Store) Pool() DBTX {
	return s.pool
}

// AppendOrder writes one seat's order for one round of one match — GDD
// §18's "resubmission is allowed while the round is open; the last
// submission stands," enforced by orders' own primary key
// (match_id, round, seat) and the UpsertOrder query's
// ON CONFLICT (match_id, round, seat) DO UPDATE (issue #315,
// internal/store/queries/orders.sql). One seat, one round, one call — never
// batched, so a caller cannot accidentally overwrite a sibling seat's row.
//
// round and seat name the row being written; o.Round (game.Order's own
// field, RFC §11.1a) is the round the submitted form was rendered for and
// travels inside the encoded payload unchanged — the two are allowed to
// differ, and detecting that they do is the submission handler's staleness
// check (M5), not this function's.
//
// o is encoded per D44/D47: game.Order's struct tags and its four enum
// types' MarshalJSON (internal/game/order.go, enums_wire.go). A struct
// field reaching its own reserved-invalid zero is expected and ordinary
// (D47) and encodes as key-absence, never an error; encoding fails only on
// a genuinely un-encodable value (D47 §5's Sector case, unreachable through
// a legally-constructed Order).
//
// src is the row's source column (see Source's own doc comment) — the
// caller states it explicitly rather than this function inferring one, so
// the human/bot/default distinction that RFC §8.2's Autopilot derivation
// depends on is never guessed at the storage layer.
func (s *Store) AppendOrder(ctx context.Context, matchID game.MatchID, round game.RoundNumber, seat game.SeatID, o game.Order, src Source) error {
	// game.RoundNumber is 1-indexed (GDD §4); reject round < 1 here rather
	// than letting it reach the table. This is checked before payload is
	// even encoded so a caller gets the same clear error whether or not the
	// row would otherwise be well-formed — orders_round_positive (migration
	// 00004) enforces the identical floor at the storage layer for any row
	// that reaches this table by some other path, and
	// internal/store/orderlog's checkNoRoundGap rejects it again on read,
	// but this is where a live write should stop first (CodeRabbit finding
	// on PR #393, issue #317).
	if round < 1 {
		return fmt.Errorf("store: append order (match %s, round %d, seat %d): round must be >= 1", matchID, round, seat)
	}

	payload, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("store: encode order (match %s, round %d, seat %d): %w", matchID, round, seat, err)
	}

	_, err = New(s.pool).UpsertOrder(ctx, UpsertOrderParams{
		MatchID: matchID,
		Round:   round,
		Seat:    seat,
		Payload: payload,
		Source:  string(src),
	})
	if err != nil {
		return fmt.Errorf("store: append order (match %s, round %d, seat %d): %w", matchID, round, seat, err)
	}
	return nil
}
