package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunRejectsMissingDB is #326's own input contract: --db is required,
// with no ambient DATABASE_URL fallback baked into this binary — the same
// no-fallback-DSN discipline RFC-001 §18 states for internal/store itself,
// kept consistent here even though that package's own TestNoFallbackDSN
// only scans internal/store's source.
func TestRunRejectsMissingDB(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with no --db = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--db") {
		t.Errorf("stderr = %q, want it to mention --db", stderr.String())
	}
}

// TestRunPrintsHelpOnFlagError mirrors cmd/replay's/cmd/simulate's own
// -h handling: flag.ErrHelp exits 0, not 2, and never reaches the --db
// check.
func TestRunPrintsHelpOnFlagError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([-h]) = %d, want 0", code)
	}
}

// TestRunRejectsUnknownFlag: an unrecognized flag is a usage error (exit 2),
// distinct from a validation error (exit 1) — flag.ContinueOnError's own
// contract, exercised here so a future refactor can't quietly swallow it.
func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run([--nope]) = %d, want 2", code)
	}
}

// TestEvaluateMigration is issue #326's fail-closed acceptance criterion in
// unit form: "a run that applies zero migrations against an empty database
// is a failure, not a success." Table-driven over every before/after shape
// evaluateMigration can see — no database involved, since the function is
// pure.
func TestEvaluateMigration(t *testing.T) {
	tests := []struct {
		name          string
		before, after int
		wantDelta     int
		wantOK        bool
	}{
		{"fresh database, migrations applied", 0, 5, 5, true},
		{"fresh database, nothing applied — the failure case", 0, 0, 0, false},
		{"already migrated, idempotent re-run", 5, 5, 0, true},
		{"already migrated, new migrations landed since", 5, 7, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta, ok := evaluateMigration(tt.before, tt.after)
			if delta != tt.wantDelta || ok != tt.wantOK {
				t.Fatalf("evaluateMigration(%d, %d) = (%d, %v), want (%d, %v)",
					tt.before, tt.after, delta, ok, tt.wantDelta, tt.wantOK)
			}
		})
	}
}
