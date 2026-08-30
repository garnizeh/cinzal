package opsmetrics

import (
	"fmt"
	"html/template"
	"io"
	"time"
)

// dashboardTemplate is D45's answer to "what is a dashboard" in a topology
// with no HTTP server yet (§18 — one binary, one Postgres, no third piece of
// infrastructure): a static page, built on stdlib html/template rather than
// templ (make generate's templ step is a no-op until M5), that
// cmd/simulate/cmd/replay each render to a file after a sweep. The same
// FoldSnapshot value serves this near-term artefact and the eventual M5 live
// page — nothing built here is thrown away once internal/web exists.
//
// Duration renders with a pass/fail mark against FoldDurationP99Threshold —
// exact regardless of process topology (D45). Allocation share renders as a
// labeled reference measurement with NO pass/fail mark against
// FoldAllocShareThreshold and a caption stating the production ratio is
// unmeasured until M5 — D51's amendment to D45's original single-number,
// pass/fail rendering, because the denominator is process-wide and neither
// M3 process (cmd/replay, cmd/simulate) is the production server §7.3's 20%
// figure was chosen against.
var dashboardTemplate = template.Must(template.New("fold-snapshot").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Cinzal fold metrics{{if .Label}} — {{.Label}}{{end}}</title></head>
<body>
<h1>Fold metrics{{if .Label}} — {{.Label}}{{end}}</h1>
<p>RFC-001 &sect;7.3's falsifiability trigger for the no-snapshot decision. Samples: {{.Count}}.</p>
<table border="1" cellpadding="4" cellspacing="0">
<tr><th>metric</th><th>value</th><th>threshold</th><th>verdict</th></tr>
<tr>
  <td>fold duration p50</td>
  <td>{{.P50Text}}</td>
  <td></td>
  <td></td>
</tr>
<tr>
  <td>fold duration p99</td>
  <td>{{.P99Text}}</td>
  <td>{{.DurationThresholdText}} (&sect;7.3)</td>
  <td>{{if .HasSamples}}{{if .DurationPass}}PASS{{else}}FAIL{{end}}{{else}}no samples{{end}}</td>
</tr>
<tr>
  <td>fold allocation share ({{.Label}})</td>
  <td>{{.AllocShareText}}</td>
  <td>{{.AllocShareThresholdText}} (&sect;7.3, deferred to M5)</td>
  <td>reference only — not compared</td>
</tr>
</table>
<p><em>Per D51: this process's allocation share is a labeled reference measurement,
not a validated bound on a production server's ratio. The real comparison against
the 20% line happens at M5, once cmd/server mixes fold work with HTTP handling,
templ rendering, pgx queries, and SSE — the workload &sect;7.3's figure was chosen
against. Neither M3 process shares that mix.</em></p>
</body></html>
`))

// dashboardView adapts FoldSnapshot into the plain, pre-formatted strings
// html/template needs — computing pass/fail and formatting durations in Go
// rather than in the template keeps the template itself free of anything
// that could silently diverge from Snapshot's own zero-sample handling.
type dashboardView struct {
	Label                   string
	Count                   int
	HasSamples              bool
	P50Text                 string
	P99Text                 string
	DurationThresholdText   string
	DurationPass            bool
	AllocShareText          string
	AllocShareThresholdText string
}

// WriteHTML renders s as the static dashboard artefact described above.
// Returns any error from the underlying writer or template execution; never
// panics on a zero-value or empty-sample FoldSnapshot — that state renders
// as "no samples" rather than a false PASS, which is exactly the fails-closed
// property a p99-computed-over-zero-samples-is-0 shortcut would violate.
func (s FoldSnapshot) WriteHTML(w io.Writer) error {
	view := dashboardView{
		Label:                   s.Label,
		Count:                   s.Count,
		HasSamples:              s.Count > 0,
		P50Text:                 formatDuration(s.P50),
		P99Text:                 formatDuration(s.P99),
		DurationThresholdText:   formatDuration(FoldDurationP99Threshold),
		DurationPass:            s.Count > 0 && s.P99 <= FoldDurationP99Threshold,
		AllocShareThresholdText: formatShare(FoldAllocShareThreshold),
	}
	if s.HasAllocShare {
		view.AllocShareText = formatShare(s.AllocShare)
	} else {
		view.AllocShareText = "no heap-churn samples"
	}

	if err := dashboardTemplate.Execute(w, view); err != nil {
		return fmt.Errorf("opsmetrics: WriteHTML: %w", err)
	}
	return nil
}

func formatDuration(d time.Duration) string {
	return d.String()
}

func formatShare(f float64) string {
	return fmt.Sprintf("%.2f%%", f*100)
}
