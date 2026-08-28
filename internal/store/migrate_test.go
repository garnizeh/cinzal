package store

import (
	"context"
	"errors"
	"testing"
)

// TestMigrationLockIDIsStable pins migrationLockID's literal value so an
// accidental edit shows up as a reviewed diff rather than a silent change —
// the constant's whole purpose is that every instance across a rolling
// deploy agrees on it (RFC-001 §7.5), and this test is what makes a change
// to it visible.
func TestMigrationLockIDIsStable(t *testing.T) {
	const want int64 = 7_312_005_501_884
	if migrationLockID != want {
		t.Fatalf("migrationLockID = %d, want %d — changing this value defeats the "+
			"advisory lock across a rolling deploy, see its doc comment", migrationLockID, want)
	}
}

// TestMigrateRejectsMissingHost mirrors TestOpenRejectsMissingHost
// (pool_test.go): Migrate must not attempt a connection over a DSN that
// resolves ambiently, the same RFC-001 §18 guarantee Open already gives the
// pool. This needs no database — openSingleConnection returns before
// dialing anything.
func TestMigrateRejectsMissingHost(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"empty", ""},
		{"hostless URL", "postgres:///cinzal"},
		{"keyword form", "dbname=cinzal"},
		{"service reference", "service=myservice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Migrate(context.Background(), tt.dsn)
			if err == nil {
				t.Fatal("Migrate returned nil error for a DSN with no explicit host")
			}
			if tt.dsn == "" {
				if !errors.Is(err, ErrEmptyDSN) {
					t.Fatalf("Migrate(%q) error = %v, want ErrEmptyDSN", tt.dsn, err)
				}
				return
			}
			if !errors.Is(err, ErrDSNMissingHost) {
				t.Fatalf("Migrate(%q) error = %v, want ErrDSNMissingHost", tt.dsn, err)
			}
		})
	}
}

// TestMigrateRejectsUnparseableDSN is the parse-error half of the same
// fails-closed-before-dialing guarantee, mirroring
// TestOpenRejectsUnparseableDSN in pool_test.go.
func TestMigrateRejectsUnparseableDSN(t *testing.T) {
	err := Migrate(context.Background(), "postgres://user:pass@realhost/db?sslmode=not-a-real-mode")
	if err == nil {
		t.Fatal("Migrate returned nil error for an unparseable DSN")
	}
	if errors.Is(err, ErrEmptyDSN) || errors.Is(err, ErrDSNMissingHost) {
		t.Fatalf("Migrate reported the wrong sentinel for a parse error: %v", err)
	}
}
