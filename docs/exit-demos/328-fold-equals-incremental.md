# Exit demo: a folded match equals the incrementally computed one, over a golden fixture

**Issue:** [#328](https://github.com/garnizeh/cinzal/issues/328)
**Milestone:** M3 — Persistence
**Kind:** Positive — the fold and the incremental path must produce byte-identical state, over a real database round trip, with a deliberate corruption shown to break the comparison.

## The criterion

> A match folded from the log equals the incrementally computed state, asserted over a golden fixture.

(Roadmap §4, M3 exit criteria.) This is the demonstration behind RFC §7.1's own claim:

> **The order log is the truth. Match state is derived and never stored.**
> `state = fold(Resolve, initial(seed, cfg), orderLog)`

and the reconstruction §18 stakes everything on:

> The order log is irreplaceable — lose it and matches cannot be reconstructed, because there is no state table to fall back on.

Two code paths compute `MatchState` in this codebase — driving rounds forward incrementally (`rules.NewMatch` + `rules.Resolve` per round, what `cmd/simulate` and M4's tick do) and folding a persisted order log (`internal/match/fold.Fold`, what `cmd/replay` and any future rebuild do). This demonstration proves they agree, with the fold's input read back from a real Postgres database rather than reused in memory.

## Provenance

| | |
|---|---|
| Base commit | `ca29c7ed987a3d3e33bf86be203d01f546879565` (main, at the time this demonstration was built) — the PR that lands this file also lands the test/fixture/gate changes it describes, so the commit that actually produced the captures below is that PR's own head, not this base. |
| Postgres image | `postgres@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280` (`internal/store/pgimage.Ref`, the same pinned digest every `storetest`-backed test in this repository uses) |
| Config | `game.DefaultConfig()`, unmodified |
| Bot tier | Operator (`bots.For(bots.Operator)`, default `OperatorOptions`) — chosen over Drifter for wider rule-surface coverage (contracts, staking, confrontations) closer to "a real match" |
| Seeds / player counts | `[32]byte{41}` at 2 players, `[32]byte{42}` at 4 players |
| Rounds | 15 (`cfg.Rounds`, the full match) |
| Test file | [`cmd/replay/fold_equivalence_integration_test.go`](../../cmd/replay/fold_equivalence_integration_test.go) |
| Golden fixtures | [`cmd/replay/testdata/fold-equivalence-2p.json`](../../cmd/replay/testdata/fold-equivalence-2p.json), [`cmd/replay/testdata/fold-equivalence-4p.json`](../../cmd/replay/testdata/fold-equivalence-4p.json) — committed, regenerated only via `go test -tags=integration ./cmd/replay/... -run TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture -update` |

Each golden fixture is the literal `{Seed, Config, Players, OrderLog, Final}` record the issue's own acceptance criterion asks for, not just the final state — a CodeRabbit review on the PR that landed this file (below) correctly flagged an earlier draft that committed only `Final`, regenerating `OrderLog` from `bots.Operator`'s current decision logic on every run instead of replaying a frozen one. The bot now runs exactly once, under `-update`, to produce the committed log; every ordinary test run decodes that log and replays it, so a future change to Operator's own heuristics can never silently redefine which historical match this file represents.

## Method

1. **Generate the fixture (once, under `-update`).** `generateFoldFixture` drives a full 15-round match: `rules.NewMatch(seed, cfg, players)`, then per round, every seat's order via `rules.ProjectView` + `bots.For(bots.Operator).Decide` + `rules.NewBotRNG(seed, seat, round)`, then `rules.Resolve`. The resulting `{seed, config, players, orderLog, final}` is written to `testdata/fold-equivalence-{2,4}p.json` and committed.
2. **Decode and self-check.** Every ordinary run reads the committed fixture — no bot involved — and replays its `OrderLog` through `runIncremental` (`rules.NewMatch` + `rules.Resolve` per round, the same shape `cmd/simulate/driver.go`'s `RunMatch` uses, applied to a log that already exists rather than one decided live). The result must equal the fixture's own committed `Final`, byte-for-byte — this is what would catch a future rule change moving the result, against a log that is now genuinely fixed data rather than something that regenerates itself under a changed bot.
3. **Persist through the real write path.** `Store.CreateMatch` + one `Store.AppendOrder` call per `(round, seat)` from the fixture's own `OrderLog`, against a real Postgres transaction (`storetest.Container`, D46 tier 1) — the same write path a live tick would use, `store.SourceBot` since the log is bot-authored.
4. **Reload through a real SELECT.** `Store.LoadMatch` + `orderlog.Load` — never the in-memory log step 3 wrote — then `internal/match/fold.Fold(seed, cfg, players, reloadedLog)`.
5. **Compare.** The folded state's JSON must equal the incremental state's JSON, byte-for-byte, including a named sub-check over every seat's `Players[i].Archive` (`game.SeatArchive`) specifically — the issue's own "where a divergence would be least visible" concern.
6. **Break it on purpose.** A second test (`TestIntegrationFoldDivergesWhenOrderCorrupted`) repeats steps 1 and 3 at 2 players, then issues one raw `UPDATE orders SET payload = $1 WHERE match_id = $2 AND round = $3 AND seat = $4` against a single `(round, seat)` row whose order carries a non-empty `Route` — bypassing `Store.AppendOrder` entirely — truncating that route by one node. Reload and fold as in step 4, and assert the folded state now **disagrees** with the (uncorrupted) incremental state, after first asserting both states are genuine 15-round completions (not two vacuous states that could trivially "differ on nothing").
7. **Coverage floor.** Both new tests follow D46's `TestIntegrationXxx` naming convention; `scripts/check-integration-coverage.sh`'s `DEFAULT_FLOOR` is bumped 66 → 68 in the same change, per its own "bumped upward only" rule.

A one-field database-level mutation was chosen for step 6 (a `Route` truncation) rather than a wholesale payload replacement, so the corruption is the smallest change that could plausibly slip past a weaker check — exactly the shape the issue asks for ("perturb one order's payload... by a single field").

## Result

Both new tests pass against a real, freshly-started Postgres container (command: `go test -tags=integration ./cmd/replay/... -run 'TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture|TestIntegrationFoldDivergesWhenOrderCorrupted' -count=1 -v`), captured verbatim in [`fold-equivalence-test-run.txt`](328/fold-equivalence-test-run.txt):

```text
=== RUN   TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture
=== RUN   TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture/2p
=== RUN   TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture/4p
--- PASS: TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture (15.86s)
    --- PASS: TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture/2p (15.63s)
    --- PASS: TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture/4p (0.23s)
=== RUN   TestIntegrationFoldDivergesWhenOrderCorrupted
--- PASS: TestIntegrationFoldDivergesWhenOrderCorrupted (0.07s)
PASS
ok  	github.com/garnizeh/cinzal/cmd/replay	15.995s
```

(Docker container bootstrap log lines between the two `2p`/`4p` sub-test starts are elided above; the full output, unedited, is in the linked file.)

`TestIntegrationFoldDivergesWhenOrderCorrupted` passing means the corruption **was** caught — a passing test here is the positive proof that the comparison is not vacuous, the same reading #206 gave a passing "gate rejected it" result.

The golden fixtures themselves record a genuine 15-round completion at both player counts — `Final.Round: 15`, `OrderLog` holding exactly 15 rounds, and the expected `Players` count — confirmed when they were generated (`-update` run, no database needed for that half, since generation and the self-check both run entirely in memory).

The coverage floor bump is confirmed by [`integration-coverage-list.txt`](328/integration-coverage-list.txt) (68 `TestIntegration*`/`TestConcurrency*` names across `internal/store`, `internal/store/orderlog`, `internal/match/fold`, `cmd/replay` — including the two new ones, lines 61-62) and [`integration-coverage-gate.txt`](328/integration-coverage-gate.txt):

```text
check-integration-coverage: OK - 68 Integration/Concurrency test(s) found across ./internal/store/... ./internal/match/... ./cmd/replay/... (floor 68)
```

`make check` (the full local gate suite) is green on the same tree, including `check-replay-deps` (`OK - 7 internal package(s), all within .../replay-deps-allowlist.txt` — unchanged from before this PR, confirming the new file's test-only `internal/bots` import never reaches `cmd/replay`'s production dependency graph) and `check-bots-isolation` (unaffected — that gate checks `internal/bots`'s own outbound imports, not who imports it).

## Verdict

**Met.** The fold and the incremental path produce byte-identical `MatchState` — including every seat's `SeatArchive` — over a full 15-round match at two player counts, with the fold's input read back through a genuine `SELECT` against a real Postgres database rather than reused from memory, and a single-field database-level corruption is shown to break the comparison rather than pass through it unnoticed. RFC §7.1's `state = fold(Resolve, initial(seed, cfg), orderLog)` holds as demonstrated; the no-snapshot bet §7.3 and §18 make remains falsifiable and, as of this measurement, unfalsified.

One thing this demonstration does **not** cover, named rather than left implicit: it exercises `internal/match/fold.Fold` directly (via `cmd/replay`'s own dependency on it), not a live M4 tick — M4 does not exist yet. The equivalence proven here is between `cmd/simulate`'s incremental shape and `cmd/replay`'s folded shape; M4's own tick will need to be shown to produce the same incremental state its own way, when it lands.
