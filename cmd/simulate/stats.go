package main

import (
	"math"

	"github.com/garnizeh/cinzal/internal/telemetry"
)

// interval computes D35's mean ± 1.96 · s/√n over values — one routine for
// every §22 row, count or ratio or 0/1 indicator alike, so no per-row
// branch on the metric's own shape ever has to exist (D35 §2: "one formula
// for every row... so cmd/simulate implements one interval routine, not a
// per-shape branch"). ok is false when len(values) < 2: RFC §16.4's own
// addendum states s is undefined at n = 1 and the vector is empty at
// n = 0, and both are "no measurement, no verdict," not a bare or
// degenerate mean.
func interval(values []float64) (mean, halfWidth float64, n int, ok bool) {
	n = len(values)
	if n < 2 {
		return 0, 0, n, false
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(n)

	sumSq := 0.0
	for _, v := range values {
		d := v - mean
		sumSq += d * d
	}
	s := math.Sqrt(sumSq / float64(n-1))

	halfWidth = 1.96 * s / math.Sqrt(float64(n))
	return mean, halfWidth, n, true
}

// metricSpec is one telemetry.MatchSummary field's own D35 reduction: how
// to pull this match's own number out, and whether it counts at all —
// false for a Rate whose N is 0, D35's "excluded from that row's vector,
// the exclusion count reported beside it." name is the Go field name
// exactly as summary.go spells it, so a CSV column traces straight back to
// that field's own GDD-§22-row doc comment.
type metricSpec struct {
	name    string
	extract func(telemetry.MatchSummary) (value float64, included bool)
}

func rateMetric(get func(telemetry.MatchSummary) telemetry.Rate) func(telemetry.MatchSummary) (float64, bool) {
	return func(m telemetry.MatchSummary) (float64, bool) {
		r := get(m)
		return r.Value, r.N > 0
	}
}

func floatMetric(get func(telemetry.MatchSummary) float64) func(telemetry.MatchSummary) (float64, bool) {
	return func(m telemetry.MatchSummary) (float64, bool) {
		return get(m), true
	}
}

func boolMetric(get func(telemetry.MatchSummary) bool) func(telemetry.MatchSummary) (float64, bool) {
	return func(m telemetry.MatchSummary) (float64, bool) {
		if get(m) {
			return 1, true
		}
		return 0, true
	}
}

func intMetric(get func(telemetry.MatchSummary) int) func(telemetry.MatchSummary) (float64, bool) {
	return func(m telemetry.MatchSummary) (float64, bool) {
		return float64(get(m)), true
	}
}

// metricSpecs lists every telemetry.MatchSummary field this CSV reports,
// in summary.go's own declaration order (its GDD §22 row order). Solo is
// deliberately absent: it's a caller-set tag, not a computed metric, and
// cmd/simulate's own matches are always bot-vs-bot — every row would carry
// the same constant, telling a reader nothing.
var metricSpecs = []metricSpec{
	{"RoutesCancelledMidRoute", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.RoutesCancelledMidRoute })},
	{"DeliveriesPerPlayer", floatMetric(func(m telemetry.MatchSummary) float64 { return m.DeliveriesPerPlayer })},
	{"WinnerRPLeadOverLastPlace", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.WinnerRPLeadOverLastPlace })},
	{"AnyPlayerReachedInfamy9", boolMetric(func(m telemetry.MatchSummary) bool { return m.AnyPlayerReachedInfamy9 })},
	{"AnyPlayerStayedAtInfamy2OrBelow", boolMetric(func(m telemetry.MatchSummary) bool { return m.AnyPlayerStayedAtInfamy2OrBelow })},
	{"SectorIncidentsHittingAPlayer", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.SectorIncidentsHittingAPlayer })},
	{"LiveLeasesAtFinalScoring", floatMetric(func(m telemetry.MatchSummary) float64 { return m.LiveLeasesAtFinalScoring })},
	{"ShareOfMapUnderSightFinalThird", floatMetric(func(m telemetry.MatchSummary) float64 { return m.ShareOfMapUnderSightFinalThird })},
	{"ConfrontationsWonAgainstEvasiveLoser", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.ConfrontationsWonAgainstEvasiveLoser })},
	{"ConfrontationsPerMatch", intMetric(func(m telemetry.MatchSummary) int { return m.ConfrontationsPerMatch })},
	{"PlayersEndingInFlaggedSector", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.PlayersEndingInFlaggedSector })},
	{"ConvergenceCardConfrontations", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.ConvergenceCardConfrontations })},
	{"RoundsFlaggedLoitering", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.RoundsFlaggedLoitering })},
	{"HeatMapLowConfidenceEntries", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.HeatMapLowConfidenceEntries })},
	{"ConfrontationsInFinal3Rounds", rateMetric(func(m telemetry.MatchSummary) telemetry.Rate { return m.ConfrontationsInFinal3Rounds })},
}

// metricResult is one metricSpec's D35 reduction across a configuration's
// whole match vector: the interval, when there were enough included
// matches to compute one, plus the bookkeeping the RFC §16.4 addendum and
// the roadmap both require beside it — effective n and excluded count — so
// a configuration can never read as having measured more than it did.
type metricResult struct {
	mean, halfWidth float64
	n               int
	excluded        int
	ok              bool
}

// computeMetrics reduces summaries — one configuration's whole match
// vector — to one metricResult per metricSpecs entry, in that order.
func computeMetrics(summaries []telemetry.MatchSummary) []metricResult {
	results := make([]metricResult, len(metricSpecs))
	for i, spec := range metricSpecs {
		values := make([]float64, 0, len(summaries))
		excluded := 0
		for _, s := range summaries {
			v, included := spec.extract(s)
			if !included {
				excluded++
				continue
			}
			values = append(values, v)
		}
		mean, halfWidth, n, ok := interval(values)
		results[i] = metricResult{mean: mean, halfWidth: halfWidth, n: n, excluded: excluded, ok: ok}
	}
	return results
}
