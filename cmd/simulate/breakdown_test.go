package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/telemetry"
)

// breakdownMatches runs a handful of real matches, the same way a sweep
// does, so the invariant tests below check the shipped computation rather
// than a hand-built fixture that could agree with it and both be wrong.
func breakdownMatches(t *testing.T, players int, tier bots.Tier, n int) ([]MatchResult, game.Config) {
	t.Helper()
	cfg := game.DefaultConfig()
	seeds := make([][32]byte, n)
	for i := range seeds {
		seeds[i] = sha256.Sum256(binary.BigEndian.AppendUint64([]byte("cinzal-breakdown-test"), uint64(i)))
	}
	results, err := RunMany(seeds, cfg, players, bots.For(tier), 1)
	if err != nil {
		t.Fatalf("RunMany: %v", err)
	}
	return results, cfg
}

// TestBreakdownAgreesWithTelemetry is the check that keeps this file
// honest. Breakdown splits three GDD §22 rows that internal/telemetry also
// computes whole, which means three definitions now exist twice. A split
// that quietly disagrees with the row it splits would produce a
// demonstration whose supporting detail contradicts its own verdict, so
// every split is asserted to sum back to telemetry's own number.
func TestBreakdownAgreesWithTelemetry(t *testing.T) {
	results, _ := breakdownMatches(t, 3, bots.Operator, 40)

	for i, res := range results {
		b := res.Breakdown

		submitted, halted := 0, 0
		for r := range b.RoutesSubmittedByRound {
			submitted += b.RoutesSubmittedByRound[r]
			halted += b.RoutesHaltedByRound[r]
		}
		if got, want := submitted, res.Summary.RoutesCancelledMidRoute.N; got != want {
			t.Errorf("match %d: routes submitted summed by round = %d, telemetry row 1's N = %d", i, got, want)
		}
		wantHalted := int(res.Summary.RoutesCancelledMidRoute.Value*float64(submitted) + 0.5)
		if halted != wantHalted {
			t.Errorf("match %d: routes halted summed by round = %d, telemetry row 1's numerator = %d", i, halted, wantHalted)
		}

		if got, want := len(b.IncidentLive), res.Summary.SectorIncidentsHittingAPlayer.N; got != want {
			t.Errorf("match %d: live incident cards = %d, telemetry row 6's N = %d", i, got, want)
		}
		wantHits := int(res.Summary.SectorIncidentsHittingAPlayer.Value*float64(len(b.IncidentLive)) + 0.5)
		if got := len(b.IncidentHit); got != wantHits {
			t.Errorf("match %d: incident cards that hit = %d, telemetry row 6's numerator = %d", i, got, wantHits)
		}

		if got, want := b.AnyPlayerFinallyReachedInfamy9, res.Summary.AnyPlayerReachedInfamy9; got != want {
			t.Errorf("match %d: AnyPlayerFinallyReachedInfamy9 = %v, telemetry row 4 = %v", i, got, want)
		}
	}
}

// TestAggregateBreakdownsReproducesTheTelemetryRowsItSplits is the
// assertion the exit demonstration's own provenance section leans on. The
// per-match check above proves the raw counts agree; this proves the whole
// pipeline agrees — the same matches, reduced by the same D35 interval
// routine, must produce identical numbers whether they arrive through
// telemetry.MatchSummary and metricSpecs or through Breakdown and
// aggregateBreakdowns. The three rows compared are exactly the three the
// write-up quotes as headline figures.
func TestAggregateBreakdownsReproducesTheTelemetryRowsItSplits(t *testing.T) {
	results, cfg := breakdownMatches(t, 4, bots.Operator, 60)

	summaries := make([]telemetry.MatchSummary, len(results))
	breakdowns := make([]Breakdown, len(results))
	for i, res := range results {
		summaries[i] = res.Summary
		breakdowns[i] = res.Breakdown
	}

	metrics := map[string]metricResult{}
	for i, m := range computeMetrics(summaries) {
		metrics[metricSpecs[i].name] = m
	}
	stats := map[string]breakdownStat{}
	for _, s := range aggregateBreakdowns(breakdowns, cfg) {
		stats[s.block+"/"+s.key] = s
	}

	for _, tt := range []struct{ row, metric string }{
		{"r1_overall/all", "RoutesCancelledMidRoute"},
		{"r6_overall/all", "SectorIncidentsHittingAPlayer"},
		{"r7_infamy9/final", "AnyPlayerReachedInfamy9"},
	} {
		t.Run(tt.row, func(t *testing.T) {
			got, ok := stats[tt.row]
			if !ok {
				t.Fatalf("aggregateBreakdowns produced no %q row", tt.row)
			}
			want := metrics[tt.metric]
			if got.ok != want.ok || got.n != want.n || got.excluded != want.excluded {
				t.Errorf("%s = (ok %v, n %d, excluded %d), telemetry %s = (ok %v, n %d, excluded %d)",
					tt.row, got.ok, got.n, got.excluded, tt.metric, want.ok, want.n, want.excluded)
			}
			if got.mean != want.mean || got.halfWidth != want.halfWidth {
				t.Errorf("%s = %g ± %g, telemetry %s = %g ± %g",
					tt.row, got.mean, got.halfWidth, tt.metric, want.mean, want.halfWidth)
			}
		})
	}
}

// TestBreakdownInfamy9EverImpliesReached checks the one direction the
// ever-versus-final comparison depends on: a match whose final state has a
// seat at Legend must have visited Legend. The converse is exactly the gap
// the demonstration reports and is deliberately not asserted.
func TestBreakdownInfamy9EverImpliesReached(t *testing.T) {
	results, _ := breakdownMatches(t, 4, bots.Operator, 40)
	for i, res := range results {
		if res.Breakdown.AnyPlayerFinallyReachedInfamy9 && !res.Breakdown.AnyPlayerEverReachedInfamy9 {
			t.Errorf("match %d: a seat is at Infamy 9 at final scoring but no crossing was ever observed", i)
		}
	}
}

// TestBreakdownTier4AccountingBalances checks the contract bookkeeping
// against the event stream and against itself: every Tier IV delivery in
// the stream is a Tier IV contract this tracker saw accepted first, and
// nothing leaves a hand twice.
func TestBreakdownTier4AccountingBalances(t *testing.T) {
	results, _ := breakdownMatches(t, 4, bots.Operator, 60)

	sawAccept := false
	for i, res := range results {
		b := res.Breakdown
		if b.Tier4Accepted > 0 {
			sawAccept = true
		}
		if got := b.Tier4Delivered + b.Tier4Abandoned + b.Tier4Unresolved; got != b.Tier4Accepted {
			t.Errorf("match %d: %d delivered + %d abandoned + %d unresolved = %d, want %d accepted",
				i, b.Tier4Delivered, b.Tier4Abandoned, b.Tier4Unresolved, got, b.Tier4Accepted)
		}

		streamDeliveries := 0
		for _, e := range res.Events {
			if e.Kind == game.EventDelivered && e.Tier == tier4Index {
				streamDeliveries++
			}
		}
		if b.Tier4Delivered != streamDeliveries {
			t.Errorf("match %d: Tier4Delivered = %d, event stream holds %d Tier IV deliveries", i, b.Tier4Delivered, streamDeliveries)
		}
	}
	if !sawAccept {
		t.Fatal("no Tier IV contract was accepted across the whole sample — the accounting under test never ran")
	}
}

// TestCrossingClassificationIsExclusiveWhereItClaimsToBe pins the two
// exclusive readings the R7 verdict is taken from. The three "with_" counts
// are deliberately non-exclusive; ConfrontationWinOnly and NoDeliberateAct
// are not, and a crossing must land in at most one of them.
func TestCrossingClassificationIsExclusiveWhereItClaimsToBe(t *testing.T) {
	tests := []struct {
		name             string
		acts             seatActs
		wantWinOnly      int
		wantNoDeliberate int
	}{
		{"nothing", seatActs{}, 0, 1},
		{"win alone", seatActs{wonConfrontation: true}, 1, 0},
		{"delivery alone", seatActs{delivered: true}, 0, 0},
		{"stake alone", seatActs{staked: true}, 0, 0},
		{"win and delivery", seatActs{wonConfrontation: true, delivered: true}, 0, 0},
		{"win and stake", seatActs{wonConfrontation: true, staked: true}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c crossingCounts
			c.record(tt.acts)
			if c.Total != 1 {
				t.Errorf("Total = %d, want 1", c.Total)
			}
			if c.ConfrontationWinOnly != tt.wantWinOnly {
				t.Errorf("ConfrontationWinOnly = %d, want %d", c.ConfrontationWinOnly, tt.wantWinOnly)
			}
			if c.NoDeliberateAct != tt.wantNoDeliberate {
				t.Errorf("NoDeliberateAct = %d, want %d", c.NoDeliberateAct, tt.wantNoDeliberate)
			}
			if c.ConfrontationWinOnly+c.NoDeliberateAct > 1 {
				t.Error("a crossing landed in both exclusive buckets")
			}
		})
	}
}

// TestAggregateBreakdownsExcludesEmptyDenominators is D35's own rule read
// at the row level: a match with no population for a row is excluded from
// that row's vector and counted, never folded in as a zero. A card absent
// from a match's deck and a match that accepted no Tier IV contract are the
// two shapes that happen constantly in a real sweep.
func TestAggregateBreakdownsExcludesEmptyDenominators(t *testing.T) {
	catalog := rules.IncidentCatalog()
	cfg := game.DefaultConfig()
	cfg.Rounds = 2

	// Match A: card 0 live and hitting, one Tier IV accepted and abandoned.
	// Match B: card 0 absent entirely, no Tier IV contract at all.
	a := Breakdown{
		RoutesSubmittedByRound: []int{2, 0},
		RoutesHaltedByRound:    []int{1, 0},
		IncidentLive:           map[rules.IncidentCardID]bool{catalog[0].ID: true},
		IncidentHit:            map[rules.IncidentCardID]bool{catalog[0].ID: true},
		Tier4Accepted:          1,
		Tier4Abandoned:         1,
	}
	b := Breakdown{
		RoutesSubmittedByRound: []int{2, 2},
		RoutesHaltedByRound:    []int{0, 0},
		IncidentLive:           map[rules.IncidentCardID]bool{catalog[1].ID: true},
		IncidentHit:            map[rules.IncidentCardID]bool{},
	}

	stats := map[string]breakdownStat{}
	for _, s := range aggregateBreakdowns([]Breakdown{a, b}, cfg) {
		stats[s.block+"/"+s.key] = s
	}

	round2 := stats["r1_round/2"]
	if round2.n != 1 || round2.excluded != 1 {
		t.Errorf("r1_round/2 = (n %d, excluded %d), want (1, 1) — the match that submitted no route in round 2 is excluded", round2.n, round2.excluded)
	}
	if round2.denominator != 2 || round2.numerator != 0 {
		t.Errorf("r1_round/2 pooled = %d/%d, want 0/2", round2.numerator, round2.denominator)
	}

	card0 := stats["r6_card/"+catalog[0].Name]
	if card0.n != 1 || card0.excluded != 1 {
		t.Errorf("r6_card/%s = (n %d, excluded %d), want (1, 1) — a card absent from a match's deck is not a miss", catalog[0].Name, card0.n, card0.excluded)
	}

	rate := stats["r7_tier4/abandon_rate"]
	if rate.n != 1 || rate.excluded != 1 {
		t.Errorf("r7_tier4/abandon_rate = (n %d, excluded %d), want (1, 1) — a match that accepted nothing has no rate", rate.n, rate.excluded)
	}

	// Every match is in a count-shaped row's population by construction.
	accepted := stats["r7_tier4/accepted"]
	if accepted.n != 2 || accepted.excluded != 0 || accepted.numerator != 1 || accepted.denominator != 2 {
		t.Errorf("r7_tier4/accepted = %+v, want n 2, excluded 0, pooled 1/2", accepted)
	}
}

// TestAggregateBreakdownsRejectsAMismatchedRoundVector pins the loud half
// of the per-round indexing rule. Truncating to the shorter of the two
// vectors would write a per-round table quietly missing rounds, which is
// the same failure as a gate reporting green having inspected nothing —
// so a mismatch names itself instead.
func TestAggregateBreakdownsRejectsAMismatchedRoundVector(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Rounds = 3

	for _, tt := range []struct {
		name string
		b    Breakdown
	}{
		{"too long", Breakdown{RoutesSubmittedByRound: []int{1, 1, 1, 1}, RoutesHaltedByRound: []int{0, 0, 0, 0}}},
		{"too short", Breakdown{RoutesSubmittedByRound: []int{1, 1}, RoutesHaltedByRound: []int{0, 0}}},
		{"halted disagrees with submitted", Breakdown{RoutesSubmittedByRound: []int{1, 1, 1}, RoutesHaltedByRound: []int{0, 0}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("aggregateBreakdowns returned normally on a mismatched per-round vector")
				}
				if msg, _ := r.(string); !strings.Contains(msg, "cfg.Rounds") {
					t.Errorf("panic message does not name the problem: %v", r)
				}
			}()
			aggregateBreakdowns([]Breakdown{tt.b}, cfg)
		})
	}
}

// breakdownArgs is realArgs plus --breakdown, at one sweep point so the
// breakdown file's row count is predictable.
func breakdownArgs(out, breakdown string) []string {
	return []string{
		"--matches", "6",
		"--players", "2",
		"--bots", "drifter",
		"--sweep", "LeaseCostPerBlock=2,3",
		"--out", out,
		"--breakdown", breakdown,
	}
}

func TestRunBreakdownHappyPath(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sweep.csv")
	bd := filepath.Join(dir, "breakdown.csv")
	var stderr bytes.Buffer

	if code := runWithDeps(breakdownArgs(out, bd), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	rows := readDataRows(t, bd)
	if len(rows) == 0 {
		t.Fatal("breakdown file has no data rows")
	}

	blocks := map[string]int{}
	configs := map[string]bool{}
	for _, row := range rows {
		if row["status"] != "ok" {
			t.Errorf("row status = %q, want ok (error=%q)", row["status"], row["error"])
		}
		if row["config_sha256"] == "" {
			t.Error("config_sha256 is empty — the row cannot be traced to a Config")
		}
		blocks[row["block"]]++
		configs[row["sweep.LeaseCostPerBlock"]] = true
	}
	if len(configs) != 2 {
		t.Errorf("breakdown covered %d configurations, want 2", len(configs))
	}
	for _, want := range []string{"r1_overall", "r1_round", "r6_overall", "r6_card", "r7_infamy9", "r7_tier4", "r7_crossing"} {
		if blocks[want] == 0 {
			t.Errorf("breakdown has no %q rows", want)
		}
	}
	if got, want := blocks["r1_round"], 2*game.DefaultConfig().Rounds; got != want {
		t.Errorf("r1_round rows = %d, want %d (one per round per configuration)", got, want)
	}
	if got, want := blocks["r6_card"], 2*len(rules.IncidentCatalog()); got != want {
		t.Errorf("r6_card rows = %d, want %d (one per catalog card per configuration)", got, want)
	}
}

// TestRunBreakdownIsDeterministic is issue #200's byte-identical criterion
// applied to the second file: the demonstration's supporting detail has to
// be as reproducible as the verdict it supports.
func TestRunBreakdownIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	read := func(name string) []byte {
		out := filepath.Join(dir, name+"-sweep.csv")
		bd := filepath.Join(dir, name+"-breakdown.csv")
		stderr.Reset()
		if code := runWithDeps(breakdownArgs(out, bd), &stderr, RunMany, fakeGitSHA); code != 0 {
			t.Fatalf("run() = %d; stderr:\n%s", code, stderr.String())
		}
		b, err := os.ReadFile(bd)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	if !bytes.Equal(read("a"), read("b")) {
		t.Error("two fresh runs of the same invocation produced different breakdown bytes")
	}
}

// TestRunBreakdownRefusesExistingFile is the fails-closed rule this file
// needs in place of a resume path: overwriting or appending to an existing
// breakdown would produce a file holding two sweeps' rows with nothing to
// tell them apart.
func TestRunBreakdownRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sweep.csv")
	bd := filepath.Join(dir, "breakdown.csv")
	if err := os.WriteFile(bd, []byte("pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := runWithDeps(breakdownArgs(out, bd), &stderr, panicRunner, fakeGitSHA); code == 0 {
		t.Error("run() = 0, want non-zero when --breakdown names an existing file")
	}
	got, err := os.ReadFile(bd)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pre-existing\n" {
		t.Errorf("the existing file was modified: %q", got)
	}
}

// TestRunBreakdownRefusesResume is the other half of that rule: a resumed
// sweep skips configurations already recorded in --out, and a breakdown
// written beside it would silently omit their rows while looking complete.
func TestRunBreakdownRefusesResume(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sweep.csv")
	var stderr bytes.Buffer

	if code := runWithDeps(realArgs(out), &stderr, RunMany, fakeGitSHA); code != 0 {
		t.Fatalf("first run = %d; stderr:\n%s", code, stderr.String())
	}

	stderr.Reset()
	bd := filepath.Join(dir, "breakdown.csv")
	code := runWithDeps(breakdownArgs(out, bd), &stderr, panicRunner, fakeGitSHA)
	if code == 0 {
		t.Error("run() = 0, want non-zero for --breakdown against a resumed sweep")
	}
	if !strings.Contains(stderr.String(), "--breakdown cannot be combined with resuming") {
		t.Errorf("stderr does not name the problem:\n%s", stderr.String())
	}
	if _, err := os.Stat(bd); !os.IsNotExist(err) {
		t.Errorf("a breakdown file was created anyway: Stat = %v", err)
	}
}

// TestRunBreakdownWritesErrorRow is issue #200's fails-closed criterion
// carried into the second file: a configuration whose matches did not all
// complete is written as a failure, never omitted.
func TestRunBreakdownWritesErrorRow(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sweep.csv")
	bd := filepath.Join(dir, "breakdown.csv")
	var stderr bytes.Buffer

	fake := func(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error) {
		if cfg.LeaseCostPerBlock == 2 {
			return nil, errors.New("synthetic failure for LeaseCostPerBlock=2")
		}
		return RunMany(seeds, cfg, players, bot, workers)
	}

	if code := runWithDeps(breakdownArgs(out, bd), &stderr, fake, fakeGitSHA); code != 1 {
		t.Fatalf("run() = %d, want 1; stderr:\n%s", code, stderr.String())
	}

	errRows := 0
	for _, row := range readDataRows(t, bd) {
		if row["sweep.LeaseCostPerBlock"] != "2" {
			continue
		}
		errRows++
		if row["status"] != "error" || row["error"] == "" {
			t.Errorf("row = %+v, want status=error with a message", row)
		}
		if row["block"] != "" {
			t.Errorf("errored configuration wrote a %q block row; it measured nothing", row["block"])
		}
	}
	if errRows != 1 {
		t.Errorf("errored configuration wrote %d rows, want exactly 1", errRows)
	}
}
