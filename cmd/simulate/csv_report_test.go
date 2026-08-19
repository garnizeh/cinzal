package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testProvenance() provenance {
	return provenance{
		rootSeedHex: "aabbcc",
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
			if len(h) > len(suffix) && h[len(h)-len(suffix):] == suffix {
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
