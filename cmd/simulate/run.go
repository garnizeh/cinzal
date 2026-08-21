package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"strconv"
	"strings"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/telemetry"
)

// defaultRootSeedString is --seed's default value: a fixed, documented
// constant, so an invocation that never names --seed is still reproducible
// months later (issue #200: "a sweep re-run months later against a changed
// Config is comparing the same maps"). The "v1" is this string's own
// version, not the CSV format's — bumping it changes every default-seeded
// sweep's maps, so it never changes silently.
const defaultRootSeedString = "cinzal-simulate-default-root-seed-v1"

// matchRunner is RunMany's own signature, named so run's per-configuration
// loop can take it as a parameter: production wires the real RunMany,
// tests wire a fake that fails on command, without either needing to coax
// a real match into failing to exercise the sweep's error-row path.
type matchRunner func(seeds [][32]byte, cfg game.Config, players int, bot bots.Bot, workers int) ([]MatchResult, error)

// run is main's entire implementation, factored out so it can be driven
// directly from a test with a controlled argv and a captured stderr,
// instead of only through a subprocess.
func run(args []string, stderr io.Writer) int {
	return runWithDeps(args, stderr, RunMany, gitSHA)
}

// logLine writes one diagnostic or progress line to w, discarding the
// write error deliberately: a failure to write a stderr line isn't
// actionable here, and every call site already reports its own error (or
// none) through run's return code, not through this line landing.
func logLine(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func runWithDeps(args []string, stderr io.Writer, runMatches matchRunner, getGitSHA func() (string, error)) (status int) {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	matches := fs.Int("matches", 0, "matches to run per configuration")
	players := fs.Int("players", 0, "player count for every match in the sweep")
	botsFlag := fs.String("bots", "", "bot tier every seat plays: drifter, runner, or operator")
	outPath := fs.String("out", "", "CSV output path")
	breakdownPath := fs.String("breakdown", "", "optional second CSV: issue #205's per-round, per-card and Tier IV decompositions of §22 rows 1, 6 and 4 (see breakdown.go)")
	seedFlag := fs.String("seed", defaultRootSeedString, "root seed string; the 32-byte seed is SHA-256 of this string")
	workers := fs.Int("workers", 0, "matches to run concurrently per configuration; <=0 uses GOMAXPROCS")
	var sweepRaw sweepFlags
	fs.Var(&sweepRaw, "sweep", "Field=v1,v2,... — repeatable; names a game.Config field to sweep")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *matches <= 0 {
		logLine(stderr, "cmd/simulate: --matches must be > 0")
		return 1
	}
	if *players <= 0 {
		logLine(stderr, "cmd/simulate: --players is required and must be > 0")
		return 1
	}
	if *outPath == "" {
		logLine(stderr, "cmd/simulate: --out is required")
		return 1
	}
	tier, err := parseTier(*botsFlag)
	if err != nil {
		logLine(stderr, "cmd/simulate: %v", err)
		return 1
	}

	dims, err := parseSweepFlags(sweepRaw)
	if err != nil {
		logLine(stderr, "%v", err)
		return 1
	}

	points := expandSweep(dims)

	var invalid []string
	for _, p := range points {
		if err := p.cfg.Validate(*players); err != nil {
			invalid = append(invalid, fmt.Sprintf("%s: %v", sweepValuesString(dims, p), err))
		}
	}
	if len(invalid) > 0 {
		logLine(stderr, "cmd/simulate: the following swept configurations fail Config.Validate:")
		for _, m := range invalid {
			logLine(stderr, "  %s", m)
		}
		return 1
	}

	sha, err := getGitSHA()
	if err != nil {
		logLine(stderr, "cmd/simulate: git SHA: %v", err)
		return 1
	}

	rootSeed := sha256.Sum256([]byte(*seedFlag))
	rootSeedHex := hex.EncodeToString(rootSeed[:])
	seeds := deriveSeeds(rootSeed, *matches)

	prov := provenance{
		rootSeedHex: rootSeedHex,
		gitSHA:      sha,
		matches:     *matches,
		players:     *players,
		bots:        strings.ToLower(*botsFlag),
		sweepSpec:   sweepSpecString(dims),
	}
	header := buildHeader(dims)

	report, err := openReport(*outPath, prov, header)
	if err != nil {
		logLine(stderr, "%v", err)
		return 1
	}
	defer func() {
		if cerr := report.close(); cerr != nil {
			// A close failure means the final flush didn't land — a full
			// disk, most plausibly — so the CSV on disk may be missing
			// data this run believes it wrote. That has to override
			// whatever status the function body already decided on.
			logLine(stderr, "cmd/simulate: close report: %v", cerr)
			status = 1
		}
	}()

	var breakdown *breakdownReport
	if *breakdownPath != "" {
		// A resumed sweep skips configurations already in the --out file,
		// and the breakdown file has no resume path of its own
		// (breakdownReport), so those configurations' rows would simply be
		// absent from a breakdown written alongside the resume — a file
		// that looks complete and silently measured less than it claims.
		// Refuse the combination rather than produce it.
		if len(report.done) > 0 {
			logLine(stderr, "cmd/simulate: --breakdown cannot be combined with resuming %s: %d configuration(s) already recorded there would have no breakdown rows", *outPath, len(report.done))
			return 1
		}
		breakdown, err = openBreakdownReport(*breakdownPath, prov, breakdownHeader(dims))
		if err != nil {
			logLine(stderr, "%v", err)
			return 1
		}
		defer func() {
			if cerr := breakdown.close(); cerr != nil {
				logLine(stderr, "cmd/simulate: close breakdown: %v", cerr)
				status = 1
			}
		}()
	}

	bot := bots.For(tier)
	// A resumed file that already holds an error-status row starts this
	// run's own exit status as failed too: that configuration is skipped
	// below rather than retried (see csvReport.hadPriorError's own
	// comment), and skipping it must not look like success.
	anyErr := report.hadPriorError
	if anyErr {
		logLine(stderr, "cmd/simulate: resumed file already contains an error-status row; exit status will be non-zero")
	}

	for i, p := range points {
		label := sweepValuesString(dims, p)
		key := configKey(p)
		if report.done[key] {
			logLine(stderr, "[%d/%d] %s: already recorded, skipping", i+1, len(points), label)
			continue
		}

		logLine(stderr, "[%d/%d] %s: running %d matches...", i+1, len(points), label, *matches)

		row := map[string]string{
			"players":     strconv.Itoa(*players),
			"bot_tier":    prov.bots,
			"match_count": strconv.Itoa(*matches),
			"root_seed":   rootSeedHex,
			"git_sha":     sha,
		}
		for j, d := range dims {
			row["sweep."+d.field] = formatScalar(p.values[j])
		}
		cfgJSON, err := json.Marshal(p.cfg)
		if err != nil {
			// game.Config's fields are all plain, exported, JSON-safe
			// types — a marshal failure here is a programming error, not
			// a runtime condition to degrade from.
			panic(fmt.Sprintf("cmd/simulate: marshal Config: %v", err))
		}
		row["config_json"] = string(cfgJSON)

		// The breakdown file's fixed columns are the sweep row's own, minus
		// the Config itself and plus its digest — see breakdownHeader.
		breakdownFixed := maps.Clone(row)
		delete(breakdownFixed, "config_json")
		cfgDigest := sha256.Sum256(cfgJSON)
		breakdownFixed["config_sha256"] = hex.EncodeToString(cfgDigest[:])

		// nil means "this configuration measured nothing" — the error row,
		// distinct from a configuration that ran and produced zero rows,
		// which aggregateBreakdowns never returns.
		var breakdownStats []breakdownStat

		results, runErr := runMatches(seeds, p.cfg, *players, bot, *workers)
		if runErr != nil {
			// Fails closed (issue #200's own acceptance criterion): a
			// configuration whose matches did not all reach cfg.Rounds is
			// written as an error row, not omitted and not averaged over
			// whatever survived — RunMany itself already refuses to
			// return a partial []MatchResult alongside an error, so there
			// is nothing to average here even if this code wanted to.
			row["status"] = "error"
			row["error"] = runErr.Error()
			row["matches_completed"] = "0"
			breakdownFixed["status"] = "error"
			breakdownFixed["error"] = runErr.Error()
			breakdownFixed["matches_completed"] = "0"
			anyErr = true
			logLine(stderr, "[%d/%d] %s: ERROR: %v", i+1, len(points), label, runErr)
		} else {
			row["status"] = "ok"
			row["matches_completed"] = strconv.Itoa(len(results))
			summaries := make([]telemetry.MatchSummary, len(results))
			for k, res := range results {
				summaries[k] = res.Summary
			}
			for j, m := range computeMetrics(summaries) {
				spec := metricSpecs[j]
				switch {
				case m.ok:
					row[spec.name+"_mean"] = strconv.FormatFloat(m.mean, 'f', 6, 64)
					row[spec.name+"_half_width"] = strconv.FormatFloat(m.halfWidth, 'f', 6, 64)
				case m.n >= 2:
					// Zero-width interval: every included match agreed, so
					// the mean is the constant value itself and worth
					// keeping (issue #249), but half_width stays empty —
					// its absence is what tells a reader this is not a
					// measurement to read a threshold verdict off of.
					row[spec.name+"_mean"] = strconv.FormatFloat(m.mean, 'f', 6, 64)
				}
				row[spec.name+"_n"] = strconv.Itoa(m.n)
				row[spec.name+"_excluded"] = strconv.Itoa(m.excluded)
			}
			if breakdown != nil {
				breakdownFixed["status"] = "ok"
				breakdownFixed["matches_completed"] = strconv.Itoa(len(results))
				breakdowns := make([]Breakdown, len(results))
				for k, res := range results {
					breakdowns[k] = res.Breakdown
				}
				breakdownStats = aggregateBreakdowns(breakdowns, p.cfg)
			}
			logLine(stderr, "[%d/%d] %s: done", i+1, len(points), label)
		}

		// The --out row lands first, unconditionally, and a --breakdown
		// failure below can no longer skip it. --out is the authoritative
		// file (issue #200: "the CSV is a document, not a debug dump") and
		// --breakdown is a second view of the same run, so a configuration
		// present in the breakdown and absent from the sweep is the one
		// disagreement between the two files that must not be reachable.
		if err := report.writeRow(row); err != nil {
			logLine(stderr, "cmd/simulate: write row: %v", err)
			return 1
		}
		if breakdown != nil {
			var err error
			if breakdownStats == nil {
				err = breakdown.writeErrorRow(breakdownFixed)
			} else {
				err = breakdown.writeRows(breakdownFixed, breakdownStats)
			}
			if err != nil {
				logLine(stderr, "cmd/simulate: write breakdown row: %v", err)
				return 1
			}
		}
	}

	if anyErr {
		return 1
	}
	return 0
}

func parseTier(s string) (bots.Tier, error) {
	switch strings.ToLower(s) {
	case "drifter":
		return bots.Drifter, nil
	case "runner":
		return bots.Runner, nil
	case "operator":
		return bots.Operator, nil
	default:
		return 0, fmt.Errorf("--bots %q: want drifter, runner, or operator", s)
	}
}

// deriveSeeds derives n match seeds from root, shared across every
// configuration in the sweep (run's own call site never derives a second
// set) — the same HMAC-SHA256 construction internal/rules/rng.go's own
// NewBotRNG uses to derive its seed from a match seed, applied here to
// derive a match seed from the sweep's root seed instead.
func deriveSeeds(root [32]byte, n int) [][32]byte {
	seeds := make([][32]byte, n)
	for i := range seeds {
		msg := binary.BigEndian.AppendUint64([]byte("cinzal.simulate.seed"), uint64(i))
		mac := hmac.New(sha256.New, root[:])
		mac.Write(msg)
		copy(seeds[i][:], mac.Sum(nil))
	}
	return seeds
}

// gitSHA reports the running binary's own commit — provenance every row
// carries, per issue #200 ("a number whose provenance is a shell history
// entry is a number nobody can act on"). Failing to resolve it is a hard
// error rather than an "unknown" placeholder: a CSV meant to be read in six
// months is exactly the artifact this repository's "gates fail closed"
// discipline (CLAUDE.md) is written for, even though this isn't a CI gate.
func gitSHA() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
