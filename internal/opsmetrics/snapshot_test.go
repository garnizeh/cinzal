package opsmetrics

import (
	"strings"
	"testing"
	"time"
)

// TestWriteHTMLZeroSamplesReportsNoSamples is the rendering half of #320's
// fails-closed acceptance criterion: an empty snapshot must render as "no
// samples," never as a silent PASS against FoldDurationP99Threshold.
func TestWriteHTMLZeroSamplesReportsNoSamples(t *testing.T) {
	var buf strings.Builder
	if err := (FoldSnapshot{}).WriteHTML(&buf); err != nil {
		t.Fatalf("WriteHTML() = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "PASS") {
		t.Error("output contains PASS for a snapshot with zero samples")
	}
	if !strings.Contains(out, "no samples") {
		t.Error("output does not report \"no samples\" for an empty snapshot")
	}
}

// TestWriteHTMLDurationPassFail checks both verdict directions render
// correctly and that the §7.3 threshold value itself is printed.
func TestWriteHTMLDurationPassFail(t *testing.T) {
	tests := []struct {
		name string
		p99  time.Duration
		want string
	}{
		{"under threshold", 10 * time.Millisecond, "PASS"},
		{"over threshold", 75 * time.Millisecond, "FAIL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := FoldSnapshot{Count: 100, P50: tc.p99 / 2, P99: tc.p99}
			var buf strings.Builder
			if err := snap.WriteHTML(&buf); err != nil {
				t.Fatalf("WriteHTML() = %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("output does not contain %q for p99=%v\n%s", tc.want, tc.p99, out)
			}
			if !strings.Contains(out, FoldDurationP99Threshold.String()) {
				t.Error("output does not print the §7.3 duration threshold value")
			}
		})
	}
}

// TestWriteHTMLAllocShareNeverPassFail is D51's specific amendment to D45:
// the allocation-share line must never carry a PASS/FAIL verdict, regardless
// of how close or far the recorded share is from FoldAllocShareThreshold —
// M3 has no process whose share is comparable to the 20% figure (D51).
func TestWriteHTMLAllocShareNeverPassFail(t *testing.T) {
	tests := []struct {
		name  string
		share float64
	}{
		{"near-1.0 fold-only reference", 0.97},
		{"small bot-diluted reference", 0.03},
		{"exactly at threshold", FoldAllocShareThreshold},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := FoldSnapshot{
				Count:         10,
				P50:           time.Millisecond,
				P99:           2 * time.Millisecond,
				AllocShare:    tc.share,
				HasAllocShare: true,
				Label:         "test reference",
			}
			var buf strings.Builder
			if err := snap.WriteHTML(&buf); err != nil {
				t.Fatalf("WriteHTML() = %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, "reference only") {
				t.Error("allocation-share line does not carry the reference-only caption")
			}
			if !strings.Contains(out, "test reference") {
				t.Error("output does not carry the snapshot's own Label")
			}
			if !strings.Contains(out, "M5") {
				t.Error("output does not caption that the production comparison is deferred to M5")
			}
		})
	}
}

// TestWriteHTMLNoAllocShareSamples checks the case where the duration
// reservoir has samples but the heap-churn sampler was never started —
// HasAllocShare stays false, and the page must say so rather than printing
// a bare 0.00%, which would read as "fold allocates nothing."
func TestWriteHTMLNoAllocShareSamples(t *testing.T) {
	snap := FoldSnapshot{Count: 5, P50: time.Millisecond, P99: 2 * time.Millisecond}
	var buf strings.Builder
	if err := snap.WriteHTML(&buf); err != nil {
		t.Fatalf("WriteHTML() = %v", err)
	}
	out := buf.String()
	// "0.00%" as a table cell value, not merely as a substring — the
	// threshold column legitimately renders "20.00%", which contains
	// "0.00%" as a substring and would make a bare strings.Contains check
	// a false positive.
	if strings.Contains(out, "<td>0.00%</td>") {
		t.Error("output prints a bare 0.00% allocation share when HasAllocShare is false")
	}
	if !strings.Contains(out, "no heap-churn samples") {
		t.Error("output does not explain the missing allocation-share reading")
	}
}
