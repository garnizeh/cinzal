package main

import (
	"encoding/csv"
	"fmt"
	"maps"
	"os"
	"strconv"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// breakdownStat is one aggregated line of the --breakdown CSV: one
// (block, key) of one configuration, reduced across that configuration's
// whole match vector.
//
// Two numbers, not one, and they answer different questions. Numerator and
// Denominator are pooled totals across every match — the population size
// issue #205 asks to see printed beside R1's rate ("the denominator is the
// demonstration"), and the only honest way to report a per-card R6 rate over
// a deck that draws 13 of 16. Mean and HalfWidth are D35's own reduction,
// where the match is the sampling unit and each match contributes one number
// before any pooling happens; they are what a threshold verdict is read off.
// The two differ whenever matches carry different weights, which for a
// per-round or per-card split is most of the time.
type breakdownStat struct {
	block, key, label      string
	numerator, denominator int
	mean, halfWidth        float64
	n, excluded            int
	ok                     bool
}

// breakdownAccum builds one breakdownStat across a configuration's matches.
type breakdownAccum struct {
	block, key, label      string
	numerator, denominator int
	values                 []float64
	excluded               int
}

func newAccum(block, key, label string, matches int) *breakdownAccum {
	return &breakdownAccum{block: block, key: key, label: label, values: make([]float64, 0, matches)}
}

// ratio records one match's (numerator, denominator) contribution to a
// rate-shaped row. A match whose denominator is zero — no route submitted
// that round, this card not in this match's deck, no Tier IV contract ever
// accepted — is excluded from the vector rather than contributing a zero,
// and the exclusion is reported beside the interval: D35's own rule, and the
// reason every row here carries per_match_excluded.
func (a *breakdownAccum) ratio(num, den int) {
	if den <= 0 {
		a.excluded++
		return
	}
	a.numerator += num
	a.denominator += den
	a.values = append(a.values, float64(num)/float64(den))
}

// count records one match's own count for a row whose population is the
// match itself — every match is in it by construction, so nothing is ever
// excluded and the pooled denominator is the match count.
func (a *breakdownAccum) count(v int) {
	a.numerator += v
	a.denominator++
	a.values = append(a.values, float64(v))
}

// indicator is count for a 0/1 row, so a caller never has to spell the
// conversion out at each call site.
func (a *breakdownAccum) indicator(v bool) {
	if v {
		a.count(1)
		return
	}
	a.count(0)
}

func (a *breakdownAccum) stat() breakdownStat {
	mean, halfWidth, n, ok := interval(a.values)
	return breakdownStat{
		block: a.block, key: a.key, label: a.label,
		numerator: a.numerator, denominator: a.denominator,
		mean: mean, halfWidth: halfWidth, n: n, excluded: a.excluded, ok: ok,
	}
}

// r1LotteryThreshold is GDD §22 row 1's own failing line, 15% of submitted
// routes. It appears here as well as in the verdict because issue #205 asks
// for the distribution and not only the mean: the share of individual
// matches that are themselves over the line is what separates "fifteen
// percent everywhere" from "fifteen percent in a handful of matches."
const r1LotteryThreshold = 0.15

// aggregateBreakdowns reduces one configuration's per-match Breakdowns to
// the rows the --breakdown CSV writes, in a fixed order that does not depend
// on map iteration: rounds ascending, then incident cards in
// rules.IncidentCatalog's canonical order, then R7's own rows.
func aggregateBreakdowns(bs []Breakdown, cfg game.Config) []breakdownStat {
	n := len(bs)

	overall := newAccum("r1_overall", "all", "halted / submitted non-empty routes", n)
	overThreshold := newAccum("r1_match_over_threshold", "0.15", "matches whose own rate clears row 1's failing line", n)
	byRound := make([]*breakdownAccum, cfg.Rounds)
	for i := range byRound {
		byRound[i] = newAccum("r1_round", strconv.Itoa(i+1), "halted / submitted, this round only", n)
	}

	for k, b := range bs {
		// Every Breakdown here was produced by breakdownTracker.finish
		// under the same cfg, so all three lengths agree by construction.
		// Checked anyway, and loudly: this is a package-level function
		// taking any slice, and the two silent alternatives are both worse
		// than a named panic. Indexing past byRound panics with nothing but
		// an offset to go on, and truncating to the shorter of the two
		// writes a per-round table that is quietly missing rounds — the
		// exact shape of "a gate that passes when it can't run" (CLAUDE.md)
		// applied to a document nobody would re-derive.
		if len(b.RoutesSubmittedByRound) != cfg.Rounds || len(b.RoutesHaltedByRound) != cfg.Rounds {
			panic(fmt.Sprintf("cmd/simulate: aggregateBreakdowns: match %d has %d submitted / %d halted per-round entries, want %d each (cfg.Rounds)",
				k, len(b.RoutesSubmittedByRound), len(b.RoutesHaltedByRound), cfg.Rounds))
		}

		submitted, halted := 0, 0
		for i := range b.RoutesSubmittedByRound {
			byRound[i].ratio(b.RoutesHaltedByRound[i], b.RoutesSubmittedByRound[i])
			submitted += b.RoutesSubmittedByRound[i]
			halted += b.RoutesHaltedByRound[i]
		}
		overall.ratio(halted, submitted)
		if submitted <= 0 {
			overThreshold.excluded++
			continue
		}
		overThreshold.indicator(float64(halted)/float64(submitted) > r1LotteryThreshold)
	}

	stats := []breakdownStat{overall.stat(), overThreshold.stat()}
	for _, a := range byRound {
		stats = append(stats, a.stat())
	}

	incidentOverall := newAccum("r6_overall", "all", "live incident rounds that hit at least one seat", n)
	catalog := rules.IncidentCatalog()
	byCard := make([]*breakdownAccum, len(catalog))
	for i, c := range catalog {
		label := "boon"
		if c.Hazard {
			label = "hazard"
		}
		byCard[i] = newAccum("r6_card", c.Name, label, n)
	}
	for _, b := range bs {
		live, hit := 0, 0
		for i, c := range catalog {
			if !b.IncidentLive[c.ID] {
				byCard[i].excluded++
				continue
			}
			live++
			if b.IncidentHit[c.ID] {
				hit++
				byCard[i].indicator(true)
				continue
			}
			byCard[i].indicator(false)
		}
		incidentOverall.ratio(hit, live)
	}
	stats = append(stats, incidentOverall.stat())
	for _, a := range byCard {
		stats = append(stats, a.stat())
	}

	finalInfamy9 := newAccum("r7_infamy9", "final", "row 4 as the verdict reads it: any seat at 9+ at final scoring", n)
	everInfamy9 := newAccum("r7_infamy9", "ever", "any seat at 9+ at the end of any round", n)
	offered := newAccum("r7_tier4", "offered", "Tier IV offers put in front of a seat", n)
	accepted := newAccum("r7_tier4", "accepted", "Tier IV contracts accepted", n)
	deliveredT4 := newAccum("r7_tier4", "delivered", "Tier IV contracts delivered", n)
	abandoned := newAccum("r7_tier4", "abandoned", "Tier IV contracts accepted and then missed", n)
	unresolved := newAccum("r7_tier4", "unresolved", "Tier IV contracts still held at final scoring", n)
	abandonRate := newAccum("r7_tier4", "abandon_rate", "abandoned / accepted", n)
	settledAbandonRate := newAccum("r7_tier4", "abandon_rate_settled", "abandoned / (delivered + abandoned)", n)
	crossTotal := newAccum("r7_crossing", "total", "transitions from below Infamy 9 to 9+", n)
	crossDelivery := newAccum("r7_crossing", "with_delivery", "crossings whose round held a delivery", n)
	crossStake := newAccum("r7_crossing", "with_stake", "crossings whose round held a post stake", n)
	crossWin := newAccum("r7_crossing", "with_confrontation_win", "crossings whose round held a confrontation win", n)
	crossWinOnly := newAccum("r7_crossing", "confrontation_win_only", "crossings with a win and neither a delivery nor a stake", n)
	crossNone := newAccum("r7_crossing", "no_deliberate_act", "crossings with none of the three acts", n)

	for _, b := range bs {
		finalInfamy9.indicator(b.AnyPlayerFinallyReachedInfamy9)
		everInfamy9.indicator(b.AnyPlayerEverReachedInfamy9)
		offered.count(b.Tier4Offered)
		accepted.count(b.Tier4Accepted)
		deliveredT4.count(b.Tier4Delivered)
		abandoned.count(b.Tier4Abandoned)
		unresolved.count(b.Tier4Unresolved)
		abandonRate.ratio(b.Tier4Abandoned, b.Tier4Accepted)
		settledAbandonRate.ratio(b.Tier4Abandoned, b.Tier4Delivered+b.Tier4Abandoned)
		crossTotal.count(b.Crossings.Total)
		crossDelivery.count(b.Crossings.WithDelivery)
		crossStake.count(b.Crossings.WithStake)
		crossWin.count(b.Crossings.WithConfrontationWin)
		crossWinOnly.count(b.Crossings.ConfrontationWinOnly)
		crossNone.count(b.Crossings.NoDeliberateAct)
	}

	stats = append(stats,
		finalInfamy9.stat(), everInfamy9.stat(),
		offered.stat(), accepted.stat(), deliveredT4.stat(), abandoned.stat(), unresolved.stat(),
		abandonRate.stat(), settledAbandonRate.stat(),
		crossTotal.stat(), crossDelivery.stat(), crossStake.stat(), crossWin.stat(),
		crossWinOnly.stat(), crossNone.stat(),
	)
	return stats
}

// breakdownHeader lists the --breakdown CSV's columns. The fixed provenance
// block matches the sweep CSV's own (buildHeader) so the two files read the
// same way, with one substitution: config_sha256 stands in for config_json.
// This file is long-format — some forty rows per configuration against the
// sweep CSV's one — and repeating a 1.4 KB Config on every one of them would
// bury the numbers the file exists to carry. The digest still pins the
// configuration exactly: it is the SHA-256 of the identical config_json
// string the paired --out CSV writes for the same row.
//
// per_match_mean can appear with per_match_half_width empty — a zero-width
// interval, kept as the constant fact it is rather than dropped (issue
// #249); see buildHeader's own comment for the sweep CSV's identical rule.
func breakdownHeader(dims []sweepDim) []string {
	header := []string{"status", "error"}
	for _, d := range dims {
		header = append(header, "sweep."+d.field)
	}
	header = append(header, "players", "bot_tier", "match_count", "matches_completed", "root_seed", "git_sha", "config_sha256")
	return append(header, "block", "key", "label", "numerator", "denominator", "pooled_rate",
		"per_match_mean", "per_match_half_width", "per_match_n", "per_match_excluded")
}

// breakdownReport is the open --breakdown file. Unlike csvReport it has no
// resume path at all, and openBreakdownReport refuses an existing file
// outright: this format writes many rows per configuration, so the
// one-row-per-configuration skip-set csvReport resumes from cannot express
// "this configuration is half written." Rather than a resume that could
// splice a partial configuration back together, run refuses the combination
// — see its own --breakdown handling.
type breakdownReport struct {
	f      *os.File
	w      *csv.Writer
	header []string
}

func openBreakdownReport(path string, prov provenance, header []string) (*breakdownReport, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cmd/simulate: create breakdown %s: %w", path, err)
	}

	// Every failure below has to close f and remove the file it just
	// created. Leaving it behind is worse here than the leaked descriptor:
	// O_EXCL is what makes this writer refuse to splice two sweeps into one
	// file, so an empty, header-less file left on disk blocks the retry that
	// the error is telling the operator to run.
	fail := func(err error) (*breakdownReport, error) {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}

	if _, err := fmt.Fprintln(f, prov.breakdownLine()); err != nil {
		return fail(fmt.Errorf("cmd/simulate: write breakdown provenance line: %w", err))
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return fail(fmt.Errorf("cmd/simulate: write breakdown header: %w", err))
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fail(fmt.Errorf("cmd/simulate: write breakdown header: %w", err))
	}
	return &breakdownReport{f: f, w: w, header: header}, nil
}

// writeRows appends one configuration's rows, sharing the fixed columns in
// fixed, and flushes — the same per-configuration streaming the sweep CSV
// does, for the same reason.
func (r *breakdownReport) writeRows(fixed map[string]string, stats []breakdownStat) error {
	for _, s := range stats {
		row := make(map[string]string, len(fixed)+10)
		maps.Copy(row, fixed)
		row["block"] = s.block
		row["key"] = s.key
		row["label"] = s.label
		row["numerator"] = strconv.Itoa(s.numerator)
		row["denominator"] = strconv.Itoa(s.denominator)
		if s.denominator > 0 {
			row["pooled_rate"] = strconv.FormatFloat(float64(s.numerator)/float64(s.denominator), 'f', 6, 64)
		}
		switch {
		case s.ok:
			row["per_match_mean"] = strconv.FormatFloat(s.mean, 'f', 6, 64)
			row["per_match_half_width"] = strconv.FormatFloat(s.halfWidth, 'f', 6, 64)
		case s.n >= 2:
			// Zero-width interval: per_match_mean is the constant value
			// every included match agreed on, kept per issue #249;
			// per_match_half_width stays empty so it never reads as a
			// measurement — pooled_rate already carries the same fact.
			row["per_match_mean"] = strconv.FormatFloat(s.mean, 'f', 6, 64)
		}
		row["per_match_n"] = strconv.Itoa(s.n)
		row["per_match_excluded"] = strconv.Itoa(s.excluded)

		if err := r.write(row); err != nil {
			return err
		}
	}
	r.w.Flush()
	return r.w.Error()
}

// writeErrorRow records a configuration whose matches did not all complete,
// as one row with no block of its own — the same fails-closed rule the sweep
// CSV follows (issue #200): a configuration that failed is written as a
// failure, never omitted so the file reads as if it had never been asked for.
func (r *breakdownReport) writeErrorRow(fixed map[string]string) error {
	if err := r.write(fixed); err != nil {
		return err
	}
	r.w.Flush()
	return r.w.Error()
}

func (r *breakdownReport) write(row map[string]string) error {
	rec := make([]string, len(r.header))
	for i, h := range r.header {
		rec[i] = row[h]
	}
	return r.w.Write(rec)
}

func (r *breakdownReport) close() error {
	r.w.Flush()
	if err := r.w.Error(); err != nil {
		return err
	}
	return r.f.Close()
}
