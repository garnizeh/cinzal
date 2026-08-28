package store

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSqlcGenerateFailsOnUnknownColumn is issue #315's fail-closed
// acceptance criterion: sqlc's own type-checking against the real schema is
// the gate here, and a config that happily type-checks against a stale or
// duplicate schema file would defeat the entire reason sqlc.yaml points at
// internal/store/migrations directly instead of a hand-copied schema.sql
// (RFC-001 §7.5). This proves the negative: a query referencing a column
// that does not exist anywhere in the real migrations fails `sqlc generate`
// rather than silently producing broken (or stale, unregenerated) code.
//
// Runs against a throwaway copy of the real migrations directory, unaltered
// — the same schema production migrate.go embeds — plus one extra query
// file naming a column that exists nowhere in it.
func TestSqlcGenerateFailsOnUnknownColumn(t *testing.T) {
	sqlcPath, err := exec.LookPath("sqlc")
	if err != nil {
		// Fail closed, not skip (CLAUDE.md: "a gate that passes when it
		// can't run is worse than no gate" — the same reasoning behind
		// every require-% target in the Makefile). CI installs this exact
		// pinned version (SQLC_VERSION, .github/workflows/ci.yml) before
		// any gate runs; a local environment missing it is a real gap in
		// what this test proved, not grounds to report a pass.
		t.Fatalf("sqlc not found on PATH (see CONTRIBUTING.md's Requirements section): %v", err)
	}

	tmp := t.TempDir()

	tmpMigrations := filepath.Join(tmp, "migrations")
	copyDir(t, "migrations", tmpMigrations)

	tmpQueries := filepath.Join(tmp, "queries")
	if err := os.MkdirAll(tmpQueries, 0o755); err != nil {
		t.Fatalf("mkdir queries: %v", err)
	}
	const badQuery = `-- name: GetMatchByNonexistentColumn :one
SELECT id FROM matches WHERE this_column_does_not_exist = $1;
`
	if err := os.WriteFile(filepath.Join(tmpQueries, "bad.sql"), []byte(badQuery), 0o644); err != nil {
		t.Fatalf("write bad query: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(tmp, "out"), 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	const cfg = `version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"
    queries: "queries"
    gen:
      go:
        package: "genfail"
        out: "out"
        sql_package: "pgx/v5"
`
	cfgPath := filepath.Join(tmp, "sqlc.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write sqlc.yaml: %v", err)
	}

	cmd := exec.Command(sqlcPath, "generate", "--file", cfgPath)
	cmd.Dir = tmp
	out, genErr := cmd.CombinedOutput()
	if genErr == nil {
		t.Fatalf("sqlc generate unexpectedly succeeded against a query naming a nonexistent column; output:\n%s", out)
	}
	if !strings.Contains(string(out), "this_column_does_not_exist") {
		t.Fatalf("sqlc generate failed, but not for the expected reason (want a complaint about this_column_does_not_exist); output:\n%s", out)
	}
}

// copyDir copies src (relative to this package's directory) to dst,
// preserving the tree shape. t.TempDir()'s tree is discarded automatically
// at test end, so this never touches the real migrations directory.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyDir(%q, %q): %v", src, dst, err)
	}
}

// TestGeneratedPackageDepsExcludeRules is issue #315's other required
// assertion: go list -deps on the package sqlc generates into (internal/
// store itself — see sqlc.yaml's own header comment on why output is not a
// separate subpackage) must name no internal/ package other than
// internal/game. This is what keeps the generated signatures' domain-
// vocabulary overrides (game.SeatID, game.MatchID, game.RoundNumber) from
// becoming a backdoor for internal/rules — or any other internal package —
// to reach the persistence layer, which D01's leaf-package rule for
// internal/game exists specifically to prevent.
func TestGeneratedPackageDepsExcludeRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/garnizeh/cinzal/internal/store").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps github.com/garnizeh/cinzal/internal/store: %v\n%s", err, out)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "github.com/garnizeh/cinzal/internal/") {
			continue
		}
		switch line {
		case "github.com/garnizeh/cinzal/internal/game", "github.com/garnizeh/cinzal/internal/store":
			continue
		}
		t.Errorf("internal/store depends on %s -- only internal/game is allowed here (issue #315)", line)
	}
}
