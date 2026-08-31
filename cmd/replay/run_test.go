package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/match/fold"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestRunRejectsNoDataSource is #322's own input-selection contract:
// exactly one of --bundle or (--db and --match) must be given.
func TestRunRejectsNoDataSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with no data source = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--bundle") {
		t.Errorf("stderr = %q, want it to mention --bundle", stderr.String())
	}
}

// TestRunRejectsBundleCombinedWithDB asserts --bundle and --db/--match are
// mutually exclusive, not silently resolved by one taking priority.
func TestRunRejectsBundleCombinedWithDB(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", "x.json", "--db", "postgres://x", "--match", "m"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with --bundle and --db/--match = %d, want 1", code)
	}
}

// TestRunRejectsPartialDBFlags asserts --db without --match (and vice
// versa) is rejected rather than silently treated as --bundle's absence.
func TestRunRejectsPartialDBFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", "postgres://x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with only --db = %d, want 1", code)
	}
}

// TestRunRejectsExportBundleWithoutDB: --export-bundle needs a match to
// export from, so it cannot be combined with --bundle or given alone.
func TestRunRejectsExportBundleWithoutDB(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", "x.json", "--export-bundle", "out.json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with --export-bundle and --bundle = %d, want 1", code)
	}
}

// TestRunRejectsRoundBelowOne: rounds are 1-indexed (GDD §4); --round 0 or
// negative is a caller error, not silently treated as "unset".
func TestRunRejectsRoundBelowOne(t *testing.T) {
	path := writeTestBundle(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", path, "--round", "-1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() with --round -1 = %d, want 1", code)
	}
}

// TestRunBundleByteIdenticalAcrossRuns is #322's acceptance criterion:
// "two runs produce byte-identical output."
//
// Fails closed: a non-empty byte count alone would also be true of a
// vacuous MatchState{} zero value marshalled to JSON, so this decodes the
// dump and asserts two concrete, deterministic facts a real 15-round fold
// of testFixture's idle-order log produces and an empty/short-circuited
// fold could not: the final Round actually reached cfg.Rounds, and seat 0
// — who never moves for the whole match — has accumulated
// LoiteringStreak == 11 (GDD §9.1's "silent" streak, uninterrupted since
// round 5, the first round idle movement can register as loitering rather
// than a fresh arrival). This is the closest analogue this dump shape has
// to "a specific round's known event appears in it": MatchState carries no
// event list (Fold's own null sink discards events), so a known,
// hand-verified Player fact stands in for a known event.
func TestRunBundleByteIdenticalAcrossRuns(t *testing.T) {
	cfg, _, _, _ := testFixture()
	path := writeTestBundle(t)

	var out1, out2, stderr bytes.Buffer
	if code := run([]string{"--bundle", path}, &out1, &stderr); code != 0 {
		t.Fatalf("run() #1 = %d, stderr = %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"--bundle", path}, &out2, &stderr); code != 0 {
		t.Fatalf("run() #2 = %d, stderr = %s", code, stderr.String())
	}

	if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
		t.Fatal("two runs against the same bundle produced different bytes")
	}

	var state rules.MatchState
	if err := json.Unmarshal(out1.Bytes(), &state); err != nil {
		t.Fatalf("decode dump: %v", err)
	}
	if state.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("dumped Round = %d, want %d (cfg.Rounds) — a vacuous fold would report 0", state.Round, cfg.Rounds)
	}
	if len(state.Players) != 2 || state.Players[0].LoiteringStreak != 11 {
		t.Fatalf("dumped seat 0 LoiteringStreak = %+v, want 11 — this exact figure only comes from actually folding all %d rounds of testFixture's idle-order log", state.Players, cfg.Rounds)
	}
}

// TestRunFailsClosedTwoErrorRunsAreByteIdentical is #322's fails-closed
// acceptance criterion applied to the error path: two runs that both fail
// to load the match must also be byte-identical, not merely both non-zero.
func TestRunFailsClosedTwoErrorRunsAreByteIdentical(t *testing.T) {
	var out1, err1, out2, err2 bytes.Buffer
	code1 := run([]string{"--bundle", "/nonexistent/path/does-not-exist.json"}, &out1, &err1)
	code2 := run([]string{"--bundle", "/nonexistent/path/does-not-exist.json"}, &out2, &err2)

	if code1 != code2 {
		t.Fatalf("exit codes differ: %d vs %d", code1, code2)
	}
	if code1 == 0 {
		t.Fatal("run() with a nonexistent bundle path succeeded, want an error")
	}
	if !bytes.Equal(err1.Bytes(), err2.Bytes()) {
		t.Fatalf("stderr differs across two failed runs:\n%s\nvs\n%s", err1.String(), err2.String())
	}
	if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
		t.Fatal("stdout differs across two failed runs")
	}
}

// TestRunRoundBeyondLastRoundIsErrorNamingLastRound is #322's own
// acceptance criterion: "--round N beyond the match's last round is an
// error naming the last round, not a silently clamped dump."
func TestRunRoundBeyondLastRoundIsErrorNamingLastRound(t *testing.T) {
	path := writeTestBundle(t)
	cfg, _, _, _ := testFixture()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", path, "--round", "999"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("run() with --round beyond the match's last round succeeded, want an error")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty on error: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), strconv.Itoa(cfg.Rounds)) {
		t.Errorf("stderr = %q, want it to name the match's actual last round (%d)", stderr.String(), cfg.Rounds)
	}
}

// TestRunSeatPrintsPlayerViewOnly is #322's acceptance criterion: "--seat N
// prints a PlayerView and nothing else."
func TestRunSeatPrintsPlayerViewOnly(t *testing.T) {
	path := writeTestBundle(t)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--bundle", path, "--seat", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}

	var view game.PlayerView
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("--seat output does not decode as a game.PlayerView: %v", err)
	}

	// A full rules.MatchState dump has top-level keys ("Graph", "Players",
	// "RoundAnchors") that game.PlayerView does not — DisallowUnknownFields
	// against PlayerView's own shape is the structural half of "and nothing
	// else": any of those keys present would fail this decode.
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	dec.DisallowUnknownFields()
	var strict game.PlayerView
	if err := dec.Decode(&strict); err != nil {
		t.Fatalf("--seat output has fields beyond game.PlayerView's own shape: %v", err)
	}
}

// TestRunSeatOutOfRangeIsError asserts a seat index outside the match's own
// player count is rejected rather than panicking or silently clamped.
func TestRunSeatOutOfRangeIsError(t *testing.T) {
	path := writeTestBundle(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", path, "--seat", "7"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("run() with an out-of-range --seat succeeded, want an error")
	}
}

// TestRunSeatBelowNegativeOneIsError is a CodeRabbit review finding on PR
// #404: -1 is the documented "no seat" default (the full-state dump), but
// anything below it (e.g. --seat=-2) was falling through the same `else`
// branch as -1 and silently emitting the full, un-fogged MatchState for an
// explicitly invalid seat instead of erroring — never touching, let alone
// bypassing, fog filtering, but wrong regardless: an invalid flag value
// must be rejected, not treated as "unset."
func TestRunSeatBelowNegativeOneIsError(t *testing.T) {
	path := writeTestBundle(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bundle", path, "--seat", "-2"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("run() with --seat=-2 succeeded, want an error")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty on error: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--seat") {
		t.Errorf("stderr = %q, want it to mention --seat", stderr.String())
	}
}

// TestRunSeatFogNegativeAssertion is #322's own fog acceptance criterion,
// RFC §16.3's negative-assertion standard applied to the CLI's output: "a
// test asserts a fact hidden from that seat (a rival's position on a
// FogHidden node) is absent from the bytes."
//
// testFixture's whole match is idle orders — nobody moves — so seat 0
// never discovers most of the map, including (with very high probability
// for a real generated topology) wherever seat 1 actually starts. This
// folds the fixture directly (bypassing the CLI) to read the raw,
// unfiltered rules.MatchState and find seat 1's true position and a node
// still Hidden to seat 0, then asserts run()'s own --seat 0 output — the
// bytes a bug reporter would actually receive — never names that node.
func TestRunSeatFogNegativeAssertion(t *testing.T) {
	cfg, seed, players, log := testFixture()

	state, _, err := fold.Fold(seed, cfg, players, log)
	if err != nil {
		t.Fatalf("fold.Fold: %v", err)
	}

	rival := state.Players[1]
	seat0Fog := state.Players[0].Fog
	if int(rival.Position) >= len(seat0Fog) || seat0Fog[rival.Position] != game.FogHidden {
		t.Skipf("fixture's rival position (node %d) is not FogHidden to seat 0 under this seed/config; fixture needs a different seed for this assertion", rival.Position)
	}

	path := writeTestBundle(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--bundle", path, "--seat", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}

	var view game.PlayerView
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode --seat output: %v", err)
	}
	if _, present := view.Nodes[rival.Position]; present {
		t.Fatalf("seat 0's dump names node %d (Hidden to seat 0, and the rival's true position) — RFC §9.1's absent-key rule is violated", rival.Position)
	}

	// Belt and braces: the node's own key, formatted the way encoding/json
	// would emit a map[game.NodeID]... key, must not appear in the raw
	// bytes either — catching a leak that landed somewhere other than
	// PlayerView.Nodes (Trail, Anchors, a future field) that the typed
	// decode above wouldn't structurally rule out.
	needle := []byte(`"` + strconv.Itoa(int(rival.Position)) + `":`)
	if bytes.Contains(stdout.Bytes(), needle) {
		t.Fatalf("seat 0's raw dump bytes contain a %s object key — the rival's hidden node leaked somewhere the typed check above didn't catch", needle)
	}
}

// TestRunDefaultDumpIsFullMatchState asserts the no-flags default prints
// the full rules.MatchState (round, graph, players — not the fog-filtered
// PlayerView shape), matching cmd/replay/doc.go's own documented default.
func TestRunDefaultDumpIsFullMatchState(t *testing.T) {
	path := writeTestBundle(t)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--bundle", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}

	var state rules.MatchState
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		t.Fatalf("default dump does not decode as a rules.MatchState: %v", err)
	}
	if len(state.Players) != 2 {
		t.Errorf("decoded MatchState has %d players, want 2", len(state.Players))
	}
}
