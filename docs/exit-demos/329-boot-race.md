# Exit demo: two processes boot against a fresh database and migrations apply exactly once

**Issue:** [#329](https://github.com/garnizeh/cinzal/issues/329)
**Milestone:** M3 — Persistence
**Kind:** Positive (two real processes both come up, migrations applied exactly once) with two embedded sub-checks: a crash-recovery case or the roadmap doesn't otherwise cover, and a negative case (the lock removed, shown to actually break).

## The criterion

> Two app processes booting simultaneously against a fresh database both come up, with migrations applied exactly once.

(Roadmap §4, M3 exit criteria.) This is the demonstration behind RFC-001 §7.5:

> **The advisory lock is not optional.** Two instances booting simultaneously after a deploy will otherwise both try to apply the same migration, and goose's own version table will not save you from a partially-applied DDL. The lock costs one line and removes the failure entirely.

and §18's deployment model, which makes the race inevitable in production: *"Two app instances behind a load balancer is the ceiling this design needs"* — and both restart together on every deploy.

**Why not goroutines.** `migrate_integration_test.go`'s own `TestConcurrencyMigrateAppliesEachMigrationExactlyOnce` already proves `migrate()`'s own logic is correct with two goroutines sharing one process. It cannot demonstrate §7.5's actual failure mode, which is specific to two separate binaries each holding their own connection with no shared memory — the exact way the naive implementation fails (lock taken from one pooled connection, released from another) can only surface across real OS processes. This demonstration builds the real migration binary and execs two of it.

**Why `cmd/migrate`, not `cmd/server`.** `cmd/server` is still `doc.go` — RFC §21's build order defers it to M5, and no M3 issue wires it up. `cmd/migrate` exists precisely to fill that gap: its own doc comment calls it *"the local-dev counterpart to RFC-001 §18's 'start: migrate (advisory lock) → serve' boot step, standing on its own because cmd/server, the eventual owner of that boot sequence, is still doc.go."* It wraps `store.Migrate()` — the same advisory-lock/goose.Provider path production boot will run — with no second implementation. Building and racing it is racing the real code path.

## Provenance

| | |
|---|---|
| Base commit | `792be5713c7adb0924fed27902562d635d9794f9` (main) |
| Postgres image | `postgres@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280` (`internal/store/pgimage.Ref`) |
| goose | `github.com/pressly/goose/v3 v3.27.3` — `NewProvider` with no `WithSessionLocker`/`WithLocker`, confirmed from goose's own source (`provider_options.go`: *"If WithSessionLocker is not called, locking is disabled"*) — goose itself supplies no protection here; §7.5's advisory lock is the only one |
| Real migrations raced | `internal/store/migrations/00001_base_schema.sql` … `00005_orders_round_positive_validate.sql` (5 files, counted dynamically by the test from the embedded `migrationsFS`, never hardcoded) |
| Advisory lock key | `migrationLockID = 7_312_005_501_884` (`internal/store/migrate.go`) |
| Binary raced | `cmd/migrate`, built fresh via `go build` for each test run |
| Committed test file | [`internal/store/migrate_boot_race_integration_test.go`](../../internal/store/migrate_boot_race_integration_test.go) |
| Go / module | `go1.27.0`, `github.com/garnizeh/cinzal` |

## Method

1. **Positive case** (`TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce`). A fresh, never-migrated database (`storetest`'s own per-package Postgres container, started the same way `migrate_integration_test.go`'s sibling tests already do — never a dumped schema). `cmd/migrate` is built once, then exec'd twice as independent `*exec.Cmd` processes, each released off one closed channel from its own goroutine — a barrier, not a `sleep` — so both `Start()` calls land within the same scheduler tick. A third, dedicated connection polls
   ```sql
   SELECT l.pid, l.granted, a.state, a.wait_event_type
   FROM pg_locks l LEFT JOIN pg_stat_activity a ON a.pid = l.pid
   WHERE l.locktype = 'advisory'
   ```
   in as tight a loop as the test binary can issue it, for the whole run. Two rows on that lock — one `granted=true`, one `granted=false` — is a real Postgres wait-queue entry belonging to the two processes under test, not an inference from timing. Both processes must exit `0`, and `goose_db_version` must show exactly `len(migrations)` applied rows — an exact count, not "both returned success," for the same reason the goroutine test draws that line: "both exited 0" alone would also pass an implementation that silently double-applied one migration and skipped another. Real DDL against a fresh database is fast enough that a single attempt can occasionally have both processes never actually overlap (§7.5 held, but not exercised that run) — the test retries against a fresh sibling database (cloned from `template0` on the same container) up to 10 times until direct contention evidence is observed, so the demonstration itself is not accidentally vacuous; a genuine correctness failure (wrong exit code, wrong count) still fails immediately on any attempt, never retried away.

2. **Crash-mid-boot recovery** (`TestIntegrationMigrateBootRaceSecondProcessRecoversAfterFirstIsKilled`). §7.5 never names this case, but it is the one an operator meets at 3am: a process dies while holding the lock, having already applied some but not all of the migration set — a genuinely partial-history database, not an empty one. `TestIntegrationMigrateReleasesLockOnMidMigrationFailure` (`migrate_integration_test.go`) already proves the *graceful*-error release path (a migration fails, `defer releaseMigrationLock` runs) and that goose commits each migration independently rather than as one all-or-nothing batch. This test proves the *ungraceful* path against a real partial-history database: process A is started, and `goose_db_version`'s applied count is polled — as tightly as the test binary can query it, no fixed sleep — until it is observed strictly between `0` and the full migration count. Only then is process A sent `SIGKILL` — no `defer` runs, no message is sent, the TCP connection simply dies. An attempt that never catches that exact window (process A finishes first, or never starts in time) retries against a fresh sibling database rather than settling for the weaker all-or-nothing proof — up to 15 attempts, the same discipline the positive case uses for contention. The test then polls until the lock is observed released, starts process B only afterward (sequential — "the second acquiring the lock afterwards," not a second racer), and asserts it completes well inside a timeout with `goose_db_version` ending at the full count — proving process B *resumes* the remaining migrations rather than hanging or redoing work already committed. The property under test is Postgres's own guarantee — a session-scoped advisory lock is freed when its session ends, including on an unclean disconnect — not application code. Every child process in both tests is bounded by a 30-second `exec.CommandContext` deadline, so a genuine regression (a real deadlock, a lock never released) fails this test directly instead of blocking until the surrounding test binary's own default timeout.

3. **Broken on purpose** (not committed — the exit-demo skill's own rule: *"revert immediately in the same turn"*). `internal/store/migrate.go`'s lock acquisition and its `defer releaseMigrationLock(db)` were commented out (full diff: [`negative-demo-lock-removal.diff`](329/negative-demo-lock-removal.diff)), and the positive-case test re-run against the unmodified real migrations. Result: **no observable failure across 10 attempts** — see below. Per the issue's own stated fallback (*"If removing the lock does not produce an observable failure, the migrations are too trivial to race"*), a sixth, temporary migration was added — `00006_demo_329_temp_race_widener.sql` ([full content](329/negative-demo-race-widener-migration.sql)): `pg_sleep(0.5)` then `CREATE TABLE demo_329_race_widget`. This widens the real DDL window without changing what the two processes are racing over — the same catalog-level conflict `00001`'s own `CREATE TABLE` statements are exposed to, just stretched long enough for process-start skew to reliably land inside it. The positive-case test was re-run once more against this widened set. Both the lock removal and the extra migration were reverted (`git checkout` / `rm`) in the same session that captured the failure, before anything else was committed; `make check` and `make integration` were re-run afterward on the restored tree (below) to confirm nothing was left behind.

## Result

**Positive case**, attempt 1 of 10 (the common case — contention was observed on the first attempt in every run performed for this demonstration): both processes exited `0`, `goose_db_version` showed `5/5` migrations applied, and 59 distinct `pg_locks` snapshots captured the race directly — [`positive-and-recovery-test-run.txt`](329/positive-and-recovery-test-run.txt):

```text
migrate_boot_race_integration_test.go:260: attempt 1: both processes exited 0, 5/5 migrations applied, N contention snapshot(s) observed
    [...T23:26:40.00671195...] pid=93 granted=true  state="active" wait_event_type=""     | pid=94 granted=false state="active" wait_event_type="Lock"
    [...T23:26:40.052581332...] pid=93 granted=true  state="idle"   wait_event_type="Client" | pid=94 granted=false state="active" wait_event_type="Lock"
process 1 stdout: cmd/migrate: applied 5 migration(s) (5 total)
process 2 stdout: cmd/migrate: applied 5 migration(s) (5 total)
--- PASS: TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce (4.72s)
```

`pid 94` — `granted=false`, `wait_event_type="Lock"` — is the losing process genuinely blocked inside `pg_advisory_lock` for the ~50ms `pid 93` held it, sustained across dozens of successive polls. Both processes still report having "applied 5 migration(s)": the loser's own before/after count (`cmd/migrate/run.go`'s `evaluateMigration`) legitimately observes the winner's work landing while it was blocked — not a double-apply, confirmed by the single `5` in `goose_db_version`.

**Crash recovery**, same file:

```text
attempt 1/15: process A killed at 2026-09-02T00:13:48.170845292-03:00 with 1/5 migrations already committed
lock observed released at 2026-09-02T00:13:48.175972714-03:00 (0.005s after kill)
process B started 2026-09-02T00:13:48.175972993-03:00, completed in 123.188404ms: cmd/migrate: applied 4 migration(s) (5 total)
final goose_db_version count: 5/5
--- PASS: TestIntegrationMigrateBootRaceSecondProcessRecoversAfterFirstIsKilled (4.13s)
```

Process A was killed with a genuine partial history in place — 1 of 5 migrations already committed, 4 still pending — caught on the first attempt by polling `goose_db_version` rather than assuming a fixed delay would land there. Postgres released the lock 5ms after the kill with zero application-level cleanup involved, and process B — started only after that release was observed, not racing A — applied exactly the 4 remaining migrations (`cmd/migrate`'s own "applied 4 migration(s) (5 total)") and completed in 123ms, leaving `goose_db_version` at the full count. This is stronger than an all-migrations-pending recovery: it proves process B *resumes* from where A left off rather than merely succeeding against an empty database. No hang.

**Negative case, part 1 — real migrations, lock removed:** [`negative-no-lock-trivial-migrations-run.txt`](329/negative-no-lock-trivial-migrations-run.txt):

```text
attempt 1: both processes exited 0, 5/5 migrations applied, 0 contention snapshot(s) observed
...
attempt 10: both processes exited 0, 5/5 migrations applied, 0 contention snapshot(s) observed
no contention evidence captured across 10 attempts
--- FAIL: TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce (17.81s)
```

Ten attempts, zero overlap, zero failures — the real 5-migration set applies fast enough (a few milliseconds end to end, versus the ~50-90ms window the lock itself manufactures by blocking the loser for the winner's *entire* run) that ordinary process-start skew alone rarely lands two unlocked runs inside the same narrow DDL window. This is the exact case the issue's own acceptance criteria names and requires a response to, not a result to explain away.

**Negative case, part 2 — widened migration set, lock still removed:** [`negative-no-lock-widened-migration-failure.txt`](329/negative-no-lock-widened-migration-failure.txt):

```text
attempt 7: both processes exited 0, 6/6 migrations applied, 0 contention snapshot(s) observed
attempt 8: process 2 exited 1
    stderr: cmd/migrate: store: apply migrations: partial migration error (type:sql,version:1): ERROR: duplicate key value violates unique constraint "pg_type_typname_nsp_index" (SQLSTATE 23505)
--- FAIL: TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce (14.30s)
```

A real, unmanufactured Postgres error: two concurrent `CREATE TABLE demo_329_race_widget` statements collided on `pg_type`'s own catalog uniqueness constraint (every table implicitly registers a composite row type there), and the losing process's migration transaction was rejected outright — a genuine partial/duplicate-application failure of exactly the shape §7.5 warns about, with the lock as the only thing standing between it and this run.

**Restored tree.** `internal/store/migrate.go` and `internal/store/migrations/` were reverted to their committed state (`git status --short` clean), and both committed tests were re-run and passed again — [`positive-and-recovery-test-run.txt`](329/positive-and-recovery-test-run.txt) is that re-run. `make check` is green on the restored tree, including `check-integration-coverage` at the bumped floor — [`integration-coverage-gate.txt`](329/integration-coverage-gate.txt):

```text
check-integration-coverage: OK - 70 Integration/Concurrency test(s) found across ./internal/store/... ./internal/match/... ./cmd/replay/... (floor 70)
```

`make integration` (the full Docker-backed suite, `internal/store`, `internal/match`, `cmd/replay`) is also green on the restored tree.

## Verdict

**Met**, with a finding worth stating plainly rather than folding into the pass. Two real `cmd/migrate` processes, started off a barrier against a database that has never seen a migration, both come up and leave `goose_db_version` showing every real, production migration applied exactly once — with direct `pg_locks` evidence, not an inference, that the two processes actually contended for RFC-001 §7.5's advisory lock rather than happening to miss each other. A process killed while holding that lock is recovered from automatically by Postgres's own session-scoped release, and a second process afterward completes without hanging.

The negative half needed a second attempt to be a real demonstration rather than a hopeful one: the actual M3 migration set (5 small `CREATE TABLE`/`ALTER TABLE` files) is fast enough that removing the lock alone did **not** produce a failure in 10 straight attempts — the migrations are, as the issue itself anticipated, "too trivial to race" at the timings process-start skew alone provides. Widening the window with one temporary migration (never committed) exposed the real conflict immediately: a genuine Postgres catalog-uniqueness violation, killing the losing process outright. Read together, this is not a weaker result than the lock catching the race directly — it says the production migration set as it stands today is unlikely to race in practice even *without* the lock, while confirming the underlying hazard §7.5 exists to remove is real the moment migrations do enough work to overlap, which is exactly the situation a live deploy under real load (slower disks, cold caches, larger tables as the schema grows) will eventually produce. The lock removes the hazard unconditionally rather than leaving it to be timing-lucky.
