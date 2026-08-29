package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #318: match creation and reload — the other half of
// fold's inputs (RFC-001 §7.1: state = fold(Resolve, initial(seed, cfg),
// orderLog)). Issue #317 (orders.go/orderlog.go) persisted orderLog; this
// file persists (seed, cfg), plus the match_players rows that make players
// (rules.NewMatch's third argument) a checkable fact instead of a number a
// caller merely asserts.
//
// CreateMatch and LoadMatch are *Store methods, not internal/match ones:
// unlike orderlog.Load (issue #317), neither returns a
// scripts/check-fog-boundary.sh-forbidden type — MatchMeta/SeatSpec carry
// lobby-roster data (seat, user, bot tier, faction), the same category
// match_players.sql's own doc comment already gives ListMatchPlayers
// ("lobby/roster data, not the game view... no fog-scoped field"), and
// game.Config/[32]byte are not fog-sensitive at all: RFC §6.2's whole point
// is that a match's config is public — every player already knows the
// dials a match was created with. Nothing here needs orderlog's dedicated
// sub-package treatment.

// SeatSpec is one seat's roster data at match creation. GDD's "five
// playable factions ... cosmetic only" (v1) makes Faction a display choice
// with no rules meaning — internal/rules' own seatPlayers (initial.go)
// builds every Player from nothing but (seed, cfg, players) and has no
// Faction field at all — so CreateMatch takes it as plain caller-supplied
// data, the same way match_players.faction (migration 00001) is TEXT with
// no CHECK constraint naming a closed set.
//
// Exactly one of UserID/BotKind is meaningful per seat: a human seat names
// the player (UserID non-nil, BotKind nil); a bot seat names its tier
// (BotKind non-nil, naming internal/bots' registry, UserID nil).
// CreateMatch does not itself enforce this exclusivity — match_players'
// schema doesn't either (migration 00001: both columns are independently
// NULL-able) — so a seat leaving both nil is a bot-less, player-less row
// today; RFC-001 §8.2's Autopilot derivation is defined in terms of orders'
// own source column, not this pair, so nothing downstream silently
// misreads that case as a fact it isn't.
type SeatSpec struct {
	UserID  *pgtype.UUID
	BotKind *string
	Faction string
}

// MatchMeta is everything about a matches row besides its frozen Config and
// seed (kept separate per D44/RFC §6.2's own split) — the lifecycle fields
// and the full seat roster LoadMatch's caller needs to drive fold() (#319)
// without a second round trip to ListMatchPlayers.
//
// Seats is ordered by seat number, 0..len(Seats)-1, contiguous with no gap
// — LoadMatch's own seatsFromRows already rejects any row set that isn't,
// so len(Seats) is safe to pass as rules.NewMatch's players argument
// directly. Round, Status, CreatedBy and the two nullable timer/deadline
// columns are read straight off the matches row with no decoding beyond
// what sqlc's generated Match struct already gives.
type MatchMeta struct {
	Status          string
	Round           game.RoundNumber
	TimerSeconds    *int32
	DeadlineSeconds *int32
	CreatedBy       pgtype.UUID
	Seats           []SeatSpec
}

// CreateMatch writes the match row and every match_players row in one
// transaction (issue #318's own acceptance criterion): a failure part-way —
// a seat insert violating the (match_id, seat) primary key, a lost
// connection mid-transaction — rolls the whole thing back and leaves no
// match row at all, never a match with fewer seats than players.
//
// cfg is validated against len(seats) before anything is written (GDD
// §16.2/§10.3/§6.1's deck-arithmetic and per-player-count table checks,
// game.Config.Validate) — an unservable config never reaches the database
// to begin with, matching LoadMatch's own re-validation on the read side
// (D44's "cfg is validated on read, not trusted" applies with equal force
// to the write that produces the row LoadMatch will eventually read back).
//
// timerSeconds/deadlineSeconds are matches' own "sync tables"/"async
// tables" columns (migration 00001) — both nullable, both left to the
// caller to decide; CreateMatch applies no default of its own.
//
// Every seat's unsubscribe_token_hash is minted here, unconditionally, per
// D19: "sha256(32 random bytes from crypto/rand), generated at seat
// creation." Seat creation is exactly what this function does — there is
// no earlier point in the system a token could be minted at, whether or
// not the mail path that eventually reads it (M6) has been built yet.
func (s *Store) CreateMatch(ctx context.Context, seed [32]byte, cfg game.Config, seats []SeatSpec, createdBy pgtype.UUID, timerSeconds, deadlineSeconds *int32) (game.MatchID, error) {
	if len(seats) == 0 {
		return "", fmt.Errorf("store: create match: at least one seat is required")
	}
	if err := cfg.Validate(len(seats)); err != nil {
		return "", fmt.Errorf("store: create match: %w", err)
	}

	payload, err := EncodeConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("store: create match: encode config: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("store: create match: begin transaction: %w", err)
	}
	// Rollback after a successful Commit is a documented no-op (pgx.Tx); the
	// error is discarded deliberately — nothing actionable to do with a
	// rollback failing after the transaction is already resolved either way.
	defer func() { _ = tx.Rollback(ctx) }()

	q := New(tx)

	m, err := q.CreateMatch(ctx, CreateMatchParams{
		Config:          payload,
		Seed:            seed[:],
		CreatedBy:       createdBy,
		TimerSeconds:    timerSeconds,
		DeadlineSeconds: deadlineSeconds,
	})
	if err != nil {
		return "", fmt.Errorf("store: create match: insert match: %w", err)
	}

	for i, seat := range seats {
		hash, err := newUnsubscribeTokenHash()
		if err != nil {
			return "", fmt.Errorf("store: create match: mint unsubscribe token for seat %d: %w", i, err)
		}
		if _, err := q.CreateMatchPlayer(ctx, CreateMatchPlayerParams{
			MatchID:              m.ID,
			Seat:                 game.SeatID(i),
			UserID:               seat.UserID,
			BotKind:              seat.BotKind,
			Faction:              seat.Faction,
			UnsubscribeTokenHash: hash,
		}); err != nil {
			return "", fmt.Errorf("store: create match: insert seat %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("store: create match: commit: %w", err)
	}
	return m.ID, nil
}

// newUnsubscribeTokenHash mints one D19 unsubscribe token the way D19 (and
// D17, which it cites) specifies: 32 bytes of crypto/rand, SHA-256'd. The
// raw token itself is never returned or stored — D19's route (M5/M6, not
// this issue) mints the same bytes again when it actually needs to compare
// against this hash; this function's only job is to produce a
// NOT-NULL-satisfying, unguessable digest at seat-creation time.
func newUnsubscribeTokenHash() ([]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("store: generate random token: %w", err)
	}
	sum := sha256.Sum256(raw[:])
	return sum[:], nil
}

// LoadMatch reads back exactly what CreateMatch wrote: the frozen seed and
// Config, plus every roster fact a caller needs to fold the match forward
// (#319).
//
// cfg is validated against the *actual* match_players row count, not
// trusted from the payload alone — D44's "cfg is validated on read, not
// trusted," and issue #318's own acceptance criterion: "LoadMatch calls
// cfg.Validate(players) and returns its error rather than handing back a
// partially-valid config." seatsFromRows enforces the row count and the
// seat-index contiguity check in the same pass — issue #318: "the players
// argument to rules.NewMatch and the match_players row count are checked
// to agree on reload; a disagreement is an error, not a fold."
func (s *Store) LoadMatch(ctx context.Context, matchID game.MatchID) ([32]byte, game.Config, MatchMeta, error) {
	var zeroSeed [32]byte
	q := New(s.pool)

	m, err := q.GetMatch(ctx, matchID)
	if err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: %w", matchID, err)
	}

	seed, err := decodeSeed(m.Seed)
	if err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: %w", matchID, err)
	}

	cfg, err := DecodeConfig(m.Config)
	if err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: decode config: %w", matchID, err)
	}

	rows, err := q.ListMatchPlayers(ctx, matchID)
	if err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: list seats: %w", matchID, err)
	}
	seats, err := seatsFromRows(rows)
	if err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: %w", matchID, err)
	}

	if err := cfg.Validate(len(seats)); err != nil {
		return zeroSeed, game.Config{}, MatchMeta{}, fmt.Errorf("store: load match %s: %w", matchID, err)
	}

	return seed, cfg, MatchMeta{
		Status:          m.Status,
		Round:           m.Round,
		TimerSeconds:    m.TimerSeconds,
		DeadlineSeconds: m.DeadlineSeconds,
		CreatedBy:       m.CreatedBy,
		Seats:           seats,
	}, nil
}

// seatsFromRows converts ListMatchPlayers' rows — already ordered by seat
// (internal/store/queries/match_players.sql's own ORDER BY seat) — into a
// dense SeatSpec slice indexed 0..len(rows)-1, rejecting anything else: a
// gap (seat 2 present with no seat 1), a duplicate, or a seat numbered
// outside that range. match_players.seat is the *only* place a match's
// player count is recorded (matches carries no separate count column), so
// this is the single check standing between a corrupted roster and a
// silently wrong players argument reaching rules.NewMatch downstream — the
// same shape orderlog.checkNoRoundGap (issue #317) already gives the order
// log's own round axis.
func seatsFromRows(rows []ListMatchPlayersRow) ([]SeatSpec, error) {
	seats := make([]SeatSpec, len(rows))
	for i, row := range rows {
		if int(row.Seat) != i {
			return nil, fmt.Errorf("seat roster is not contiguous from 0: expected seat %d at position %d, got seat %d", i, i, row.Seat)
		}
		seats[i] = SeatSpec{
			UserID:  row.UserID,
			BotKind: row.BotKind,
			Faction: row.Faction,
		}
	}
	return seats, nil
}
