// Package orderlog reads the whole order log for one match back out of
// Postgres as a rules.OrderLog — the fold-loading path RFC-001 §7.1's
// state = fold(Resolve, initial(seed, cfg), orderLog) needs, and the
// counterpart to Store.AppendOrder (internal/store/orders.go), which writes
// one row at a time.
//
// # Why this is a separate package, not a *store.Store method
//
// rules.OrderLog "is never fog-filtered and is at least as sensitive as
// MatchState itself — it names every seat's full route and action history,
// not one seat's" (internal/rules/order_log.go's own doc comment). D01/D34
// already forbid internal/render and internal/web from importing
// internal/rules directly, enforced by scripts/check-fog-boundary.sh's
// FORBIDDEN import list. internal/store itself is not on that list and is
// not added here: internal/web has a standing, RFC-documented reason to
// import internal/store directly (RFC-001 §11.2's board-panel component
// fetches []BoardNote straight from internal/store, since board notes have
// no fog projection to perform — D18), so forbidding internal/store
// wholesale would block that already-decided, legitimate edge.
//
// If Load lived as a method on *store.Store instead, that same edge —
// internal/web importing internal/store for BoardNotes — would also make
// Load reachable, and with it a full, unfiltered rules.OrderLog inside a
// handler. That is exactly the precondition check-fog-boundary.sh's own
// header names: "this holds only if every exported function reachable
// through an ALLOWED import itself returns nothing from a FORBIDDEN
// package." internal/match/fold (D49, docs/decisions/D49-fold-package-
// boundary.md) already solved this identical shape of problem for
// Fold/FoldMeasured returning rules.MatchState: a function returning a
// forbidden-package type moves to its own sub-package, and only that
// sub-package's import path is added to FORBIDDEN, leaving the parent
// package's other, safe exports importable exactly as before.
//
// This package is the same fix, applied here: internal/store/orderlog is
// listed in scripts/check-fog-boundary.sh's FORBIDDEN array (this issue's
// own commit), so internal/web can go on importing internal/store for
// BoardNotes/invite links/etc. while a second, distinct import — this
// package — is what check-fog-boundary.sh actually forbids. internal/match
// (M3 #319, M4's tick) is this package's real caller, exactly as
// internal/match already imports its own internal/match/fold child for the
// same reason (D49's "parent-imports-child" precedent, itself citing
// internal/rules -> internal/rules/gen).
//
// Load is a package-level function rather than a method on some local type
// specifically so it takes no *store.Store — a *store.Store parameter would
// need internal/store/orderlog to import internal/store only for that
// type's sake, and nothing here needs the pool lifecycle *store.Store
// manages, only a store.DBTX to build a *store.Queries from (Store.Pool()
// hands one back — see Load's own doc comment for why a DBTX rather than a
// pre-built *Queries).
package orderlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
)

// Load reads every order ever submitted for matchID and returns it as a
// rules.OrderLog, ready to fold (RFC-001 §7.1). Rows arrive from
// ListOrdersForMatch already ordered by (round, seat) — see
// internal/store/queries/orders.sql — "THIS IS PRE-FOG DATA," per that
// query's own comment: the result names every seat's route and action
// history for the whole match, and nothing above the fold pipeline may see
// it.
//
// db is the minimal store.DBTX surface (satisfied by *store.Store.Pool(),
// or by a pgx.Tx a caller began itself) rather than a *store.Queries —
// mirroring ratelimit.go's package-level ConsumeRateLimit/CleanupRateLimits,
// which take the same shape of interface for the identical reason: the
// caller decides whether this runs standalone or coupled to a sibling
// operation inside one transaction, and this package has no opinion on
// which.
//
// A completely fresh match (no orders submitted yet) returns an empty,
// non-nil rules.OrderLog and a nil error — that is not a gap, it is round 0
// of a match that has not resolved its first round.
func Load(ctx context.Context, db store.DBTX, matchID game.MatchID) (rules.OrderLog, error) {
	rows, err := store.New(db).ListOrdersForMatch(ctx, matchID)
	if err != nil {
		return nil, fmt.Errorf("orderlog: list orders for match %s: %w", matchID, err)
	}
	return Decode(matchID, rows)
}

// Decode is Load's decode-and-shape logic, exported so a caller that already
// holds []store.Order rows from somewhere other than ListOrdersForMatch can
// reuse the identical D44 decode discipline — same strictness, same gap
// check — rather than reimplementing it. cmd/replay's --bundle path (#322)
// is exactly this: a replay bundle's order log arrives as JSON read from a
// file, not a database row set, but once it is reshaped into []store.Order
// (round, seat, payload — the same three fields ListOrdersForMatch's own
// rows carry) it needs to pass through the same corruption/staleness checks
// a database-sourced log does, or a hand-edited bundle could smuggle a
// structurally invalid order log past --bundle that --match would have
// rejected. Split out, prior to this export, so it could be unit tested
// against hand-built rows with no database involved (orderlog_test.go) — the
// gap check and the decode strictness are pure functions of the rows once
// fetched, regardless of where they came from.
func Decode(matchID game.MatchID, rows []store.Order) (rules.OrderLog, error) {
	log := make(rules.OrderLog)

	for _, row := range rows {
		// D44: DisallowUnknownFields is the corruption guard for orders —
		// a genuinely removed field or a stray key is a decode error, never
		// silently ignored. A field absent from an older row is the
		// ordinary case (D44/D47's "absence is a legal historical
		// meaning") and needs no special handling here: encoding/json
		// already leaves an absent field at its zero value on a freshly
		// allocated destination.
		dec := json.NewDecoder(bytes.NewReader(row.Payload))
		dec.DisallowUnknownFields()

		// A fresh, zero-valued Order per row — never reused across
		// iterations. D47's own consequence: "Order decode call sites must
		// construct a fresh zero-valued Order per row, not reuse one
		// across rows, or an absent key reads as the prior row's value
		// instead of unset." Declaring o inside the loop body does this by
		// construction.
		var o game.Order
		if err := dec.Decode(&o); err != nil {
			return nil, fmt.Errorf("orderlog: decode order (match %s, round %d, seat %d): %w",
				matchID, row.Round, row.Seat, err)
		}

		// A CodeRabbit review finding on PR #393: json.Decoder.Decode stops
		// as soon as it has read one complete JSON value and never checks
		// whether the stream holds more after it, so a payload of a valid
		// order object followed by trailing junk (another top-level JSON
		// value, or garbage bytes) would decode o successfully and leave
		// the trailing data silently ignored. Requiring the next Decode to
		// return io.EOF forces the whole payload to be exactly one JSON
		// value with nothing after it, the same "no partial trust" stance
		// DisallowUnknownFields already takes within the object.
		if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("orderlog: decode order (match %s, round %d, seat %d): trailing data after order JSON",
				matchID, row.Round, row.Seat)
		}

		// A CodeRabbit review finding on PR #393: the payload's own Round
		// field is decoded but was never checked against the row it was
		// actually stored under, so a payload claiming a different round
		// than row.Round would silently index into the wrong round's slot
		// in log below. Rows are keyed by row.Round (the column
		// AppendOrder wrote it under) both here and by
		// ListOrdersForMatch's own ordering; a mismatch means the payload
		// was corrupted or mis-written, not a case this reader can paper
		// over.
		if o.Round != row.Round {
			return nil, fmt.Errorf("orderlog: order round %d does not match row round %d (match %s, seat %d)",
				o.Round, row.Round, matchID, row.Seat)
		}

		seats, ok := log[row.Round]
		if !ok {
			seats = make(map[game.SeatID]game.Order)
			log[row.Round] = seats
		}

		// A CodeRabbit review finding on PR #404: a duplicate (round, seat)
		// row — reachable both from a corrupted database read and from a
		// hand-edited/corrupted offline bundle (cmd/replay's --bundle path,
		// which reshapes its rows into []store.Order and calls this same
		// function) — used to overwrite the earlier payload silently, with
		// nothing recording that two rows ever claimed the same slot. A
		// duplicate is exactly the same kind of structurally invalid input
		// the round-mismatch check above already refuses to paper over, so
		// it is rejected here the same way: loudly, naming the match, round
		// and seat, before either payload's data reaches the fold.
		if _, dup := seats[row.Seat]; dup {
			return nil, fmt.Errorf("orderlog: decode order (match %s, round %d, seat %d): duplicate entry for this round and seat",
				matchID, row.Round, row.Seat)
		}
		seats[row.Seat] = o
	}

	if err := checkNoRoundGap(log); err != nil {
		return nil, fmt.Errorf("orderlog: match %s: %w", matchID, err)
	}

	return log, nil
}

// checkNoRoundGap asserts the rounds present in log form a contiguous run
// starting at round 1 (GDD §4: rounds are 1-indexed), with no round
// entirely missing in between. A gap here means fold() would silently stop
// short of the match's real length — issue #317's own acceptance criterion:
// "a match with a gap in its rounds (round 3 missing) is an error, not a
// silently short fold." This does not check that every seat submitted for
// every present round — a round with some, but not all, seats recorded is
// a fold()/M4 concern (an unresolved or in-progress round), not a
// corruption signal this reader can distinguish from one.
//
// It also rejects any round below 1 outright. Scanning only [1, maxRound]
// for gaps would otherwise let a round-0 (or negative) row ride along
// unnoticed whenever the log also has a normal, gapless 1..maxRound run —
// maxRound is computed from every key present, including an invalid one,
// but the gap loop below never revisits it once maxRound is set. That row
// is exactly the malformed data this reader exists to catch (issue #317's
// own acceptance criterion, sharpened by a CodeRabbit review finding on
// PR #393): AppendOrder now refuses to write round < 1 and migration 00004
// adds the same floor as a CHECK, but a log built from hand-written rows
// (this package's own unit tests, a future repair path) must not rely on
// either of those to have run.
func checkNoRoundGap(log rules.OrderLog) error {
	if len(log) == 0 {
		return nil
	}

	var maxRound game.RoundNumber
	for r := range log {
		if r < 1 {
			return fmt.Errorf("round %d is invalid (rounds are 1-indexed)", r)
		}
		if r > maxRound {
			maxRound = r
		}
	}

	for r := game.RoundNumber(1); r <= maxRound; r++ {
		if _, ok := log[r]; !ok {
			return fmt.Errorf("round %d is missing (rounds present up to %d)", r, maxRound)
		}
	}
	return nil
}
