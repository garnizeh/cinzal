package telemetry

import (
	"reflect"
	"testing"
)

// roundActionRow is one bullet of GDD §22's "Per round" or "Per action"
// list (docs/project/cinzal-gdd.md:1608-1619) — D33 never audited these
// bullets (it scoped itself to the twenty numbered per-match rows), so
// this table is this package's own accounting, the same discipline
// internal/rules/gdd22_metrics_test.go already applies to the per-match
// set: at least one of field or outOfScope is set, and #198's own
// acceptance criterion ("every row is a field... or explicitly declared
// out of scope with the milestone that owns it named") is checked against
// RoundActionSummary's own fields by reflection below, not a hand-
// maintained list that a field rename could silently leave stale.
type roundActionRow struct {
	section    string // "per round" or "per action", matching the GDD heading
	metric     string
	field      string // RoundActionSummary field name, if this package answers it
	outOfScope string // milestone + reason, if it does not
}

var roundActionRows = []roundActionRow{
	{section: "per round", metric: "Median/90th-percentile time to submit an order, by round",
		outOfScope: "M4 — human-timing metric, no meaning against bots (issue #198)"},
	{section: "per round", metric: "Number of players who let the timer expire",
		outOfScope: "M4 — human-timing metric, no meaning against bots (issue #198)"},
	{section: "per round", metric: "Distribution of stance choices", field: "StanceDistribution"},
	{section: "per round", metric: "Ledger purchase rate", field: "LedgerPurchaseRate"},
	{section: "per round", metric: "Ledger purchases by round number", field: "LedgerPurchasesByRound"},
	{section: "per action", metric: "Action selection frequency", field: "ActionFrequency"},
	{section: "per action", metric: "Item purchase frequency by item", field: "ItemPurchaseFrequency"},
	{section: "per action", metric: "Global event and incident cards, by how much they swing final standings",
		outOfScope: "open, same as MatchSummary's row 13 — GDD §22 gives no operational definition of \"swing\", and answering it precisely needs the per-round MatchState diffing D33 declined to build for row 13 (RoundActionSummary's own doc comment)"},
	{section: "per action", metric: "Post-match recall",
		outOfScope: "M5.5 — a human-facing playtest question, not a headless fact (same category as MatchSummary's rows 15/18)"},
}

// TestGDD22RoundAndActionRowsHaveAFieldOrAreOutOfScope is #198's own
// completeness criterion, made executable: every bullet in GDD §22's "Per
// round" and "Per action" lists is accounted for exactly once, either by
// naming the RoundActionSummary field that answers it or by stating which
// milestone owns it instead — and every one of RoundActionSummary's own
// fields is claimed by some row, derived via reflection so a field rename
// fails this test rather than leaving a stale string literal behind.
func TestGDD22RoundAndActionRowsHaveAFieldOrAreOutOfScope(t *testing.T) {
	const wantRows = 9 // 5 "per round" bullets + 4 "per action" bullets
	if len(roundActionRows) != wantRows {
		t.Fatalf("roundActionRows has %d entries, want %d (docs/project/cinzal-gdd.md:1608-1619)", len(roundActionRows), wantRows)
	}

	seen := make(map[string]bool, len(roundActionRows))
	fields := make(map[string]bool, len(roundActionRows))
	sectionCounts := make(map[string]int, 2)
	for _, r := range roundActionRows {
		if seen[r.metric] {
			t.Errorf("metric %q appears more than once", r.metric)
		}
		seen[r.metric] = true
		sectionCounts[r.section]++

		if (r.field == "") == (r.outOfScope == "") {
			t.Errorf("metric %q: exactly one of field or outOfScope must be set, got field=%q outOfScope=%q", r.metric, r.field, r.outOfScope)
			continue
		}
		if r.field != "" {
			if fields[r.field] {
				t.Errorf("field %q claimed by more than one metric", r.field)
			}
			fields[r.field] = true
		}
	}

	// GDD §22's own heading split: 5 "Per round" bullets, 4 "Per action"
	// bullets (docs/project/cinzal-gdd.md:1609-1619) — a row filed under
	// the wrong heading would still pass every other check above.
	if got := sectionCounts["per round"]; got != 5 {
		t.Errorf("roundActionRows has %d \"per round\" rows, want 5", got)
	}
	if got := sectionCounts["per action"]; got != 4 {
		t.Errorf("roundActionRows has %d \"per action\" rows, want 4", got)
	}

	rt := reflect.TypeFor[RoundActionSummary]()
	if len(fields) != rt.NumField() {
		t.Errorf("roundActionRows names %d distinct RoundActionSummary fields, want %d (RoundActionSummary's own field count)", len(fields), rt.NumField())
	}
	for field := range rt.Fields() {
		if !fields[field.Name] {
			t.Errorf("RoundActionSummary field %q is not claimed by any row in roundActionRows", field.Name)
		}
	}
}
