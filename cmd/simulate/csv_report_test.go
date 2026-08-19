package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProvenance() provenance {
	return provenance{
		rootSeedHex: "aabbcc",
		gitSHA:      "cafef00d",
		matches:     10,
		players:     2,
		bots:        "drifter",
		sweepSpec:   "LeaseCostPerBlock:1,2",
	}
}

func testHeader() []string {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2"})
	if err != nil {
		panic(err)
	}
	return buildHeader(dims)
}

func TestBuildHeaderShape(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	header := buildHeader(dims)

	if header[0] != "status" || header[1] != "error" {
		t.Errorf("header[0:2] = %v, want [status error]", header[0:2])
	}
	if header[2] != "sweep.LeaseCostPerBlock" {
		t.Errorf("header[2] = %q, want sweep.LeaseCostPerBlock", header[2])
	}

	// Every metricSpecs entry must contribute exactly its 4 columns, in
	// the order stats.go declares them — this is what a reader six months
	// from now traces a column name back to summary.go's own field
	// comment through.
	wantMetricCols := len(metricSpecs) * 4
	gotMetricCols := 0
	for _, h := range header {
		for _, suffix := range []string{"_mean", "_half_width", "_n", "_excluded"} {
			if len(h) > len(suffix) && strings.HasSuffix(h, suffix) {
				gotMetricCols++
			}
		}
	}
	if gotMetricCols != wantMetricCols {
		t.Errorf("metric columns = %d, want %d", gotMetricCols, wantMetricCols)
	}
}

func TestOpenReportCreatesFreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()
	header := testHeader()

	report, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() = %v", err)
	}
	if len(report.done) != 0 {
		t.Errorf("len(done) = %d, want 0 for a fresh file", len(report.done))
	}
	if report.hadPriorError {
		t.Error("hadPriorError = true, want false for a fresh file")
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	got := string(data)
	wantProv := prov.line() + "\n"
	if len(got) < len(wantProv) || got[:len(wantProv)] != wantProv {
		t.Errorf("file does not start with the provenance line %q:\n%s", wantProv, got)
	}
}

func TestOpenReportResumeSkipsExistingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()
	header := testHeader()

	report, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	row := map[string]string{"status": "ok", "sweep.LeaseCostPerBlock": "1"}
	if err := report.writeRow(row); err != nil {
		t.Fatalf("writeRow() = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	resumed, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (resume) = %v", err)
	}
	defer func() { _ = resumed.close() }()

	if len(resumed.done) != 1 {
		t.Fatalf("len(done) = %d, want 1", len(resumed.done))
	}

	// The skip-set must be keyed the same way configKey derives it from a
	// live configPoint, or a resumed sweep would never actually skip
	// anything it already ran.
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	points := expandSweep(dims)
	if !resumed.done[configKey(points[0])] {
		t.Errorf("done[configKey(points[0])] = false, want true — resume's skip-set key format diverged from configKey's")
	}
	if resumed.done[configKey(points[1])] {
		t.Error("done[configKey(points[1])] = true, want false — only LeaseCostPerBlock=1 was recorded")
	}
}

// TestOpenReportResumeTracksPriorErrors is the CodeRabbit-flagged bug this
// package's own fix addresses: an error-status row already on disk must
// still be visible to a resumed run's exit status, even though the
// configuration itself is skipped rather than retried (retrying a
// deterministic failure can only reproduce it).
func TestOpenReportResumeTracksPriorErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()
	header := testHeader()

	report, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.writeRow(map[string]string{"status": "error", "error": "synthetic", "sweep.LeaseCostPerBlock": "1"}); err != nil {
		t.Fatalf("writeRow() = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	resumed, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (resume) = %v", err)
	}
	defer func() { _ = resumed.close() }()

	if !resumed.hadPriorError {
		t.Error("hadPriorError = false, want true — the existing file has an error-status row")
	}

	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	points := expandSweep(dims)
	if !resumed.done[configKey(points[0])] {
		t.Error("done[configKey(points[0])] = false, want true — an error row is still skipped on resume, not retried")
	}
}

// TestOpenReportResumeRejectsInvalidStatus and
// TestOpenReportResumeRejectsDuplicateKey are the CodeRabbit-flagged gap:
// the reader used to accept any non-"ok" value as an implicit error row,
// and silently let a later row overwrite an earlier one's entry in done.
// Both are symptoms of the same thing — a corrupted or hand-edited file —
// and both must now fail closed rather than let a resume misclassify or
// silently skip a configuration.
//
// Both tests write their bad row through writeRow directly: writeRow
// itself performs no content validation (nothing this package's own
// production code ever writes needs it to), so it's the natural way to
// construct a file only a corrupted write or manual edit could produce.
func TestOpenReportResumeRejectsInvalidStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()
	header := testHeader()

	report, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.writeRow(map[string]string{"status": "bogus", "sweep.LeaseCostPerBlock": "1"}); err != nil {
		t.Fatalf("writeRow() = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	if _, err := openReport(path, prov, header); err == nil {
		t.Error("openReport() over a row with an unrecognized status = nil error, want an error")
	}
}

func TestOpenReportResumeRejectsDuplicateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()
	header := testHeader()

	report, err := openReport(path, prov, header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.writeRow(map[string]string{"status": "ok", "sweep.LeaseCostPerBlock": "1"}); err != nil {
		t.Fatalf("writeRow() (1) = %v", err)
	}
	if err := report.writeRow(map[string]string{"status": "ok", "sweep.LeaseCostPerBlock": "1"}); err != nil {
		t.Fatalf("writeRow() (2) = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	if _, err := openReport(path, prov, header); err == nil {
		t.Error("openReport() over a file with a duplicate configuration row = nil error, want an error")
	}
}

func TestOpenReportResumeProvenanceMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	header := testHeader()

	report, err := openReport(path, testProvenance(), header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	changed := testProvenance()
	changed.matches = 999
	if _, err := openReport(path, changed, header); err == nil {
		t.Error("openReport() with a different provenance = nil error, want an error")
	}
}

// TestOpenReportResumeGitSHAMismatch is the CodeRabbit-flagged gap:
// without git SHA in provenance, a binary built from a different commit
// but given identical flags could resume into a file and mix rows
// computed by different simulation code. Same invocation, same flags,
// only the git SHA differs — must still be rejected.
func TestOpenReportResumeGitSHAMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	header := testHeader()

	report, err := openReport(path, testProvenance(), header)
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	changed := testProvenance()
	changed.gitSHA = "deadbeef"
	if _, err := openReport(path, changed, header); err == nil {
		t.Error("openReport() with a different git SHA = nil error, want an error")
	}
}

func TestOpenReportResumeHeaderMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	prov := testProvenance()

	report, err := openReport(path, prov, testHeader())
	if err != nil {
		t.Fatalf("openReport() (first) = %v", err)
	}
	if err := report.close(); err != nil {
		t.Fatalf("close() = %v", err)
	}

	differentHeader := append([]string{"extra_column"}, testHeader()...)
	if _, err := openReport(path, prov, differentHeader); err == nil {
		t.Error("openReport() with a different header = nil error, want an error")
	}
}

func TestOpenReportResumeCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := os.WriteFile(path, []byte("not a cinzal-simulate file at all"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	if _, err := openReport(path, testProvenance(), testHeader()); err == nil {
		t.Error("openReport() against a corrupt file = nil error, want an error")
	}
}

// TestCSVReportCloseReportsError is the other half of the CodeRabbit-
// flagged close-error bug: close() itself must surface a failure (a final
// flush that didn't land, e.g. a full disk) rather than swallow it — it's
// run.go's job to turn that into a non-zero exit status, but it can only
// do that if close() actually returns the error.
func TestCSVReportCloseReportsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	report, err := openReport(path, testProvenance(), testHeader())
	if err != nil {
		t.Fatalf("openReport() = %v", err)
	}

	// Close the underlying file out from under the csvReport, so its own
	// close() call fails the way a real write failure would.
	if err := report.f.Close(); err != nil {
		t.Fatalf("f.Close() = %v", err)
	}

	if err := report.close(); err == nil {
		t.Error("close() on an already-closed file = nil error, want an error")
	}
}
