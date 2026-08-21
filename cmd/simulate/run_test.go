package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/telemetry"
)

// fakeGitSHA stands in for the real `git rev-parse HEAD` call in every
// test below except the ones checking that provenance travels correctly —
// the real command works fine inside this repo, but pinning it keeps
// these tests independent of which commit happens to be checked out.
func fakeGitSHA() (string, error) { return "deadbeef", nil }

// realArgs is the RFC §16.4 invocation shape at a size small enough to
// run as a unit test: 2 sweep points, 6 matches each, Drifter (the
// cheapest tier), 2 players.
func realArgs(out string) []string {
	return []string{
		"--matches", "6",
		"--players", "2",
		"--bots", "drifter",
		"--sweep", "LeaseCostPerBlock=2,3",
		"--out", out,
	}
}

func readDataRows(t *testing.T, path string) []map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%s) = %v", path, err)
	}
	defer func() { _ = f.Close() }()

	br := bufio.NewReader(f)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatalf("read provenance line: %v", err)
	}
	r := csv.NewReader(br)
	header, err := r.Read()
	if err != nil {
		t.Fatalf("read header: %v", err)
	}

	var rows []map[string]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read row: %v", err)
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			row[h] = rec[i]
		}
		rows = append(rows, row)
	}
	return rows
}

func TestRunHappyPath(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sweep.csv")
	var stderr bytes.Buffer

	code := runWithDeps(realArgs(out), &stderr, RunMany, fakeGitSHA)
	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	rows := readDataRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row["status"] != "ok" {
			t.Errorf("row status = %q, want ok (error=%q)", row["status"], row["error"])
		}
		if row["matches_completed"] != "6" {
			t.Errorf("matches_completed = %q, want 6", row["matches_completed"])
		}
		if row["git_sha"] != "deadbeef" {
			t.Errorf("git_sha = %q, want deadbeef", row["git_sha"])
		}
		if row["config_json"] == "" {
			t.Error("config_json is empty")
		}
	}
}

// TestRunZeroWidthIntervalOmitsHalfWidth is issue #249: a metric that comes
// back identical across every match in a configuration — here,
// DeliveriesPerPlayer, a floatMetric that is never excluded — must not
// write a "_half_width" that reads as a measurement of zero uncertainty.
// The mean is still worth keeping (it is the constant value itself), so it
// travels while its half-width sibling stays empty.
func TestRunZeroWidthIntervalOmitsHalfWidth(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sweep.csv")
	var stderr bytes.Buffer

	fake := func(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
		results := make([]MatchResult, len(seeds))
		for i := range results {
			results[i] = MatchResult{
				Seed:    seeds[i],
				Players: players,
				Summary: telemetry.MatchSummary{DeliveriesPerPlayer: 2.5},
			}
		}
		return results, nil
	}

	code := runWithDeps(realArgs(out), &stderr, fake, fakeGitSHA)
	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	for _, row := range readDataRows(t, out) {
		if got := row["DeliveriesPerPlayer_mean"]; got != "2.500000" {
			t.Errorf("DeliveriesPerPlayer_mean = %q, want 2.500000 (the constant value, kept per #249)", got)
		}
		if got := row["DeliveriesPerPlayer_half_width"]; got != "" {
			t.Errorf("DeliveriesPerPlayer_half_width = %q, want empty (zero-width interval is not a measurement)", got)
		}
		if got := row["DeliveriesPerPlayer_n"]; got != "6" {
			t.Errorf("DeliveriesPerPlayer_n = %q, want 6", got)
		}
	}
}

// TestRunByteIdenticalAcrossRuns is issue #200's own acceptance criterion:
// "Two runs of the same invocation produce byte-identical CSVs."
func TestRunByteIdenticalAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	out1 := filepath.Join(dir, "a.csv")
	out2 := filepath.Join(dir, "b.csv")
	var stderr bytes.Buffer

	if code := runWithDeps(realArgs(out1), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("run() (1) = %d; stderr:\n%s", code, stderr.String())
	}
	stderr.Reset()
	if code := runWithDeps(realArgs(out2), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("run() (2) = %d; stderr:\n%s", code, stderr.String())
	}

	b1, err := os.ReadFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Error("two fresh runs of the same invocation produced different bytes")
	}
}

// TestRunResumeProducesSameFileAsUninterrupted is issue #200's own
// acceptance criterion: "Resuming an interrupted sweep produces the same
// file as running it uninterrupted." The interruption is simulated by
// truncating a completed run's own file down to its provenance line,
// header line, and first data row — exactly the shape a process killed
// right after flushing the first configuration's row would leave behind
// (checkpoint granularity is one configuration, per csvReport's own doc
// comment).
func TestRunResumeProducesSameFileAsUninterrupted(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "full.csv")
	resumed := filepath.Join(dir, "resumed.csv")
	var stderr bytes.Buffer

	if code := runWithDeps(realArgs(full), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("run() (uninterrupted) = %d; stderr:\n%s", code, stderr.String())
	}

	fullBytes, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(fullBytes), "\n")
	if len(lines) < 4 {
		t.Fatalf("full run produced too few lines: %d", len(lines))
	}
	partial := strings.Join(lines[:3], "") // provenance + header + row 1
	if err := os.WriteFile(resumed, []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr.Reset()
	if code := runWithDeps(realArgs(resumed), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("run() (resume) = %d; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "already recorded, skipping") {
		t.Errorf("resume's stderr did not report a skip:\n%s", stderr.String())
	}

	resumedBytes, err := os.ReadFile(resumed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fullBytes, resumedBytes) {
		t.Errorf("resumed file differs from an uninterrupted run's file:\n--- full ---\n%s\n--- resumed ---\n%s", fullBytes, resumedBytes)
	}
}

// TestRunResumeHeaderMismatchExitsNonZero is issue #200's own acceptance
// criterion: "resuming against a file whose header disagrees exits
// non-zero."
func TestRunResumeHeaderMismatchExitsNonZero(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sweep.csv")
	if err := os.WriteFile(out, []byte("# cinzal-simulate v1 seed=00 matches=6 players=2 bots=drifter sweep=LeaseCostPerBlock:2,3\nstatus,error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := runWithDeps(realArgs(out), &stderr, RunMany, fakeGitSHA)
	if code == 0 {
		t.Error("run() = 0, want non-zero for a resume whose header disagrees")
	}
}

// TestRunErrorRowContinuesSweepAndExitsNonZero is issue #200's own
// fails-closed acceptance criterion: "a configuration whose matches did
// not all reach cfg.Rounds is written as an error row, not omitted and
// not averaged over the survivors" — and the sweep keeps going rather
// than aborting on the first bad configuration.
func TestRunErrorRowContinuesSweepAndExitsNonZero(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sweep.csv")
	var stderr bytes.Buffer

	calls := 0
	fake := func(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
		calls++
		if cfg.LeaseCostPerBlock == 2 {
			return nil, errors.New("synthetic failure for LeaseCostPerBlock=2")
		}
		results := make([]MatchResult, len(seeds))
		for i := range results {
			results[i] = MatchResult{Seed: seeds[i], Players: players}
		}
		return results, nil
	}

	code := runWithDeps(realArgs(out), &stderr, fake, fakeGitSHA)
	if code != 1 {
		t.Fatalf("run() = %d, want 1 (one configuration errored)", code)
	}
	if calls != 2 {
		t.Fatalf("matchRunner called %d times, want 2 — an errored configuration must not stop the sweep", calls)
	}

	rows := readDataRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (both configurations wrote a row)", len(rows))
	}
	var errRow, okRow map[string]string
	for _, row := range rows {
		if row["sweep.LeaseCostPerBlock"] == "2" {
			errRow = row
		} else {
			okRow = row
		}
	}
	if errRow == nil || errRow["status"] != "error" || errRow["error"] == "" {
		t.Errorf("errRow = %+v, want status=error with a non-empty error message", errRow)
	}
	if okRow == nil || okRow["status"] != "ok" {
		t.Errorf("okRow = %+v, want status=ok", okRow)
	}
}

// TestRunResumeWithPriorErrorRowExitsNonZero is the CodeRabbit-flagged bug
// fixed via csvReport.hadPriorError: a resumed run over a file that
// already holds an error-status row must not report success just because
// every configuration in it is already recorded (and therefore skipped).
// panicRunner proves the failed configuration is genuinely skipped, not
// retried — retrying it could only reproduce the same deterministic
// failure, so resuming spends no compute re-confirming that.
func TestRunResumeWithPriorErrorRowExitsNonZero(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sweep.csv")
	var stderr bytes.Buffer

	failing := func(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
		if cfg.LeaseCostPerBlock == 2 {
			return nil, errors.New("synthetic failure for LeaseCostPerBlock=2")
		}
		return make([]MatchResult, len(seeds)), nil
	}
	if code := runWithDeps(realArgs(out), &stderr, failing, fakeGitSHA); code != 1 {
		t.Fatalf("first run = %d, want 1", code)
	}

	stderr.Reset()
	code := runWithDeps(realArgs(out), &stderr, panicRunner, fakeGitSHA)
	if code != 1 {
		t.Errorf("resumed run over a file with a prior error row = %d, want 1 (must not report success)", code)
	}
	if !strings.Contains(stderr.String(), "already recorded, skipping") {
		t.Errorf("resume did not skip the already-recorded configurations:\n%s", stderr.String())
	}
}

func panicRunner(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
	panic("matchRunner should not be called when validation fails before any match runs")
}

func panicGitSHA() (string, error) {
	panic("getGitSHA should not be called when validation fails before any match runs")
}

// TestRunFailsClosedBeforeAnyWork is issue #200's own acceptance
// criterion: "--matches 0, an empty sweep list, and a config that fails
// Config.Validate() all exit non-zero with a message naming the problem"
// — checked here as "before any match runs," using panicRunner/
// panicGitSHA to prove it, not just asserting the exit code.
func TestRunFailsClosedBeforeAnyWork(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"matches zero", []string{"--matches", "0", "--players", "2", "--bots", "drifter", "--sweep", "LeaseCostPerBlock=1", "--out", "x.csv"}},
		{"empty sweep", []string{"--matches", "5", "--players", "2", "--bots", "drifter", "--out", "x.csv"}},
		{"config fails Validate", []string{"--matches", "5", "--players", "2", "--bots", "drifter", "--sweep", "Rounds=0", "--out", "x.csv"}},
		{"unknown bots tier", []string{"--matches", "5", "--players", "2", "--bots", "nonsense", "--sweep", "LeaseCostPerBlock=1", "--out", "x.csv"}},
		{"players zero", []string{"--matches", "5", "--players", "0", "--bots", "drifter", "--sweep", "LeaseCostPerBlock=1", "--out", "x.csv"}},
		{"missing out", []string{"--matches", "5", "--players", "2", "--bots", "drifter", "--sweep", "LeaseCostPerBlock=1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{}, tt.args...)
			if idx := slices.Index(args, "x.csv"); idx != -1 {
				args[idx] = filepath.Join(t.TempDir(), "x.csv")
			}
			var stderr bytes.Buffer
			code := runWithDeps(args, &stderr, panicRunner, panicGitSHA)
			if code == 0 {
				t.Errorf("run() = 0, want non-zero; stderr:\n%s", stderr.String())
			}
			if stderr.Len() == 0 {
				t.Error("stderr is empty, want a message naming the problem")
			}
		})
	}
}
