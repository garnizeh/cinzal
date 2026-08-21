package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// provenance is the first line of every CSV this command writes: the
// facts issue #200 requires travel with the file rather than in a shell
// history entry ("a number whose provenance is a shell history entry is a
// number nobody can act on"), and the facts a resume must agree with
// before it appends a single row ("checked against the file's header so a
// resume cannot silently splice two different sweeps together"). gitSHA is
// part of that agreement, not just a per-row column: without it here, a
// resume only checks the *invocation* (seed, matches, players, bots,
// sweep), not the *code* that ran it — a binary rebuilt from a different
// commit but given the identical flags would pass every other check and
// silently append rows computed by different simulation logic into the
// same file.
type provenance struct {
	rootSeedHex string
	gitSHA      string
	matches     int
	players     int
	bots        string
	sweepSpec   string
}

func (p provenance) line() string {
	return fmt.Sprintf("# cinzal-simulate v1 seed=%s git_sha=%s matches=%d players=%d bots=%s sweep=%s",
		p.rootSeedHex, p.gitSHA, p.matches, p.players, p.bots, p.sweepSpec)
}

// breakdownLine is the same provenance under its own format marker. The two
// files carry different columns and, for the breakdown, no resume path at
// all, so a marker that told them apart only by column layout would let a
// reader — or a later tool — mistake one for a truncated version of the
// other.
func (p provenance) breakdownLine() string {
	return fmt.Sprintf("# cinzal-simulate-breakdown v1 seed=%s git_sha=%s matches=%d players=%d bots=%s sweep=%s",
		p.rootSeedHex, p.gitSHA, p.matches, p.players, p.bots, p.sweepSpec)
}

// buildHeader lists every column this command writes, in the order issue
// #200 states: "the swept parameters, the player count, the bot tier, the
// match count, then every §22 field" — with status/error first (so a
// reader scanning a row for "did this work" never has to scroll right) and
// the remaining provenance columns (root seed, git SHA, full Config)
// folded in beside match_count, per the issue's separate "every row
// carries" requirement.
//
// A "_mean" cell can appear with its "_half_width" sibling empty: that pair
// is a zero-width interval, a constant vector kept as a fact rather than an
// interval (issue #249) — read the mean, not a confidence bound, off it.
// Both cells empty together still means "not a measurement," same as
// before. CSVs written before #249 (docs/exit-demos/203, 204, 229, 205) use
// the older rule and print a non-empty "_half_width=0.000000" for what was
// actually a constant vector; they were not regenerated, since the
// underlying numbers are unchanged.
func buildHeader(dims []sweepDim) []string {
	header := []string{"status", "error"}
	for _, d := range dims {
		header = append(header, "sweep."+d.field)
	}
	header = append(header, "players", "bot_tier", "match_count", "matches_completed", "root_seed", "git_sha", "config_json")
	for _, spec := range metricSpecs {
		header = append(header,
			spec.name+"_mean",
			spec.name+"_half_width",
			spec.name+"_n",
			spec.name+"_excluded",
		)
	}
	return header
}

// csvReport is the open --out file: either freshly created (provenance and
// header just written) or resumed (provenance and header verified against
// an existing file, done populated from its data rows). Every writeRow
// call flushes immediately — issue #200's own "results streamed to the CSV
// as each configuration completes" is what makes an interrupted sweep
// resumable at configuration granularity instead of losing everything back
// to the last full flush.
type csvReport struct {
	f      *os.File
	w      *csv.Writer
	header []string
	done   map[string]bool

	// hadPriorError is true when a resumed file already held at least one
	// error-status row. A configuration that already failed once will
	// fail identically on a retry (the whole system is deterministic:
	// same shared seed set, same Config), so resuming skips it rather
	// than re-running a foregone conclusion — but that failure still has
	// to reach run's exit code, or a resumed sweep over a file with a
	// standing error row would silently report success.
	hadPriorError bool
}

// openReport opens outPath for a sweep described by prov and header. A
// nonexistent file is created fresh. An existing file is resumed: its own
// provenance and header lines are compared verbatim against prov and
// header — any difference is issue #200's "resuming against a file whose
// header disagrees exits non-zero," extended here to the full column
// layout, not just the root seed and sweep set, because a header that
// changed shape underneath a resume (a metric renamed or added since the
// file was started) is the same splicing hazard under a different name.
func openReport(outPath string, prov provenance, header []string) (*csvReport, error) {
	existing, err := os.Open(outPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return createReport(outPath, prov, header)
	case err != nil:
		return nil, fmt.Errorf("cmd/simulate: open %s: %w", outPath, err)
	}
	defer func() { _ = existing.Close() }() // read-only handle, already fully consumed by the point this runs

	done, hadPriorError, err := verifyAndReadExisting(existing, prov, header)
	if err != nil {
		return nil, fmt.Errorf("cmd/simulate: resume %s: %w", outPath, err)
	}

	f, err := os.OpenFile(outPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cmd/simulate: reopen %s for append: %w", outPath, err)
	}
	return &csvReport{f: f, w: csv.NewWriter(f), header: header, done: done, hadPriorError: hadPriorError}, nil
}

func createReport(outPath string, prov provenance, header []string) (*csvReport, error) {
	f, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("cmd/simulate: create %s: %w", outPath, err)
	}
	if _, err := fmt.Fprintln(f, prov.line()); err != nil {
		return nil, fmt.Errorf("cmd/simulate: write provenance line: %w", err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("cmd/simulate: write header: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("cmd/simulate: write header: %w", err)
	}
	return &csvReport{f: f, w: w, header: header, done: map[string]bool{}}, nil
}

// verifyAndReadExisting reads f's provenance and header lines, comparing
// them verbatim against prov and header, then reads every data row that
// follows to build the resume skip-set keyed by each row's own swept
// values (existingRowKey) — the same key format configKey produces for a
// candidate configPoint, so the two agree without either depending on
// where the fixed columns happen to sit in header. hadPriorError is true
// when any row already on disk has status != "ok" — see csvReport's own
// field comment for why that still has to reach an exit code even though
// the configuration itself is skipped, not retried.
func verifyAndReadExisting(f *os.File, prov provenance, header []string) (done map[string]bool, hadPriorError bool, err error) {
	br := bufio.NewReader(f)

	line1, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, fmt.Errorf("read provenance line: %w", err)
	}
	line1 = strings.TrimRight(line1, "\n")
	if line1 != prov.line() {
		return nil, false, fmt.Errorf("existing file's provenance line does not match this invocation:\n  have: %s\n  want: %s", line1, prov.line())
	}

	r := csv.NewReader(br)
	gotHeader, err := r.Read()
	if err != nil {
		return nil, false, fmt.Errorf("read header line: %w", err)
	}
	if !slices.Equal(gotHeader, header) {
		return nil, false, fmt.Errorf("existing file's header does not match this invocation's columns:\n  have: %s\n  want: %s", strings.Join(gotHeader, ","), strings.Join(header, ","))
	}

	sweepIdx := make([]int, 0)
	statusIdx := -1
	for i, h := range header {
		if strings.HasPrefix(h, "sweep.") {
			sweepIdx = append(sweepIdx, i)
		}
		if h == "status" {
			statusIdx = i
		}
	}
	// buildHeader always puts "status" first, and gotHeader was just
	// checked equal to header above — statusIdx == -1 here would mean
	// that equality check was wrong, not a real input to handle.

	done = map[string]bool{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("read data row: %w", err)
		}

		// A duplicate configuration row or a status outside {ok, error}
		// only happens if the file was corrupted, hand-edited, or written
		// by something other than this command — silently accepting
		// either would let a resume skip (or worse, misclassify) a
		// configuration this run believes is genuinely recorded.
		key := existingRowKey(row, sweepIdx)
		if done[key] {
			return nil, false, fmt.Errorf("existing file has more than one row for configuration %q", key)
		}
		switch row[statusIdx] {
		case "ok":
		case "error":
			hadPriorError = true
		default:
			return nil, false, fmt.Errorf("existing file has status %q for configuration %q, want \"ok\" or \"error\"", row[statusIdx], key)
		}
		done[key] = true
	}
	return done, hadPriorError, nil
}

// existingRowKey is configKey's counterpart for a CSV row already on disk:
// the same swept values, in the same order, joined the same way.
func existingRowKey(row []string, sweepIdx []int) string {
	parts := make([]string, len(sweepIdx))
	for i, c := range sweepIdx {
		parts[i] = row[c]
	}
	return strings.Join(parts, "\x1f")
}

// writeRow appends one configuration's result to the CSV, in header order,
// and flushes immediately (see csvReport's own doc comment). row is keyed
// by column name rather than position so callers can't misalign a value
// with the wrong column by miscounting; any column a caller leaves unset
// writes as "".
func (r *csvReport) writeRow(row map[string]string) error {
	rec := make([]string, len(r.header))
	for i, h := range r.header {
		rec[i] = row[h]
	}
	if err := r.w.Write(rec); err != nil {
		return err
	}
	r.w.Flush()
	return r.w.Error()
}

func (r *csvReport) close() error {
	r.w.Flush()
	if err := r.w.Error(); err != nil {
		return err
	}
	return r.f.Close()
}
