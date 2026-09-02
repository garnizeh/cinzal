//go:build integration

package store

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// This file is issue #329's exit demonstration in committed form: RFC-001
// §7.5's advisory lock, proven across two real OS processes rather than two
// goroutines in one. migrate_integration_test.go's own
// TestConcurrencyMigrateAppliesEachMigrationExactlyOnce is the right unit
// test for migrate()'s own logic; it cannot demonstrate §7.5's actual
// failure mode, which is specific to two separate binaries each holding
// their own connection — a goroutine test can accidentally reuse a
// connection across two calls and pass regardless. These tests instead
// build cmd/migrate (RFC-001 §18's "migrate (advisory lock)" boot step,
// standing in for cmd/server per cmd/migrate/doc.go, since cmd/server
// itself is still doc.go until M5) and exec two real processes of it
// against one freshly-created, never-migrated database.
//
// The negative half of #329's acceptance criteria — the same race with the
// advisory lock removed, shown to fail — is not committed here: it requires
// editing migrate.go's lock acquisition itself, which the exit-demo skill's
// own rule ("break it, capture the failure, revert in the same turn") says
// must never be left in the tree. See docs/exit-demos/329-boot-race.md for
// that capture.

// buildMigrateBinary compiles cmd/migrate into a throwaway binary for this
// test binary's own temp directory, by import path rather than a relative
// path, so it resolves the same way regardless of the package this file's
// test binary happens to run from.
func buildMigrateBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cmd-migrate-race")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/garnizeh/cinzal/cmd/migrate")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/migrate: %v\n%s", err, out)
	}
	return bin
}

// countProductionMigrations counts the real, embedded migrations Migrate()
// applies in production — read from migrationsFS itself (migrate.go) rather
// than hardcoded, so this test can never silently fall out of sync with
// internal/store/migrations gaining or losing a file.
func countProductionMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read production migrations directory: %v", err)
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue // README.md and any non-migration file goose itself ignores
		}
		if name[0] < '0' || name[0] > '9' {
			continue
		}
		n++
	}
	if n == 0 {
		t.Fatal("counted zero production migrations — internal/store/migrations looks empty or misnamed")
	}
	return n
}

// createFreshUnmigratedDatabase creates one more never-migrated database on
// the same running container adminDSN already connects to, cloned from
// Postgres's own template0 exactly like storetest.FreshUnmigratedDatabase
// does for the same purpose — that helper cannot be reused directly here
// (package store cannot import storetest without an import cycle; see
// migrate_integration_test.go's own header), so this is that helper's
// package-store-local equivalent, scoped to this file alone.
func createFreshUnmigratedDatabase(t *testing.T, adminDSN, name string) string {
	t.Helper()
	admin := openDedicated(t, adminDSN)
	if _, err := admin.ExecContext(context.Background(),
		fmt.Sprintf("CREATE DATABASE %s TEMPLATE template0", name)); err != nil {
		t.Fatalf("create fresh unmigrated database %s: %v", name, err)
	}
	return replaceDatabaseName(t, adminDSN, name)
}

// replaceDatabaseName swaps the path component of a testcontainers-issued
// DSN (host:port/dbname?params) for a different database name on the same
// server, so a second/third attempt can target a sibling database without
// spinning up a second container.
func replaceDatabaseName(t *testing.T, dsn, newName string) string {
	t.Helper()
	slash := strings.LastIndex(dsn, "/")
	q := strings.IndexByte(dsn, '?')
	if slash < 0 || q < slash {
		t.Fatalf("dsn %q not in the expected host/dbname?params shape", dsn)
	}
	return dsn[:slash+1] + newName + dsn[q:]
}

// procResult is one exec'd cmd/migrate process's outcome.
type procResult struct {
	stdout, stderr string
	exitCode       int
}

// processTimeout bounds every migration child process this file execs.
// Without it, a genuine regression — a migration that deadlocks, or an
// advisory lock that is acquired but never released — would block
// cmd.Wait/cmd.Run until the surrounding go test binary's own default
// 10-minute timeout, which kills the whole binary with a stack dump rather
// than failing this one test with a readable message. 30s is generous
// against the ~100-200ms an unblocked run actually takes.
const processTimeout = 30 * time.Second

// runTwoProcessesBarrier execs bin --db dsn as two independent OS
// processes, released together off one closed channel (a barrier, not a
// sleep — issue #329's own requirement) so they start close enough together
// to actually contend for RFC-001 §7.5's advisory lock, and concurrently
// polls pg_locks/pg_stat_activity as fast as this test binary can query
// them for direct proof of contention: a locktype='advisory' row with
// granted=false coexisting with another backend's granted=true row on the
// same lock is a real Postgres wait-queue entry, not an inference. It
// returns every distinct snapshot observed in that shape.
func runTwoProcessesBarrier(t *testing.T, bin, dsn string) (procResult, procResult, []string) {
	t.Helper()

	observer := openDedicated(t, dsn)
	stopObserving := make(chan struct{})
	observingDone := make(chan struct{})
	var evidence []string

	go func() {
		defer close(observingDone)
		for {
			select {
			case <-stopObserving:
				return
			default:
			}
			rows, err := observer.QueryContext(context.Background(),
				`SELECT l.pid, l.granted, coalesce(a.state, ''), coalesce(a.wait_event_type, '')
				 FROM pg_locks l LEFT JOIN pg_stat_activity a ON a.pid = l.pid
				 WHERE l.locktype = 'advisory'`)
			if err != nil {
				return // observer connection torn down alongside the database
			}
			var snap []string
			for rows.Next() {
				var pid int
				var granted bool
				var state, wait string
				if scanErr := rows.Scan(&pid, &granted, &state, &wait); scanErr == nil {
					snap = append(snap, fmt.Sprintf("pid=%d granted=%v state=%q wait_event_type=%q", pid, granted, state, wait))
				}
			}
			_ = rows.Close()
			if len(snap) >= 2 {
				evidence = append(evidence, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339Nano), strings.Join(snap, " | ")))
			}
		}
	}()

	ctx1, cancel1 := context.WithTimeout(context.Background(), processTimeout)
	defer cancel1()
	ctx2, cancel2 := context.WithTimeout(context.Background(), processTimeout)
	defer cancel2()
	cmd1 := exec.CommandContext(ctx1, bin, "--db", dsn)
	cmd2 := exec.CommandContext(ctx2, bin, "--db", dsn)
	var out1, err1, out2, err2 bytes.Buffer
	cmd1.Stdout, cmd1.Stderr = &out1, &err1
	cmd2.Stdout, cmd2.Stderr = &out2, &err2

	barrier := make(chan struct{})
	var starters sync.WaitGroup
	var startErr1, startErr2 error
	starters.Add(2)
	go func() { defer starters.Done(); <-barrier; startErr1 = cmd1.Start() }()
	go func() { defer starters.Done(); <-barrier; startErr2 = cmd2.Start() }()
	close(barrier)
	starters.Wait()
	if startErr1 != nil {
		t.Fatalf("start process 1: %v", startErr1)
	}
	if startErr2 != nil {
		t.Fatalf("start process 2: %v", startErr2)
	}

	waitErr1 := cmd1.Wait()
	waitErr2 := cmd2.Wait()
	close(stopObserving)
	<-observingDone

	r1 := procResult{stdout: out1.String(), stderr: err1.String(), exitCode: exitCodeOf(cmd1, waitErr1)}
	r2 := procResult{stdout: out2.String(), stderr: err2.String(), exitCode: exitCodeOf(cmd2, waitErr2)}
	return r1, r2, evidence
}

// exitedBySignal reports whether cmd (already Wait()ed) was terminated by a
// signal rather than exiting on its own — the only authoritative proof that
// a SIGKILL actually ended the process, since Process.Kill() returning nil
// only means the syscall was accepted, not that it beat the process to the
// finish line.
func exitedBySignal(cmd *exec.Cmd) bool {
	if cmd.ProcessState == nil {
		return false
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled()
}

// exitCodeOf reads cmd's real exit code from ProcessState once Wait has
// returned; a non-nil waitErr with no ProcessState means the process never
// started cleanly, reported as -1 rather than a false 0.
func exitCodeOf(cmd *exec.Cmd, waitErr error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if waitErr != nil {
		return -1
	}
	return 0
}

// TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce is #329's
// positive demonstration: two real cmd/migrate processes, started off one
// barrier against a database that has never seen a migration, both come up
// (exit 0) and RFC-001 §7.5's advisory lock leaves goose_db_version showing
// every real, production migration applied exactly once — never double,
// never short. The assertion is exact-count, not "both returned success",
// for the same reason TestConcurrencyMigrateAppliesEachMigrationExactlyOnce
// already draws that line: "both exited 0" alone passes just as well
// against a migrations directory that silently applied twice.
//
// Real DDL against a fresh database is fast enough that a single attempt
// can occasionally have both processes never actually overlap on the lock
// (§7.5 held, but not exercised) — this retries with a fresh database each
// time until direct contention evidence (a queued pg_locks row) is
// observed, capping at maxContentionAttempts so a genuine regression still
// fails in bounded time rather than spinning forever.
func TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce(t *testing.T) {
	baseDSN := startPostgres(t) // "cinzal_test" — fresh, unmigrated
	bin := buildMigrateBinary(t)
	want := countProductionMigrations(t)

	const maxContentionAttempts = 10
	var (
		evidence       []string
		attemptUsed    int
		lastR1, lastR2 procResult
	)

	for attempt := 1; attempt <= maxContentionAttempts; attempt++ {
		dsn := baseDSN
		if attempt > 1 {
			dsn = createFreshUnmigratedDatabase(t, baseDSN, fmt.Sprintf("cinzal_race_%d", attempt))
		}

		r1, r2, ev := runTwoProcessesBarrier(t, bin, dsn)
		lastR1, lastR2 = r1, r2

		if r1.exitCode != 0 {
			t.Fatalf("attempt %d: process 1 exited %d\nstdout: %s\nstderr: %s", attempt, r1.exitCode, r1.stdout, r1.stderr)
		}
		if r2.exitCode != 0 {
			t.Fatalf("attempt %d: process 2 exited %d\nstdout: %s\nstderr: %s", attempt, r2.exitCode, r2.stdout, r2.stderr)
		}

		db := openDedicated(t, dsn)
		var applied int
		if err := db.QueryRowContext(context.Background(),
			"SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&applied); err != nil {
			t.Fatalf("attempt %d: query goose_db_version: %v", attempt, err)
		}
		if applied != want {
			t.Fatalf("attempt %d: goose_db_version shows %d applied migrations, want exactly %d — "+
				"either the two processes double-applied one, or one came up short", attempt, applied, want)
		}

		t.Logf("attempt %d: both processes exited 0, %d/%d migrations applied, %d contention snapshot(s) observed",
			attempt, applied, want, len(ev))

		if len(ev) > 0 {
			evidence = ev
			attemptUsed = attempt
			break
		}
	}

	if len(evidence) == 0 {
		t.Fatalf("no contention evidence captured across %d attempts — the two processes never observably "+
			"overlapped on the advisory lock, so this run does not demonstrate the race", maxContentionAttempts)
	}

	t.Logf("contention captured on attempt %d/%d:\n%s", attemptUsed, maxContentionAttempts, strings.Join(evidence, "\n"))
	t.Logf("process 1 stdout: %s", lastR1.stdout)
	t.Logf("process 2 stdout: %s", lastR2.stdout)
}

// TestIntegrationMigrateBootRaceSecondProcessRecoversAfterFirstIsKilled is
// #329's third case: a process crashing while it holds RFC-001 §7.5's
// advisory lock — with at least one migration already committed and at
// least one still pending, the genuine partial-history state an operator
// meets — and a second process afterward resuming to completion rather than
// hanging or redoing already-applied work. §7.5 itself never names this
// case, but it is what an operator actually meets: a deploy killed
// mid-rollout, an OOM, a node eviction. The property under test is
// Postgres's own guarantee, not application code: a session-scoped advisory
// lock is released automatically when its session ends, including an
// unclean SIGKILL that runs no defer and sends no message.
// TestIntegrationMigrateReleasesLockOnMidMigrationFailure
// (migrate_integration_test.go) already proves the graceful-error release
// path, and that goose commits each migration independently rather than as
// one all-or-nothing batch, at the unit level; this proves the
// ungraceful-death path against a real, partially-applied database, with a
// real second OS process rather than a second call in the same binary.
//
// Catching process A mid-batch needs deterministic synchronization, not a
// fixed sleep: it is killed the instant goose_db_version's applied count is
// observed strictly between 0 and the full migration count, polled as
// tightly as this test binary can query it. An attempt that never observes
// that window — process A finishes before it's caught, or never starts in
// time — retries against a fresh sibling database rather than accepting a
// weaker (all-or-nothing) proof of recovery.
func TestIntegrationMigrateBootRaceSecondProcessRecoversAfterFirstIsKilled(t *testing.T) {
	baseDSN := startPostgres(t)
	bin := buildMigrateBinary(t)
	want := countProductionMigrations(t)

	const maxPartialAttempts = 15
	var (
		partialCaptured bool
		partialApplied  int
		killedAt        time.Time
		releasedAt      time.Time
		startB          time.Time
		elapsedB        time.Duration
		outBFinal       string
		attemptUsed     int
	)

	for attempt := 1; attempt <= maxPartialAttempts; attempt++ {
		dsn := baseDSN
		if attempt > 1 {
			dsn = createFreshUnmigratedDatabase(t, baseDSN, fmt.Sprintf("cinzal_recover_%d", attempt))
		}
		observer := openDedicated(t, dsn)

		ctxA, cancelA := context.WithTimeout(context.Background(), processTimeout)
		cmdA := exec.CommandContext(ctxA, bin, "--db", dsn)
		var outA, errA bytes.Buffer
		cmdA.Stdout, cmdA.Stderr = &outA, &errA
		if err := cmdA.Start(); err != nil {
			cancelA()
			t.Fatalf("attempt %d: start process A: %v", attempt, err)
		}

		partialDeadline := time.Now().Add(5 * time.Second)
		var applied int
		gotPartial := false
		for time.Now().Before(partialDeadline) {
			if err := observer.QueryRowContext(context.Background(),
				"SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&applied); err == nil {
				if applied > 0 && applied < want {
					gotPartial = true
					break
				}
				if applied >= want {
					break // process A finished before this attempt caught it mid-flight
				}
			}
			time.Sleep(time.Millisecond)
		}

		if !gotPartial {
			_ = cmdA.Process.Kill()
			_ = cmdA.Wait()
			cancelA()
			t.Logf("attempt %d: never observed a genuine partial-application window (last seen %d/%d applied) — retrying", attempt, applied, want)
			continue
		}

		killedAt = time.Now()
		killErr := cmdA.Process.Kill() // SIGKILL: no defer, no graceful unlock — the crash this test simulates
		waitErr := cmdA.Wait()
		cancelA()
		if killErr != nil {
			// The partial window can close between the count observation above and
			// this Kill call — process A finished the remaining migrations on its
			// own in that gap. This attempt can no longer prove anything about a
			// killed-mid-batch process, so it retries rather than failing the test
			// over a race it lost, the same as the !gotPartial branch above.
			t.Logf("attempt %d: process A had already exited before the kill (%v) — the partial window closed first, retrying", attempt, killErr)
			continue
		}
		if !exitedBySignal(cmdA) {
			// Kill() succeeding only proves the SIGKILL syscall was accepted, not
			// that it was what actually ended the process — the same race as
			// above, just resolved a step later: process A could have exited
			// normally in the gap between Kill() being sent and Wait() returning.
			// Only a confirmed signal death lets what follows be trusted as a
			// genuine killed-mid-batch recovery rather than an ordinary
			// already-finished run.
			t.Logf("attempt %d: process A exited on its own (%v) rather than by our signal — the partial window closed first, retrying", attempt, waitErr)
			continue
		}

		releaseDeadline := time.Now().Add(10 * time.Second)
		released := false
		for time.Now().Before(releaseDeadline) {
			var n int
			if err := observer.QueryRowContext(context.Background(),
				"SELECT count(*) FROM pg_locks WHERE locktype = 'advisory'").Scan(&n); err == nil && n == 0 {
				released = true
				releasedAt = time.Now()
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !released {
			t.Fatalf("attempt %d: advisory lock still held 10s after the holding process was killed — "+
				"Postgres should release a session-scoped lock when its session ends", attempt)
		}

		startB = time.Now()
		ctxB, cancelB := context.WithTimeout(context.Background(), processTimeout)
		cmdB := exec.CommandContext(ctxB, bin, "--db", dsn)
		var outB, errB bytes.Buffer
		cmdB.Stdout, cmdB.Stderr = &outB, &errB
		runErrB := cmdB.Run()
		elapsedB = time.Since(startB)
		cancelB()

		if runErrB != nil {
			t.Fatalf("attempt %d: process B failed: %v\nstdout: %s\nstderr: %s", attempt, runErrB, outB.String(), errB.String())
		}
		if elapsedB > 5*time.Second {
			t.Fatalf("attempt %d: process B took %s to complete — looks like it blocked rather than acquiring the freed lock", attempt, elapsedB)
		}

		db := openDedicated(t, dsn)
		var finalApplied int
		if err := db.QueryRowContext(context.Background(),
			"SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&finalApplied); err != nil {
			t.Fatalf("attempt %d: query goose_db_version: %v", attempt, err)
		}
		if finalApplied != want {
			t.Fatalf("attempt %d: goose_db_version shows %d applied migrations after recovery, want exactly %d", attempt, finalApplied, want)
		}

		partialCaptured = true
		partialApplied = applied
		outBFinal = outB.String()
		attemptUsed = attempt
		break
	}

	if !partialCaptured {
		t.Fatalf("never observed process A holding the lock with a genuine partial application in progress "+
			"(0 < applied < %d) across %d attempts", want, maxPartialAttempts)
	}

	t.Logf("attempt %d/%d: process A killed at %s with %d/%d migrations already committed",
		attemptUsed, maxPartialAttempts, killedAt.Format(time.RFC3339Nano), partialApplied, want)
	t.Logf("lock observed released at %s (%.3fs after kill)", releasedAt.Format(time.RFC3339Nano), releasedAt.Sub(killedAt).Seconds())
	t.Logf("process B started %s, completed in %s: %s", startB.Format(time.RFC3339Nano), elapsedB, strings.TrimSpace(outBFinal))
	t.Logf("final goose_db_version count: %d/%d", want, want)
}
