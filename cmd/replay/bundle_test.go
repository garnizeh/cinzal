package main

import (
	"os"
	"path/filepath"
	"testing"
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
	b := Bundle{Seed: make([]byte, 31), Config: []byte(`{"v":1,"config":{}}`), OrderLog: nil}
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with a 31-byte seed returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsEmptyOrderLog: a bundle with no orders carries no
// way to derive a player count, and must fail rather than guess one.
func TestReadBundleRejectsEmptyOrderLog(t *testing.T) {
	_, seed, _, _ := testFixture()
	path := filepath.Join(t.TempDir(), "empty-log.json")
	b := Bundle{Seed: seed[:], Config: []byte(`{"v":1,"config":{}}`), OrderLog: nil}
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	if _, _, _, _, err := readBundle(path); err == nil {
		t.Fatal("readBundle with an empty order log returned nil error, want a rejection")
	}
}

// TestReadBundleRejectsUnknownField: a bundle field this version does not
// know about is a decode error, mirroring D44's DisallowUnknownFields
// discipline for the two columns this format concatenates.
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
