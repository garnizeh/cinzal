# Exit demo: Operator was written against PlayerView alone

**Issue:** [#206](https://github.com/garnizeh/cinzal/issues/206)
**Milestone:** M2 — Bots and simulation
**Kind:** Negative — something must be rejected. Unlike [#203](203-confrontation-load.md)/[#204](204-lease-rate.md)/[#205](205-r1-r6-r7.md), a clean measurement is not the demonstration here: RFC-001 §14.1's claim only means something if the failure mode it predicts — the projection quietly starving a bot the way it would quietly starve a human — is shown *not* to occur, and the CI gates built to catch that failure are shown to actually fire when it is forced to occur anyway.

## Provenance

| | |
|---|---|
| Git SHA (`main`) | `20b639426617e198917a54669c1859fcb110f3f5` |
| Positive-half test | `internal/rules/bots_operator_golden_external_test.go`, `TestOperatorBeatsRunnerOverAThousandMatches` |
| Config | `game.DefaultConfig()`, unmodified |
| Player count | 4 (the test's own fixed cohort size) |
| Matches per cohort | 1,000, paired on one shared golden seed set |

## Positive half — Operator beats Runner over 1,000 matches at 4 players

```console
$ go test ./internal/rules/... -run TestOperatorBeatsRunnerOverAThousandMatches -v
    bots_operator_golden_external_test.go:138: Operator mean RP = 1.903, Runner mean RP = 1.855, margin = 0.049 (1000 matches per cohort)
--- PASS: TestOperatorBeatsRunnerOverAThousandMatches (63.57s)
PASS
```

**Operator mean RP 1.903 vs Runner mean RP 1.855 — margin +0.049, reported regardless of sign per the test's own fails-closed rule** (a zero spread, or either cohort failing to complete all 1,000 matches to round 15, is a hard failure, not a quiet pass). This is up from the +0.006 margin reported when Operator first shipped ([PR #219](https://github.com/garnizeh/cinzal/pull/219)) — the intervening #234/#239 node-count raise and #243 stake-sizing fix moved the game, and the same paired-seed test tracks that shift rather than needing a rewrite. A margin near zero would have been a balance finding, not a test failure (RFC §14.3); this one clears zero by roughly 8× its own PR-219-era value, which is a result worth having, not a coincidence of one run — the test is deterministic (`seed + order log`, RFC §6.3) and reproduces this exact figure on every invocation against this SHA.

## The six §14.3 behaviours, firing from view data alone

RFC-001 §14.3 packs Operator's whole spec into one six-clause sentence. Each clause is independently exercised by a named test against a hand-built `game.PlayerView` — no `rules.MatchState` in reach, only the view fields cited:

| §14.3 clause | View fields read | Test | Result |
|---|---|---|---|
| "Plans across rounds" (no cross-round memory) | Every field above, re-derived fresh each `Decide` call — nothing carried between rounds | `TestBotHasNoCrossRoundState` (`bot_test.go`) | PASS |
| "Reads the heat map for chokepoints and leases them" | `v.NodeStats[id].{ObservedRounds,TrafficRounds}`, `v.You.Posts`, `v.You.Balance` | `TestOperatorLeasesChokepoint` | PASS |
| "Routes around unstable sectors weighted by the displayed deck counts" | `v.Headline.Sector`, `v.Deck.{HazardsRemaining,BoonsRemaining}`, `v.Nodes[id].Sector` | `TestOperatorAvoidsUnstableSectorWeightedByDeck` | PASS |
| "Times the Infamy climb against Contact Cooldown" | `v.You.Infamy`, `v.You.RoundsToNextOffer` | `TestOperatorTimesInfamyAgainstCooldown` | PASS |
| "Buys items when a confrontation looks likely" | `v.Others[i].Infamy`, `v.Archive.Trail[i].{Kind,Round}`, `v.Nodes[id].{Fog,Type,Market}` | `TestOperatorBuysItemsUnderThreat` | PASS |
| "Uses the Ledger when a rival's band jumps" | `v.Anchors[i].{Kind,Round,Actor,Tier}`, `v.You.Balance`, `v.Round`, `cfg.{Rounds,LedgerCost}` | `TestOperatorBuysLedgerOnRivalBandJump` | PASS |

```console
$ go test ./internal/bots/... -run 'TestOperatorLeasesChokepoint|TestOperatorAvoidsUnstableSectorWeightedByDeck|TestOperatorTimesInfamyAgainstCooldown|TestOperatorBuysItemsUnderThreat|TestOperatorBuysLedgerOnRivalBandJump|TestBotHasNoCrossRoundState' -v
--- PASS: TestBotHasNoCrossRoundState (0.01s)
--- PASS: TestOperatorLeasesChokepoint (0.00s)
--- PASS: TestOperatorAvoidsUnstableSectorWeightedByDeck (0.00s)
--- PASS: TestOperatorTimesInfamyAgainstCooldown (0.00s)
--- PASS: TestOperatorBuysItemsUnderThreat (0.00s)
--- PASS: TestOperatorBuysLedgerOnRivalBandJump (0.00s)
PASS
```

No behaviour required a field `game.PlayerView` does not already carry — every one of the six clauses above lists only view fields.

## The projection-gap audit

The scope the issue names is #191 (`internal/bots/legalspace.go` — the shared route/action enumerator every tier including Operator builds on) and #194 (Operator itself). Both are re-checked here rather than taken on faith:

| Found while writing | What | Resolution |
|---|---|---|
| #191 | `SelfState.StepAllowance`/`RoundsToNextOffer` read as zero from `rules.Project` alone | **Not an M1 defect.** [D27](../decisions/D27-project-config-parameter.md) deliberately kept `Project` free of `game.Config` — Config feeds formulas, not visibility — and named the fill-in the caller's job. #191 flagged it as a real pothole for M2 specifically because `internal/match` (D27's intended caller) does not exist yet; #199 closed the gap at the harness level instead, promoting the fill into one exported `rules.ProjectView(s, seat, cfg)` (`internal/rules/fog.go:55-60`), backed by `TestProjectViewStepAllowanceNeverZero` (`internal/rules/projectview_external_test.go`) asserting it non-zero for every seat, every round, at every player count. `cmd/simulate` and the paired-cohort/determinism tests call `ProjectView` directly; `operator_test.go`'s `TestOperatorNeverIllegalOverGoldenCorpus` instead calls `rules.Project` and fills `v.You.StepAllowance` inline itself (predating #199's shared helper) — a duplication worth collapsing onto `ProjectView`, but not itself a fog gap, since the value it fills is identical. `legalspace_test.go`'s own golden-corpus test calls `rules.Project` with no fill at all, which is fine there: `Sample`'s route search reads `Affordances`' `StepsRemaining` (`cfg`-derived), never `v.You.StepAllowance` directly. This is break-on-purpose #3 below. |
| #191 | "The order space is large" (uniform sampling without materialising every order) | Not a projection gap — an algorithmic concern about `Sample`'s own design, resolved by the stage-wise generator `legalspace.go` ships, not by anything `rules` needed to expose. |
| #194 | (six-clause behaviour review, PR #219) | **None.** `v.NodeStats` (Heat Map), `v.Headline`/`v.Deck` (sector incidents), `v.You.RoundsToNextOffer` (Contact Cooldown, already filled in per D27/#199), `v.Others`/`v.Archive.Trail` (threat), and `v.Anchors` (band-jump inference) already carried what Operator needed. No M1 issue was filed against #191 or #194 (confirmed against both issues' cross-reference timelines) — an absence consistent with, not merely asserted by, this audit. |

**An empty list would have been a suspicious result; this one has exactly one real entry**, and it resolves to a harness gap (M2's own responsibility, closed by #199) rather than a fog defect (M1's), which is the distinction the issue asks this audit to draw.

## Break it on purpose

Three deliberate violations, each pushed to a temporary, throwaway PR against `main` so the CI gate runs for real rather than being asserted to run — the same pattern issues #15–#18 used for M1's negative demonstrations. None of the three branches was merged.

### 1. `rules.MatchState` named as a `Decide` parameter, read in Operator's pathfinding

`operatorBot.Decide` gains a `state rules.MatchState` parameter; `chooseObjective` reads `state.Graph.Nodes` before calling `findChokepoint`. This also breaks the `Bot` interface (`operatorBot` no longer implements it), so `go build`/`go vet`/`go test` fail too — expected collateral, not the point.

- Branch/PR: [`exit-demo/206-break1-matchstate-param`, PR #252](https://github.com/garnizeh/cinzal/pull/252)
- Expected gate: `check-bots-isolation` (issue #195), via `scripts/check-bots-isolation.go`
- CI run: [`check` job, run 32478098305](https://github.com/garnizeh/cinzal/actions/runs/32478098305/job/96758535699) — **FAILED**, as expected:
  ```console
  check-bots-isolation: internal/bots may not name MatchState, the graph, or the seed.
    internal/bots/operator.go:165:88: rules.MatchState is not on the isolation allow-list (scripts/bots-isolation-allowlist.txt)
    internal/bots/operator.go:208:63: rules.MatchState is not on the isolation allow-list (scripts/bots-isolation-allowlist.txt)
  make: *** [Makefile:149: bots-isolation] Error 1
  ```
  CI stopped at the `bots-isolation` Make target (`make -k check`'s `check-bots-isolation_test` self-test still ran and passed afterward). The `replay` jobs also failed — expected collateral from the broken `Bot` interface, not the point of this demonstration.

### 2. A `rules.Graph` value read directly, reaching a node the view does not carry

`findChokepoint` declares `var hidden rules.Graph` and returns `hidden.Nodes[0].ID` when non-empty — the unfiltered graph, not `v.Nodes`. This branch **builds and type-checks cleanly**; the isolation gate is the only thing expected to fail.

- Branch/PR: [`exit-demo/206-break2-hidden-node`, PR #253](https://github.com/garnizeh/cinzal/pull/253)
- Expected gate: `check-bots-isolation` (issue #195)
- Confirmed locally before pushing:
  ```console
  $ go run scripts/check-bots-isolation.go
  check-bots-isolation: internal/bots may not name MatchState, the graph, or the seed.
                         RFC-001 §14.5. Offending references:
    internal/bots/operator.go:236:13: rules.Graph is not on the isolation allow-list (scripts/bots-isolation-allowlist.txt)
  ```
- CI run: [`check` job, run 32478179070](https://github.com/garnizeh/cinzal/actions/runs/32478179070/job/96758768246) — **FAILED**, as expected, identical output to the local run above. Confirms this branch is otherwise clean: both `replay` jobs (which need a working build) **passed** — the isolation gate is the only failure, isolating the demonstration from the collateral compile break in #1.

### 3. The `StepAllowance` post-fill deleted from the harness's view assembly

`rules.ProjectView` (`internal/rules/fog.go:55-60`) drops its `v.You.StepAllowance = Steps(v, cfg)` line, leaving `Project`'s own zero value in place — the exact failure #191 named as a known pothole and #199 closed.

- Branch/PR: [`exit-demo/206-break3-stepallowance`, PR #254](https://github.com/garnizeh/cinzal/pull/254)
- Expected assertion: `TestProjectViewStepAllowanceNeverZero` (issue #199, `internal/rules/projectview_external_test.go`)
- Confirmed locally before pushing:
  ```console
  $ go test ./internal/rules/... -run TestProjectViewStepAllowanceNeverZero -v
  projectview_external_test.go:41: players=2 round=1 seat=0: ProjectView returned StepAllowance == 0
  --- FAIL: TestProjectViewStepAllowanceNeverZero (0.00s)
  ```
- CI run: [`check` job, run 32478212580](https://github.com/garnizeh/cinzal/actions/runs/32478212580/job/96758870170) — **FAILED**, as expected, identical failure line to the local run above. The `replay` jobs (which run `go test -race ./internal/rules/...`, a superset of the plain suite) caught the same regression independently — **two separately-configured CI jobs, same assertion, same failure**:
  ```console
  --- FAIL: TestProjectViewStepAllowanceNeverZero (0.00s)
      projectview_external_test.go:41: players=2 round=1 seat=0: ProjectView returned StepAllowance == 0
  ```

All three were reverted (never merged) — `main` at the SHA above carries none of them.

## Fails closed

The issue's own condition: *"the break-on-purpose runs assert the gate ran and inspected a non-empty package."* `check-bots-isolation.go`'s success message reports its inspected file count — a directory holding nothing but `doc.go` exits `VACUOUS` rather than reporting that pass on zero (`scripts/check-bots-isolation.go:145-162`). On the clean `main` SHA above:

```console
$ go run scripts/check-bots-isolation.go
check-bots-isolation: OK - 6 production file(s) in internal/bots, no reference outside scripts/bots-isolation-allowlist.txt's allow-list
```

Six production files inspected — not a VACUOUS pass on zero. The gate's *failure* path (breaks #1 and #2, above) does not repeat that count, but is non-silent on its own terms: it names every offending file and line rather than exiting with a bare non-zero status. Break #3 is a `testing.T` assertion, not a script, and Go's own test runner cannot silently skip a test that exists and is not marked `Skip` — the transcripts above show `TestProjectViewStepAllowanceNeverZero` actually ran, in two independently-configured CI jobs, and failed loudly in both.

## Result

- Operator's margin over Runner: **+0.049 mean RP**, reported and non-zero (positive half, above).
- All six §14.3 behaviours demonstrated firing, each from named `game.PlayerView` fields (table above).
- Projection-gap audit: one entry, resolved as a harness gap closed by #199, not an M1 fog defect (table above).
- All three deliberate breaks rejected by their stated gate/assertion, confirmed at CI, and reverted (temporary PRs #252, #253, #254 — none merged).
- Fails-closed condition met: the isolation gate reports its inspected file count on every run, and inspected 6 files (non-zero) on `main`.
