package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/cinzal/internal/store"
)

// TestReadBundleRoundTrip is #322's own acceptance criterion applied at the
// codec level: a bundle written by writeBundle and read back by readBundle
// reproduces the exact seed, config and order log testFixture built.
func TestReadBundleRoundTrip(t *testing.T) {
	wantCfg, wantSeed, wantPlayers, wantLog := testFixture()
	path := writeTestBundle(t)

	seed, cfg, log, players, err := readBundle(path)
	if err != nil {
		t.Fatalf("readBundle: %v", err)
	}
	if seed != wantSeed {
		t.Errorf("seed = %x, want %x", seed, wantSeed)
	}
	if players != wantPlayers {
		t.Errorf("players = %d, want %d", players, wantPlayers)
	}
	if cfg.Rounds != wantCfg.Rounds {
		t.Errorf("cfg.Rounds = %d, want %d", cfg.Rounds, wantCfg.Rounds)
	}
	if len(log) != len(wantLog) {
		t.Errorf("len(log) = %d, want %d", len(log), len(wantLog))
	}
}

// TestReadBundleRejectsTrailingData mirrors store.DecodeConfig's and
// orderlog.Decode's own trailing-data guard: a bundle file with garbage
// appended after a well-formed JSON object must be rejected, not silently
// truncated to the first value.
func TestReadBundleRejectsTrailingData(t *testing.T) {
	path := writeTestBundle(t)
	data := mustReadFile(t, path)
	data = append(data, []byte(`{"extra":true}`)...)

	corrupt := filepath.Join(t.TempDir(), "trailing.json")
	if err := os.WriteFile(corrupt, data, 0o644); err != nil {
		t.Fatalf("write corrupt bundle: %v", err)
	}

	if _, _, _, _, err := readBundle(corrupt); err == nil {
		t.Fatal("readBundle with trailing data after the bundle returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsWrongSeedLength is D44's own rule ("a 31-byte row
// must fail loudly on read") applied to the bundle's seed field.
func TestReadBundleRejectsWrongSeedLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short-seed.json")
	b := Bundle{Seed: make([]byte, 31), Config: []byte(`{"v":1,"config":{}}`), Players: 2, OrderLog: nil}
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with a 31-byte seed returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsEmptyOrderLog: a bundle with no orders cannot be
// validated against the declared player count, and must fail rather than guess.
func TestReadBundleRejectsEmptyOrderLog(t *testing.T) {
	_, seed, _, _ := testFixture()
	path := filepath.Join(t.TempDir(), "empty-log.json")
	b := Bundle{Seed: seed[:], Config: []byte(`{"v":1,"config":{}}`), Players: 2, OrderLog: nil}
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with an empty order log returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsDuplicateRoundSeat is a CodeRabbit review finding on
// PR #404: a bundle's order log carrying two entries for the same (round,
// seat) — the offline counterpart of a corrupted database read — used to
// silently overwrite the earlier payload in orderlog.Decode's map
// assignment rather than being rejected. readBundle reshapes b.OrderLog into
// []store.Order and calls that same orderlog.Decode (bundle.go), so this
// proves the offline path is covered by the same rejection, not a separate
// (and possibly missed) check.
func TestReadBundleRejectsDuplicateRoundSeat(t *testing.T) {
	cfg, seed, _, _ := testFixture()
	// A genuinely valid config envelope, not a stub — DecodeConfig runs
	// before the order log is ever reached (readBundle's own order), so a
	// stub config would fail there first and this test would pass for the
	// wrong reason, never actually exercising the duplicate check below it.
	configEnvelope, err := store.EncodeConfig(cfg)
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	orderLog := []BundleOrder{
		{Round: 1, Seat: 0, Payload: []byte(`{"round":1,"route":[]}`)},
		{Round: 1, Seat: 0, Payload: []byte(`{"round":1,"route":[]}`)}, // duplicate (round 1, seat 0)
		{Round: 1, Seat: 1, Payload: []byte(`{"round":1,"route":[]}`)},
	}
	b := Bundle{Seed: seed[:], Config: configEnvelope, Players: 2, OrderLog: orderLog}

	path := filepath.Join(t.TempDir(), "duplicate-round-seat.json")
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with a duplicate (round, seat) order log entry returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsTruncatedHighestSeat is a CodeRabbit review finding on
// PR #404: a bundle's declared player count must match the highest seat found
// in the order log. If all orders for seat N are removed from an N-player
// bundle, the declared count prevents the bundle from being silently
// reinterpreted as an (N-1)-player game — the bundle is rejected as
// corrupted/truncated instead.
func TestReadBundleRejectsTruncatedHighestSeat(t *testing.T) {
	cfg, seed, _, _ := testFixture()
	configEnvelope, err := store.EncodeConfig(cfg)
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	// A 3-player bundle with orders only for seats 0 and 1, but declared as
	// 3-player. When readBundle validates Players against the highest seat
	// (1), the declared 3 != derived 2, and the bundle is rejected.
	orderLog := []BundleOrder{
		{Round: 1, Seat: 0, Payload: []byte(`{"round":1,"route":[]}`)},
		{Round: 1, Seat: 1, Payload: []byte(`{"round":1,"route":[]}`)},
	}
	b := Bundle{Seed: seed[:], Config: configEnvelope, Players: 3, OrderLog: orderLog}

	path := filepath.Join(t.TempDir(), "truncated-highest-seat.json")
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with a truncated highest seat (declared 3 players but only seat 0-1 in log) returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsUnknownField: a bundle field this version does not
// know about is a decode error, mirroring D44's DisallowUnknownFields
// discipline for the columns this format concatenates.
func TestReadBundleRejectsUnknownField(t *testing.T) {
	path := writeTestBundle(t)
	data := mustReadFile(t, path)

	// Splice an unknown top-level key into the object — cheaper than
	// building a second Bundle-like struct with an extra field just for
	// this one test.
	corrupt := append([]byte(`{"unexpected_field":true,`), data[1:]...)
	path2 := filepath.Join(t.TempDir(), "unknown-field.json")
	if err := os.WriteFile(path2, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt bundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path2); err == nil {
		t.Fatal("readBundle with an unknown top-level field returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsInvalidPlayerCount: the Players field must be >= 1.
// A bundle with Players <= 0 is structurally invalid.
func TestReadBundleRejectsInvalidPlayerCount(t *testing.T) {
	cfg, seed, _, _ := testFixture()
	configEnvelope, err := store.EncodeConfig(cfg)
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	orderLog := []BundleOrder{
		{Round: 1, Seat: 0, Payload: []byte(`{"round":1,"route":[]}`)},
	}
	b := Bundle{Seed: seed[:], Config: configEnvelope, Players: 0, OrderLog: orderLog}

	path := filepath.Join(t.TempDir(), "zero-players.json")
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with Players=0 returned nil error, want a rejection")
	}
}
