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
		for line := range strings.SplitSeq(string(data), "\n") {
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
	leadingSelectOrWithRe = regexp.MustCompile(`(?is)^\s*(SELECT|WITH)\b`)
	// tableRefRe matches either table named after FROM or JOIN — the two
	// clauses SQL uses to pull rows from a table — so a plain single-table
	// SELECT, a JOIN read, and a CTE's own inner SELECT (WITH e AS (SELECT
	// * FROM events) SELECT * FROM e) are all recognized alike: the CTE
	// case matches because its defining SELECT still says "FROM events"
	// even though the outer statement's own FROM clause names the CTE, not
	// the table.
	tableRefRe = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(events|match_summary)\b`)
	// fromClauseRe captures an entire comma-separated FROM list verbatim —
	// the legacy implicit-join form "FROM matches, events" — from the FROM
	// keyword up to whatever ends the list: an explicit JOIN, WHERE, GROUP
	// BY, HAVING, ORDER BY, LIMIT, WINDOW, UNION, FOR UPDATE/SHARE, a
	// closing paren, a semicolon, or end of string. It hands the raw list
	// text to fromListTables for per-item parsing rather than trying to
	// validate each item's shape itself, so a table named after a comma —
	// with or without an alias — is still caught. Scoped to the FROM
	// clause itself rather than any comma in the query, so a column named
	// "events" in a SELECT list doesn't false-positive. tableRefRe already
	// handles a single FROM/JOIN table correctly even with an alias (FROM
	// events AS e still matches "events", since the alias trails the
	// matched word); this pair exists only for the multi-table list form.
	fromClauseRe = regexp.MustCompile(`(?is)\bFROM\s+(.*?)(?:\bJOIN\b|\bWHERE\b|\bGROUP\s+BY\b|\bHAVING\b|\bORDER\s+BY\b|\bLIMIT\b|\bWINDOW\b|\bUNION\b|\bFOR\s+(?:UPDATE|SHARE)\b|[;)]|$)`)
	// fromListItemTableRe extracts the leading table identifier from one
	// comma-separated FROM-list item, stopping at the first character that
	// can't be part of a (possibly schema-qualified) identifier — which is
	// exactly where an "AS <alias>" or a bare trailing alias begins, so
	// both are discarded for free: "events AS e", "events e", and bare
	// "events" all yield "events".
	fromListItemTableRe = regexp.MustCompile(`(?i)^([a-zA-Z_][a-zA-Z0-9_.]*)`)
)

// fromListTables splits a FROM clause (as captured by fromClauseRe) on its
// top-level commas and returns the bare table name for each item, with any
// "AS <alias>"/bare trailing alias and any schema qualifier (public.events
// -> events) stripped.
func fromListTables(clause string) []string {
	var tables []string
	for item := range strings.SplitSeq(clause, ",") {
		name := fromListItemTableRe.FindString(strings.TrimSpace(item))
		if name == "" {
			continue
		}
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		tables = append(tables, strings.ToLower(name))
	}
	return tables
}

// isProjectionRead reports whether body is a genuine read of events or
// match_summary: its leading statement is SELECT or WITH (a CTE), and
// somewhere in the body a FROM or JOIN clause names one of the two tables —
// directly (FROM events, JOIN events), inside a comma-separated FROM list
// (FROM matches, events), or inside a CTE's own inner SELECT. An INSERT ...
// ON CONFLICT ... RETURNING (UpsertMatchSummary, UpsertOrder, CreateMatch,
// CreateMatchPlayer elsewhere in this package) is a write that merely
// returns the row just written — its leading keyword is INSERT, not SELECT
// or WITH — so it is deliberately not classified as a read here.
func isProjectionRead(body string) bool {
	if !leadingSelectOrWithRe.MatchString(body) {
		return false
	}
	if tableRefRe.MatchString(body) {
		return true
	}
	for _, m := range fromClauseRe.FindAllStringSubmatch(body, -1) {
		for _, tbl := range fromListTables(m[1]) {
			switch tbl {
			case "events", "match_summary":
				return true
			}
		}
	}
	return false
}

// allowlistEntryRe matches one structured allow-list line: a query name, a
// caller, and a safety justification, separated by "|". All three fields
// are mandatory and must be non-blank after trimming — issue #321's
// acceptance criterion is "an allow-list with a stated caller," and a bare
// query name states no caller at all. scripts/bots-isolation-allowlist.txt
// carries a caller/justification too, but as a free-standing comment above
// the identifier line that nothing parses or enforces; this format instead
// makes the caller and justification part of the parsed entry itself, so a
// future entry that omits either is rejected rather than silently accepted.
var allowlistEntryRe = regexp.MustCompile(`^([^|]+)\|([^|]+)\|([^|]+)$`)

// parseAllowlistEntry parses one non-blank, non-comment allow-list line
// into its query/caller/justification fields. Returns an error for a bare
// query name, a line missing one or more "|" separators, or any field that
// is empty after trimming whitespace — malformed input is rejected, not
// coerced into a best guess.
func parseAllowlistEntry(line string) (query, caller, justification string, err error) {
	m := allowlistEntryRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", "", fmt.Errorf("want \"query | caller | justification\", got %q", line)
	}
	query = strings.TrimSpace(m[1])
	caller = strings.TrimSpace(m[2])
	justification = strings.TrimSpace(m[3])
	if query == "" || caller == "" || justification == "" {
		return "", "", "", fmt.Errorf("query, caller, and justification must all be non-empty, got %q", line)
	}
	return query, caller, justification, nil
}

// checkProjectionReadAllowlist enumerates every query under queriesDir,
// classifies which are genuine reads of events/match_summary
// (isProjectionRead), and cross-checks that set against allowlistPath — one
// structured "query | caller | justification" entry per line
// (parseAllowlistEntry), blank lines and #-comments ignored. It returns one
// violation string per problem found: a qualifying read missing from the
// allow-list, an allow-list entry naming a query that does not exist, or a
// malformed allow-list line (missing a field, or a bare query name with no
// caller/justification at all). A nil/empty result with a nil error means
// the package is clean.
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
	var malformed []string
	for line := range strings.SplitSeq(string(allowlistData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		query, _, _, err := parseAllowlistEntry(line)
		if err != nil {
			malformed = append(malformed, fmt.Sprintf(
				"allow-list %q: malformed entry: %v", allowlistPath, err))
			continue
		}
		allowed[query] = true
	}

	known := make(map[string]bool, len(blocks))
	var reads []string
	for _, b := range blocks {
		known[b.name] = true
		if isProjectionRead(b.body) {
			reads = append(reads, b.name)
		}
	}

	violations := append([]string(nil), malformed...)
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
	writeQueryFixture(t, dir, "allowlist.txt", "ListEventsForMatch | cmd/replay | test fixture, not a real caller\n")

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
	writeQueryFixture(t, dir, "allowlist.txt", "ListEventsForMatch | cmd/replay | test fixture, not a real caller\n")

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

// TestProjectionReadAllowlistDetectsCTERead proves isProjectionRead's CTE
// case: a query whose outer statement is "WITH ... SELECT" and whose
// defining sub-SELECT reads events in a FROM clause must still be
// classified as a read, even though the outer statement's own FROM clause
// names the CTE (e) rather than the table. Run against an empty allow-list,
// so a classifier that missed this form would pass here — the same bug
// the finding this test exists for described.
func TestProjectionReadAllowlistDetectsCTERead(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", strings.Join([]string{
		"-- name: ListRecentEventNames :many",
		"WITH e AS (SELECT * FROM events WHERE match_id = $1)",
		"SELECT kind FROM e;",
		"",
	}, "\n"))
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an unlisted CTE read of events with an empty allow-list, got none")
	}
}

// TestProjectionReadAllowlistDetectsJoinRead is TestProjectionReadAllowlistDetectsCTERead's
// counterpart for a JOIN read: events named after JOIN rather than FROM
// must still be classified as a read.
func TestProjectionReadAllowlistDetectsJoinRead(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", strings.Join([]string{
		"-- name: ListMatchesWithEvents :many",
		"SELECT matches.id FROM matches JOIN events ON events.match_id = matches.id;",
		"",
	}, "\n"))
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an unlisted JOIN read of events with an empty allow-list, got none")
	}
}

// TestProjectionReadAllowlistDetectsCommaJoinRead is
// TestProjectionReadAllowlistDetectsJoinRead's counterpart for the legacy
// implicit-join form: events named in a comma-separated FROM list (FROM
// matches, events), rather than after an explicit FROM or JOIN keyword,
// must still be classified as a read. This is the form tableRefRe alone
// missed — "events" here is preceded by "," not FROM or JOIN.
func TestProjectionReadAllowlistDetectsCommaJoinRead(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", strings.Join([]string{
		"-- name: ListMatchesWithEventsCommaJoin :many",
		"SELECT matches.id FROM matches, events WHERE events.match_id = matches.id;",
		"",
	}, "\n"))
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an unlisted comma-join read of events with an empty allow-list, got none")
	}
}

// TestProjectionReadAllowlistDetectsAliasedCommaJoinRead is
// TestProjectionReadAllowlistDetectsCommaJoinRead's counterpart when both
// sides of the comma-separated FROM list carry an explicit alias: an
// aliased non-projection table (matches AS m) precedes an aliased
// projection table (events AS e). fromListRe used to stop at "matches" —
// the " AS m" text broke its bare-identifier-list shape — so the aliased
// "events" item was never reached at all.
func TestProjectionReadAllowlistDetectsAliasedCommaJoinRead(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", strings.Join([]string{
		"-- name: ListMatchesWithEventsAliasedCommaJoin :many",
		"SELECT matches.id FROM matches AS m, events AS e WHERE events.match_id = matches.id;",
		"",
	}, "\n"))
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "# nothing allowed\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for an unlisted aliased comma-join read of events with an empty allow-list, got none")
	}
}

// TestProjectionReadAllowlistRejectsBareQueryNameEntry is the allow-list
// parser's own fail-closed case: a bare query name with no stated caller or
// justification must be rejected as malformed, not silently accepted as an
// approval. Uses a query that genuinely reads events, on an allow-list
// containing only its bare name, so the only way this test could pass by
// accident is if the bare entry were wrongly treated as valid.
func TestProjectionReadAllowlistRejectsBareQueryNameEntry(t *testing.T) {
	dir := t.TempDir()
	writeQueryFixture(t, dir, "events.sql", "-- name: ListEventsForMatch :many\nSELECT * FROM events WHERE match_id = $1;\n")
	allowlist := filepath.Join(dir, "allowlist.txt")
	writeQueryFixture(t, dir, "allowlist.txt", "ListEventsForMatch\n")

	violations, err := checkProjectionReadAllowlist(dir, allowlist)
	if err != nil {
		t.Fatalf("checkProjectionReadAllowlist: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("want a violation for a bare query-name entry with no caller or justification, got none")
	}
}
