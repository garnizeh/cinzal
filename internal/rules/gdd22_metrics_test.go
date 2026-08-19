package rules

import (
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// gdd22Row is one row of D33's audit (docs/decisions/D33-telemetry-event-
// stream-coverage.md) — the classification of one GDD §22 per-match metric
// against the event stream, transcribed here as executable documentation.
// At least one of Events or Other is set: Events names the EventKind(s) a
// telemetry consumer reads to compute the metric; Other records a caveat
// or explains why no event answers it (a final-state read, order-log
// access, a deferred milestone, or no operational definition at all).
type gdd22Row struct {
	row    int
	metric string
	events []game.EventKind
	other  string
}

// gdd22Rows is D33's twenty-row audit table (docs/decisions/D33-telemetry-
// event-stream-coverage.md), one entry per GDD §22 per-match metric. #196's
// own acceptance criterion: a metric with neither Events nor Other set is
// unmapped and fails TestGDD22MetricsHaveAnEventOrOtherSource below — this
// table is the single place that closes the world, not a sample of it.
var gdd22Rows = []gdd22Row{
	{row: 1, metric: "Routes cancelled mid-route (R1)", events: []game.EventKind{game.EventRouteHalted},
		other: "numerator only — the denominator (\"of submitted routes\") is order-log-shaped, not an event (D33 row 1)"},
	{row: 2, metric: "Deliveries per player", events: []game.EventKind{game.EventDelivered}},
	{row: 3, metric: "Winner's RP lead over last place", other: "final-state read — Post majority/lease RP is computed once at scoring (D33 row 3)"},
	{row: 4, metric: "Matches where a player reached Infamy 9", other: "final-state read — EventTierLegend fires for the whole 9-10 band, not 9 specifically (D33 row 4)"},
	{row: 5, metric: "Matches where a player stayed at Infamy <= 2 to the end", other: "final-state read — no event exists below the Feared tier at all (D33 row 5)"},
	{row: 6, metric: "Sector incidents actually hitting a player", events: []game.EventKind{game.EventIncidentHit}},
	{row: 7, metric: "Live leases at final scoring, per player", other: "final-state read — EventLeaseExpired only fires on expiry, never on what's still live (D33 row 7)"},
	{row: 8, metric: "Share of map under sight in the final third", other: "final-state read, fog-private — SeatArchive.Sight is never aggregated across seats (D33 row 8)"},
	{row: 9, metric: "Confrontations won against an Evasive loser", events: []game.EventKind{game.EventConfrontation},
		other: "read via EventConfrontation's own Stance + Decisive fields (D33 row 9), not a separate kind"},
	{row: 10, metric: "Confrontations per match, 4-5 players", events: []game.EventKind{game.EventConfrontation},
		other: "group by (Round, Node), not raw count — a tie or multi-loser decisive emits more than one event per confrontation (D33 row 10)"},
	{row: 11, metric: "Confrontations per match, 2 players", events: []game.EventKind{game.EventConfrontation}},
	{row: 12, metric: "Players ending a route in a flagged unstable sector", events: []game.EventKind{game.EventIncidentExposed}},
	{row: 13, metric: "Share of RP swing traceable to [O] cards", other: "open pending a design call — no precise event or state source in M2's first pass (D33 row 13)"},
	{row: 14, metric: "Convergence [C] cards that produced a confrontation", events: []game.EventKind{game.EventConfrontation},
		other: "loose reading only: a confrontation occurred in a round a [C] card was live, via Event.Round and the seed-derivable card identity — not causation (D33 row 14)"},
	{row: 15, metric: "Attribution queries that ruled out at least one player", other: "deferred to M5.5 — a human-facing UI interaction, not a headless fact (D33 row 15)"},
	{row: 16, metric: "Heat Map opened per player per match", other: "deferred to M5 — UI instrumentation on a feature not yet built (D33 row 16)"},
	{row: 17, metric: "Rounds flagged as Loitering", events: []game.EventKind{game.EventLoitering},
		other: "denominator (player-rounds) is plain arithmetic over setup facts, not an event (D33 row 17)"},
	{row: 18, metric: "Loitering flags triggered by legitimate short-haul play", other: "no operational definition exists to compute against (D33 row 18)"},
	{row: 19, metric: "Heat Map entries at low confidence (< 3 observations)", other: "final-state read, fog-private — NodeStats.ObservedRounds is sourced from the same owner-only SeatArchive as row 8 (D33 row 19)"},
	{row: 20, metric: "Confrontations occurring in the final 3 rounds", events: []game.EventKind{game.EventConfrontation},
		other: "Event.Round >= cfg.Rounds-2, same (Round, Node) grouping caveat as row 10 (D33 row 20)"},
}

// TestGDD22MetricsHaveAnEventOrOtherSource is #196's own fails-closed
// acceptance criterion: every one of GDD §22's twenty per-match metrics
// must have a stated source — either the EventKind(s) that answer it, or an
// explicit reason none does. A row with neither is not a metric #196 forgot
// to wire up silently; it is a bug in this table, and the test must fail
// rather than pass by omission.
func TestGDD22MetricsHaveAnEventOrOtherSource(t *testing.T) {
	if len(gdd22Rows) != 20 {
		t.Fatalf("gdd22Rows has %d entries, want 20 (docs/decisions/D33-telemetry-event-stream-coverage.md's own recount of GDD §22's table)", len(gdd22Rows))
	}

	seen := make(map[int]bool, len(gdd22Rows))
	for _, r := range gdd22Rows {
		if seen[r.row] {
			t.Errorf("row %d (%s) appears more than once", r.row, r.metric)
		}
		seen[r.row] = true

		if len(r.events) == 0 && r.other == "" {
			t.Errorf("row %d (%s): unmapped — neither an EventKind source nor an explanation for why one doesn't apply", r.row, r.metric)
		}
		for _, k := range r.events {
			if strings.HasPrefix(k.String(), "EventKind(") {
				t.Errorf("row %d (%s): %v is not a declared EventKind (String() falls through to its invalid-value format)", r.row, r.metric, k)
			}
		}
	}
	for i := 1; i <= 20; i++ {
		if !seen[i] {
			t.Errorf("row %d missing from gdd22Rows entirely", i)
		}
	}
}
