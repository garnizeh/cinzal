package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file is issue #321's own acceptance criterion: "a source-level test
// enumerates every query in the package that reads events or match_summary
// and asserts each is on an allow-list with a stated caller — the same
// shape as scripts/bots-isolation-allowlist.txt" — and its fail-closed
// corollary: the check fails when the allow-list is empty and the package
// contains reads, and fails when an allow-list entry names a query that no
// longer exists.
//
// checkProjectionReadAllowlist is the pure checker both
// TestProjectionReadsAreAllowlisted (the real gate, run against
// internal/store/queries and testdata/projections-read-allowlist.txt) and
// the fixture tests below (synthetic query sets in t.TempDir(), proving the
// fail-closed cases actually fail before trusting the real, currently-empty
// case to mean the mechanism works — CLAUDE.md's "probe new gate logic
// before opening the PR") exercise.

// projectionQueryBlock is one "-- name: X :type" ... SQL block parsed out of
// a sqlc query file. body has every "--"-prefixed comment line stripped, so
// a comment merely *mentioning* events or match_summary (as several already
// do, in this package's own doc comments) never trips the table-reference
// check below — only actual SQL does.
type projectionQueryBlock struct {
	name string
	body string
}

var queryNameRe = regexp.MustCompile(`^--\s*name:\s*(\S+)\s*:(\S+)\s*$`)

// parseProjectionQueryBlocks reads every *.sql file directly under dir and
// splits it into its named query blocks. Fails closed: an unreadable
// directory or file, or a directory with no blocks at all, is an error, not
// an empty result silently treated as "nothing to check."
func parseProjectionQueryBlocks(dir string) ([]projectionQueryBlock, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read queries dir %q: %w", dir, err)
	}

	var blocks []projectionQueryBlock
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}

		var cur *projectionQueryBlock
		var bodyLines []string
		flush := func() {
			if cur != nil {
				cur.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
				blocks = append(blocks, *cur)
			}
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := queryNameRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				flush()
				cur = &projectionQueryBlock{name: m[1]}
				bodyLines = nil
				continue
			}
			if cur == nil {
				continue // file-header comment, before this file's first query
			}
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue // a comment line inside/after a query, not SQL
			}
			bodyLines = append(bodyLines, line)
		}
		flush()
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no query blocks found under %q — this check would inspect nothing", dir)
	}
	return blocks, nil
}

var (
	leadingSelectRe    = regexp.MustCompile(`(?is)^\s*SELECT\b`)
	fromEventsRe       = regexp.MustCompile(`(?i)\bFROM\s+events\b`)
	fromMatchSummaryRe = regexp.MustCompile(`(?i)\bFROM\s+match_summary\b`)
)

// isProjectionRead reports whether body is a genuine read of events or
// match_summary: its leading statement is SELECT, referencing one of the
// two tables in a FROM clause. An INSERT ... ON CONFLICT ... RETURNING
// (UpsertMatchSummary, UpsertOrder, CreateMatch, CreateMatchPlayer
// elsewhere in this package) is a write that merely returns the row just
// written — its leading keyword is INSERT, not SELECT — so it is
// deliberately not classified as a read here.
func isProjectionRead(body string) bool {
	if !leadingSelectRe.MatchString(body) {
		return false
	}
	return fromEventsRe.MatchString(body) || fromMatchSummaryRe.MatchString(body)
}

// checkProjectionReadAllowlist enumerates every query under queriesDir,
// classifies which are genuine reads of events/match_summary
// (isProjectionRead), and cross-checks that set against allowlistPath —
// one bare query name per line, blank lines and #-comments ignored, the
// same shape scripts/bots-isolation-allowlist.txt already uses. It returns
// one violation string per problem found: a qualifying read missing from
// the allow-list, or an allow-list entry naming a query that does not
// exist. A nil/empty result with a nil error means the package is clean.
func checkProjectionReadAllowlist(queriesDir, allowlistPath string) ([]string, error) {
	blocks, err := parseProjectionQueryBlocks(queriesDir)
	if err != nil {
		return nil, err
	}

	allowlistData, err := os.ReadFile(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("read allow-list %q: %w", allowlistPath, err)
	}
	allowed := make(map[string]bool)
	for _, line := range strings.Split(string(allowlistData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed[line] = true
	}

	known := make(map[string]bool, len(blocks))
	var reads []string
	for _, b := range blocks {
		known[b.name] = true
		if isProjectionRead(b.body) {
			reads = append(reads, b.name)
		}
	}

	var violations []string
	for _, name := range reads {
		if !allowed[name] {
			violations = append(violations, fmt.Sprintf(
				"query %q reads events or match_summary but is not on the allow-list %q", name, allowlistPath))
		}
	}
	for name := range allowed {
		if !known[name] {
			violations = append(violations, fmt.Sprintf(
				"allow-list %q names query %q, which no longer exists", allowlistPath, name))
		}
	}
	sort.Strings(violations) // deterministic failure output
	return violations, nil
}

// TestProjectionReadsAreAllowlisted is the real gate: every query this
// package actually ships must satisfy checkProjectionReadAllowlist against
// the real allow-list. Today that list is empty and this package ships no
// read of either table (M3's own scope is the writers), so this currently
// passes vacuously — the fixture tests below are what prove the mechanism
// itself, rather than trusting an always-clean real case to have exercised
// it.
func TestProjectionReadsAreAllowlisted(t *testing.T) {
	violations, err := checkProjectionReadAllowlist("queries", "testdata/projections-read-allowlist.txt")
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

func writeQueryFixture(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

// TestProjectionReadAllowlistFailsClosedOnUnlistedRead is the acceptance
// criterion's own fail-closed case, built rather than assumed: "the check
// fails when the allow-list is empty and the package contains reads."
func TestProjectionReadAllowlistFailsClosedOnUnlistedRead(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", "-- name: ListEventsForMatch :many\nSELECT * FROM events WHERE match_id = $1;\n")
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an unlisted SELECT against events with an empty allow-list, got none")
	}
}

// TestProjectionReadAllowlistFailsClosedOnStaleEntry is the acceptance
// criterion's other named case: "fails when an allow-list entry names a
// query that no longer exists" — a stale allow-list that matches nothing
// must not read as clean.
func TestProjectionReadAllowlistFailsClosedOnStaleEntry(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", "-- name: DeleteEventsByMatch :exec\nDELETE FROM events WHERE match_id = $1;\n")
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "ListEventsForMatch\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an allow-list entry naming a nonexistent query, got none")
	}
}

// TestProjectionReadAllowlistPassesWhenListed is the positive case: a
// qualifying read whose name is on the allow-list produces no violation.
func TestProjectionReadAllowlistPassesWhenListed(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", "-- name: ListEventsForMatch :many\nSELECT * FROM events WHERE match_id = $1;\n")
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "ListEventsForMatch\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want no violations, got %v", violations)
	}
}

// TestProjectionReadAllowlistIgnoresReturningWrites proves the
// classification boundary itself: an INSERT ... ON CONFLICT ... RETURNING
// (UpsertMatchSummary's own real shape) is a write, not a read, so it needs
// no allow-list entry at all, even though its comment could easily mention
// the words "select" or the table name.
func TestProjectionReadAllowlistIgnoresReturningWrites(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "match_summary.sql", strings.Join([]string{
		"-- name: UpsertMatchSummary :one",
		"INSERT INTO match_summary (match_id, round, submitted_seats) VALUES ($1, $2, $3)",
		"ON CONFLICT (match_id, round) DO UPDATE SET submitted_seats = excluded.submitted_seats",
		"RETURNING *;",
		"",
	}, "\n"))
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("an INSERT ... RETURNING is a write, not a read; want no violations, got %v", violations)
	}
}

// TestProjectionReadAllowlistFailsClosedOnMissingQueriesDir is the same
// "unreadable input is a failure, not a vacuous pass" discipline
// purity_test.go and sqlc_generate_test.go already hold this package to.
func TestProjectionReadAllowlistFailsClosedOnMissingQueriesDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := checkProjectionReadAllowlist(missing, "testdata/projections-read-allowlist.txt"); err == nil {
		t.Fatal("want an error reading a nonexistent queries dir, got nil")
	}
}
