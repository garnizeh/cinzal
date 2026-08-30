package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/orderlog"
)

// Bundle is RFC-001 §15.4's own replay bundle: "{seed, config, players,
// orderLog} for a finished match, downloadable by its players... attach it
// to an issue and cmd/replay reproduces the exact match." Issue #322 decides
// its wire shape here, and D44 already decided the two encodings it reuses:
// this is "the JSONB columns concatenated, not a fourth format" — matches.config
// and orders.payload travel exactly as those columns store them, never
// re-derived from a decoded value. board_notes is excluded by construction
// (D18): Bundle carries nothing beyond the four values a fold needs
// (including the validated player count added in PR #404).
type Bundle struct {
	// Seed is the match's 32-byte seed, exactly as matches.seed stores it.
	// []byte, not [32]byte, so encoding/json's built-in []byte<->base64
	// handling applies with no custom (Un)MarshalJSON — a length other than
	// 32 is still a hard decode-time error, checked explicitly once a
	// Bundle is turned back into fold inputs (readBundle), never silently
	// padded or truncated (D44's own rule for matches.seed, applied
	// identically here).
	Seed []byte `json:"seed"`

	// Config is matches.config's own bytes, verbatim — the D44 version
	// envelope ({"v":1,"config":{...}}), never re-marshalled from a decoded
	// game.Config. Kept as json.RawMessage so building a Bundle cannot
	// introduce key-order or whitespace drift from the row's own stored
	// bytes, and reading one decodes it through the same store.DecodeConfig
	// every other reader of matches.config already uses.
	Config json.RawMessage `json:"config"`

	// Players is the number of seats in the match, explicitly stored to
	// validate against the order log's actual content. A bundle with no orders
	// for seat N is truncated and must be rejected, not silently reinterpreted
	// as a (N-1)-player game. This field is validated in readBundle by
	// comparing against the maximum seat index found in the order log —
	// on mismatch, the bundle is rejected as corrupted or hand-edited.
	Players int `json:"players"`

	// OrderLog is every orders row for the match, one entry per (round,
	// seat), in the same (round, seat) ascending order
	// ListOrdersForMatch's own query already returns
	// (internal/store/queries/orders.sql) — never a map, so writing a
	// Bundle needs no separate sort step and produces the same bytes on
	// every run (RFC §6.3: no map-range order).
	OrderLog []BundleOrder `json:"order_log"`
}

// BundleOrder is one orders row, carrying exactly the three columns a fold
// needs to place it: which round, which seat, and the row's own payload
// bytes (D44/D47's Order codec) — verbatim, never re-encoded from a decoded
// game.Order, for the same byte-fidelity reason Bundle.Config isn't.
type BundleOrder struct {
	Round   game.RoundNumber `json:"round"`
	Seat    game.SeatID      `json:"seat"`
	Payload json.RawMessage  `json:"payload"`
}

// exportBundleFromDB assembles a Bundle straight from matchID's own rows —
// GetMatch for seed/config, ListOrdersForMatch for the log, ListMatchPlayers
// for the player count — with no decode or re-encode step in between, so a
// bundle exported this way is byte-for-byte the same data the database itself
// holds. This is #322's own acceptance criterion: "a bundle exported from a
// match and a bundle assembled from that match's rows are equal" — true here
// by construction, since all data comes straight from the identical rows,
// untouched. The Players field is populated from the match_players row count
// and validated in readBundle.
func exportBundleFromDB(ctx context.Context, db store.DBTX, matchID game.MatchID) (Bundle, error) {
	q := store.New(db)

	m, err := q.GetMatch(ctx, matchID)
	if err != nil {
		return Bundle{}, fmt.Errorf("cmd/replay: export bundle: get match %s: %w", matchID, err)
	}

	seats, err := q.ListMatchPlayers(ctx, matchID)
	if err != nil {
		return Bundle{}, fmt.Errorf("cmd/replay: export bundle: list seats for match %s: %w", matchID, err)
	}

	rows, err := q.ListOrdersForMatch(ctx, matchID)
	if err != nil {
		return Bundle{}, fmt.Errorf("cmd/replay: export bundle: list orders for match %s: %w", matchID, err)
	}

	orderLog := make([]BundleOrder, len(rows))
	for i, r := range rows {
		orderLog[i] = BundleOrder{Round: r.Round, Seat: r.Seat, Payload: json.RawMessage(r.Payload)}
	}

	return Bundle{
		Seed:     m.Seed,
		Config:   json.RawMessage(m.Config),
		Players:  len(seats),
		OrderLog: orderLog,
	}, nil
}

// writeBundle marshals b as indented, deterministic JSON and writes it to
// path. Field order is Bundle's own declared order (encoding/json never
// reorders struct fields) and OrderLog is already a slice in
// ListOrdersForMatch's fixed order, so two exports of the same match rows
// produce byte-identical files.
//
// A CodeRabbit review finding on PR #404: json.MarshalIndent compacts a
// nested json.RawMessage's own bytes before indenting the surrounding
// document, so Config's and each BundleOrder.Payload's bytes on disk here
// are not guaranteed to match matches.config's/orders.payload's original
// stored bytes verbatim (whitespace/escaping may differ) — only the decoded
// JSON value is guaranteed identical. That is the actual contract: nothing
// in this package ever compares a bundle file's raw bytes against the
// database's raw bytes. Bundle.Config's own "verbatim"/"byte-fidelity"
// language above describes exportBundleFromDB's assembly step — Config and
// Payload are copied straight from the query rows with no decode-then-
// re-encode in between, so building a Bundle cannot itself introduce drift —
// not a promise about this function's on-disk formatting. Every reader
// (store.DecodeConfig, orderlog.Decode) parses structurally, and both
// #322's byte-identity acceptance criteria check either the fold's dump
// output (TestReplayMatchAndBundleProduceByteIdenticalOutput) or
// exportBundleFromDB's return value against an independently assembled
// Bundle (TestExportBundleFromDBEqualsAssembledFromRows) — neither compares
// this file's on-disk bytes to the row's original bytes, so semantic JSON
// equivalence is the whole requirement here.
func writeBundle(path string, b Bundle) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("cmd/replay: encode bundle: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cmd/replay: write bundle %s: %w", path, err)
	}
	return nil
}

// readBundle reads and decodes a Bundle from path and turns it into the same
// fold inputs the --match path gets from the database.
//
// cfg is decoded through store.DecodeConfig — a bundle's config bytes get
// exactly the same D44 corruption checks a database row does, never a
// weaker one because the source is a file instead of a table. log is
// decoded through orderlog.Decode — the identical D44/D47 Order codec and
// round-gap check ListOrdersForMatch's own rows are put through. Neither
// check is reimplemented here.
//
// players is read from the bundle's explicit Players field (PR #404 finding)
// and validated against the highest seat index in the order log: if the
// declared player count disagrees with the actual orders, the bundle is
// rejected as corrupted or hand-edited. This catch truncated bundles — if a
// 3-player bundle loses all orders for seat 2, players would wrongly be
// derived as 2 instead of being rejected as an incomplete/corrupted bundle.
func readBundle(path string) (seed [32]byte, cfg game.Config, log rules.OrderLog, players int, err error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: read bundle %s: %w", path, readErr)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var b Bundle
	if err := dec.Decode(&b); err != nil {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: decode bundle %s: %w", path, err)
	}
	// Mirrors store.DecodeConfig's and orderlog.Decode's own trailing-data
	// guard (CodeRabbit finding on PR #393): json.Decoder.Decode stops at
	// the first complete value, so without this check a bundle file with
	// garbage appended after a well-formed object would decode successfully
	// and silently ignore the rest.
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: decode bundle %s: trailing data after JSON value", path)
	}

	if len(b.Seed) != 32 {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: bundle %s: seed must be exactly 32 bytes, got %d", path, len(b.Seed))
	}
	copy(seed[:], b.Seed)

	if b.Players < 1 {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: bundle %s: players must be >= 1, got %d", path, b.Players)
	}

	cfg, err = store.DecodeConfig(b.Config)
	if err != nil {
		return seed, game.Config{}, nil, 0, fmt.Errorf("cmd/replay: bundle %s: decode config: %w", path, err)
	}

	rows := make([]store.Order, len(b.OrderLog))
	maxSeat := -1
	for i, o := range b.OrderLog {
		rows[i] = store.Order{Round: o.Round, Seat: o.Seat, Payload: o.Payload}
		if int(o.Seat) > maxSeat {
			maxSeat = int(o.Seat)
		}
	}
	if maxSeat < 0 {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: bundle %s: order log is empty, cannot validate player count", path)
	}

	derivedPlayers := maxSeat + 1
	if b.Players != derivedPlayers {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: bundle %s: declared players (%d) does not match highest seat in order log (%d) — bundle may be corrupted or truncated", path, b.Players, derivedPlayers)
	}

	// matchID is used only inside orderlog.Decode's own error messages — a
	// bundle has no match identity of its own (RFC §15.4's whole point is
	// that it needs none), so a fixed value naming the bundle's own path
	// stands in for it.
	log, err = orderlog.Decode(game.MatchID("bundle:"+path), rows)
	if err != nil {
		return seed, cfg, nil, 0, fmt.Errorf("cmd/replay: bundle %s: decode order log: %w", path, err)
	}

	return seed, cfg, log, derivedPlayers, nil
}
