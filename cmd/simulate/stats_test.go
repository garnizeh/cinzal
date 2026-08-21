package main

import (
	"math"
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/telemetry"
)

func TestIntervalTooFewValues(t *testing.T) {
	for _, values := range [][]float64{nil, {}, {5}} {
		_, _, n, ok := interval(values)
		if ok {
			t.Errorf("interval(%v) ok = true, want false (n=%d < 2 is not a measurement)", values, len(values))
		}
		if n != len(values) {
			t.Errorf("interval(%v) n = %d, want %d", values, n, len(values))
		}
	}
}

// TestIntervalConstantVector is issue #249: a vector of two or more
// identical values has zero variance, so its interval is degenerate — "not
// a result" per the roadmap, same reasoning as n < 2. ok must be false, but
// mean still comes back as the constant value itself: it is a fact worth
// keeping, just not a measurement (see interval's own doc comment).
func TestIntervalConstantVector(t *testing.T) {
	values := []float64{4, 4, 4, 4}
	mean, halfWidth, n, ok := interval(values)
	if ok {
		t.Fatalf("interval(%v) ok = true, want false (zero-width interval is not a measurement)", values)
	}
	if mean != 4 {
		t.Errorf("mean = %v, want 4 (the constant value, kept even though ok = false)", mean)
	}
	if halfWidth != 0 {
		t.Errorf("halfWidth = %v, want 0 (zero variance)", halfWidth)
	}
	if n != 4 {
		t.Errorf("n = %d, want 4", n)
	}
}

// TestIntervalKnownSample checks the formula end to end against a value
// computed by hand: {1,2,3,4,5}, mean=3, sample stddev = sqrt(2.5) ≈
// 1.5811, halfWidth = 1.96 · 1.5811 / sqrt(5) ≈ 1.3859.
func TestIntervalKnownSample(t *testing.T) {
	mean, halfWidth, n, ok := interval([]float64{1, 2, 3, 4, 5})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if n != 5 {
		t.Errorf("n = %d, want 5", n)
	}
	if math.Abs(mean-3) > 1e-9 {
		t.Errorf("mean = %v, want 3", mean)
	}
	wantHalfWidth := 1.96 * math.Sqrt(2.5) / math.Sqrt(5)
	if math.Abs(halfWidth-wantHalfWidth) > 1e-9 {
		t.Errorf("halfWidth = %v, want %v", halfWidth, wantHalfWidth)
	}
}

func TestRateMetricExcludesZeroN(t *testing.T) {
	extract := rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.RoutesCancelledMidRoute })

	v, included := extract(telemetry.MatchSummary{RoutesCancelledMidRoute: telemetry.Rate{Value: 0.5, N: 10}})
	if !included || v != 0.5 {
		t.Errorf("N=10: (v, included) = (%v, %v), want (0.5, true)", v, included)
	}

	v, included = extract(telemetry.MatchSummary{RoutesCancelledMidRoute: telemetry.Rate{Value: 0, N: 0}})
	if included {
		t.Errorf("N=0: included = true, want false (empty denominator excluded per D35)")
	}
	_ = v
}

func TestBoolMetric(t *testing.T) {
	extract := boolMetric(func(m telemetry.MatchSummary) bool { return m.AnyPlayerReachedInfamy9 })
	if v, included := extract(telemetry.MatchSummary{AnyPlayerReachedInfamy9: true}); v != 1 || !included {
		t.Errorf("true: (v, included) = (%v, %v), want (1, true)", v, included)
	}
	if v, included := extract(telemetry.MatchSummary{AnyPlayerReachedInfamy9: false}); v != 0 || !included {
		t.Errorf("false: (v, included) = (%v, %v), want (0, true)", v, included)
	}
}

// TestComputeMetricsExclusionCount is D35's own bookkeeping requirement:
// a match with an empty denominator is excluded from that row's vector,
// and the exclusion count is reported beside it.
func TestComputeMetricsExclusionCount(t *testing.T) {
	summaries := []telemetry.MatchSummary{
		{RoutesCancelledMidRoute: telemetry.Rate{Value: 0.1, N: 5}},
		{RoutesCancelledMidRoute: telemetry.Rate{Value: 0.2, N: 5}},
		{RoutesCancelledMidRoute: telemetry.Rate{Value: 0, N: 0}}, // excluded
	}

	results := computeMetrics(summaries)
	var got *metricResult
	for i, spec := range metricSpecs {
		if spec.name == "RoutesCancelledMidRoute" {
			got = &results[i]
		}
	}
	if got == nil {
		t.Fatal("RoutesCancelledMidRoute not found in metricSpecs")
	}
	if got.n != 2 {
		t.Errorf("n = %d, want 2", got.n)
	}
	if got.excluded != 1 {
		t.Errorf("excluded = %d, want 1", got.excluded)
	}
	if !got.ok {
		t.Error("ok = false, want true (n=2 is enough for a measurement)")
	}
}

// TestComputeMetricsAllExcluded is the n<2-after-exclusions case the RFC
// §16.4 addendum states explicitly: not a measurement, not an error.
func TestComputeMetricsAllExcluded(t *testing.T) {
	summaries := []telemetry.MatchSummary{
		{RoutesCancelledMidRoute: telemetry.Rate{Value: 0, N: 0}},
		{RoutesCancelledMidRoute: telemetry.Rate{Value: 0, N: 0}},
	}

	results := computeMetrics(summaries)
	for i, spec := range metricSpecs {
		if spec.name != "RoutesCancelledMidRoute" {
			continue
		}
		got := results[i]
		if got.ok {
			t.Error("ok = true, want false — every match excluded, n effective is 0")
		}
		if got.excluded != 2 {
			t.Errorf("excluded = %d, want 2", got.excluded)
		}
	}
}

// TestMetricSpecsCoverage guards the CSV's own promise ("every §22 field")
// against a future MatchSummary field silently landing without a
// metricSpecs entry: the number of specs must track the number of
// telemetry-computed fields, less the one deliberate omission (Solo).
func TestMetricSpecsCoverage(t *testing.T) {
	// Derived from telemetry.MatchSummary itself, not a literal count: a
	// literal only catches an edit to metricSpecs, never a new
	// MatchSummary field landing without one (the actual failure mode
	// this test exists to catch).
	summaryType := reflect.TypeFor[telemetry.MatchSummary]()
	wantCount := summaryType.NumField() - 1 // one deliberate omission: Solo

	if len(metricSpecs) != wantCount {
		t.Errorf("len(metricSpecs) = %d, want %d — did telemetry.MatchSummary gain or lose a field?", len(metricSpecs), wantCount)
	}
	seen := map[string]bool{}
	for _, spec := range metricSpecs {
		if seen[spec.name] {
			t.Errorf("metricSpecs has a duplicate name %q", spec.name)
		}
		seen[spec.name] = true
		if _, ok := summaryType.FieldByName(spec.name); !ok {
			t.Errorf("metricSpecs name %q is not a telemetry.MatchSummary field", spec.name)
		}
	}
	if seen["Solo"] {
		t.Error("metricSpecs unexpectedly includes Solo")
	}
}
