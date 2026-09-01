//go:build amd64

// TestResolveAllocationBudget's constants (opsmetrics.BytesPerInitialCall,
// opsmetrics.BytesPerResolveCall) were measured on one machine, once. D45's
// own reasoning ("allocation count and bytes are deterministic given
// deterministic code") holds for one build, but not across the axes this
// repository's own CI already spans (issue #352):
//
//   - GOARCH. Go's compiler backend differs per architecture — inlining
//     budget, register allocation, and therefore escape-analysis outcomes can
//     all move a heap byte count without Resolve's own code changing at all.
//     CI's replay job runs the whole internal/rules package on both
//     ubuntu-latest (amd64) and macos-latest — arm64, per that job's own
//     comment — and both legs are required merge-blocking checks. Pinning
//     this file to amd64 keeps the self-check honest on the platform it was
//     actually measured on, rather than asserting cross-architecture equality
//     D45 never claimed and this repository cannot verify (no arm64 CI
//     runner records these bytes today).
//   - Go toolchain version. GOTOOLCHAIN=local plus go.mod's `go` directive
//     pins the compiler CI actually uses, but a future version bump is
//     exactly the kind of change that can shift escape analysis without
//     touching internal/rules — the same hazard as GOARCH, on a different
//     axis.
//
// The exclusion is a build tag, not a t.Skip, the same shape
// bots_operator_golden_external_test.go already uses in this package (there,
// to keep an expensive cohort comparison out of -race's 10-minute budget):
// on any GOARCH other than amd64, this file — and therefore
// TestResolveAllocationBudget — is not part of the compiled test binary at
// all, so there is no code path here to mistake for "ran and passed" on a
// platform it never measured.
//
// -race: this file runs under -race wherever it compiles, by construction —
// make test, make check (via test) and make integration (once it exists,
// D46) all invoke `go test -race`, and none of them exclude internal/rules.
// That is intentional, not merely tolerated: -race changes allocation
// behaviour, but empirically not enough to threaten the 10% band on amd64.
// Measured directly against these same constants (linux/amd64, go1.27.0,
// resolveBenchConfig(), the same fixed match measureResolveAllocationBudget
// below uses):
//
//	no -race: initBytes 0.09% under BytesPerInitialCall, resolveBytes exact
//	-race:    initBytes 0.84% over BytesPerInitialCall, resolveBytes 2.01% over BytesPerResolveCall
//
// Both comfortably inside the 10% band (assertWithinTenPercent's own
// justification below), so -race stays in scope rather than gaining its own
// exclusion — a second build tag here would hide a real regression on the
// one axis (-race) that this measurement shows is not actually a problem.
//
// Parallelism: this test must never gain a t.Parallel() call.
// debug.SetGCPercent(-1) is process-global — the deferred restore below
// undoes it only after this function returns, not for the duration of any
// other test's own allocations. What keeps that safe today is that this
// test is not marked parallel: Go's testing package runs every non-parallel
// top-level test in a binary to completion, in order, before resuming any
// test that called t.Parallel() (contracts_test.go and property_test.go are
// the two files in this package that do, as of this writing) — so this
// test's GC-disabled window can never overlap a parallel test's own
// allocations. That guarantee is a property of this test staying serial; it
// would not survive this test, or a new sibling in this file, adding
// t.Parallel().
package rules_test

import (
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
	"github.com/garnizeh/cinzal/internal/rules"
)

// measureResolveAllocationBudget is the deterministic, GC-disabled
// measurement D45 specifies: debug.SetGCPercent(-1) for the duration,
// runtime.MemStats.TotalAlloc read before/after each call, restored on
// return. Allocation count and bytes are deterministic given deterministic
// code on one build — see this file's own header comment for the axes that
// is not deterministic across.
//
// Measures one fixed match: players=4, seed=[32]byte{1},
// resolveBenchConfig() — the same (seed, cfg, players) tuple
// opsmetrics.BytesPerInitialCall/BytesPerResolveCall's own doc comment
// names as their source. initBytes is NewMatch's own allocation; resolveBytes
// is the average per-Resolve-call allocation across all cfg.Rounds rounds of
// a real, complete match (every seat idle every round) — not one isolated
// call, so the average absorbs whatever round-to-round variance idling
// through the whole match produces (event cards from round 4 on, deadline
// checks, etc.).
func measureResolveAllocationBudget(tb testing.TB) (initBytes, resolveBytes uint64) {
	tb.Helper()

	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)

	cfg := resolveBenchConfig()
	seed := [32]byte{1}
	const players = 4

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)
	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		tb.Fatalf("rules.NewMatch() = %v", err)
	}
	runtime.ReadMemStats(&after)
	initBytes = after.TotalAlloc - before.TotalAlloc

	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	orders := make(map[game.SeatID]game.Order, players)
	for seat := range game.SeatID(players) {
		orders[seat] = idleOrder
	}

	var totalResolveBytes uint64
	for round := 1; round <= cfg.Rounds; round++ {
		runtime.ReadMemStats(&before)
		next, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
		runtime.ReadMemStats(&after)
		if err != nil {
			tb.Fatalf("rules.Resolve() round %d: %v", round, err)
		}
		totalResolveBytes += after.TotalAlloc - before.TotalAlloc
		s = next
	}
	resolveBytes = totalResolveBytes / uint64(cfg.Rounds)

	return initBytes, resolveBytes
}

// TestResolveAllocationBudget is D45's own self-check: "asserts the freshly
// measured bytes-per-call figure is within 10% of the hardcoded
// BytesPerResolveCall/BytesPerInitialCall constants, and fails, not skips,
// on drift — the PR that changed Resolve's allocation shape is the PR that
// must update the constant, in the same diff, the same discipline
// scripts/bots-isolation-allowlist.txt already enforces for a different
// hazard." Portability to other GOARCH/Go-version combinations is this
// file's own build tag's job, not this function's — see the file header.
func TestResolveAllocationBudget(t *testing.T) {
	initBytes, resolveBytes := measureResolveAllocationBudget(t)

	assertWithinTenPercent(t, "BytesPerInitialCall", initBytes, opsmetrics.BytesPerInitialCall)
	assertWithinTenPercent(t, "BytesPerResolveCall", resolveBytes, opsmetrics.BytesPerResolveCall)
}

// assertWithinTenPercent fails (never skips) when measured is more than 10%
// away from constant in either direction.
//
// Why 10%: the worst drift measured on this file's own pinned amd64
// build — across the one axis (-race) this file's header documents as
// in-scope rather than excluded — is ~2% (resolveBytes, under -race). 10%
// leaves roughly 5x that margin, the same "band comfortably clear of
// measured noise, tight enough to still catch a real change" shape
// scripts/check-bench-regression.sh's own THRESHOLD documents (its 10%
// default against an observed ~1-6% single-benchmark drift). A real
// allocation-shape change — adding a field to a struct Resolve threads
// through every round, say — moves these numbers by far more than 10%, so
// the band is not close to hiding one.
func assertWithinTenPercent(t *testing.T, name string, measured, constant uint64) {
	t.Helper()

	var diff uint64
	if measured > constant {
		diff = measured - constant
	} else {
		diff = constant - measured
	}

	// diff/constant > 0.10, computed without floats to keep this test's own
	// arithmetic exact rather than introducing the float noise this
	// measurement is specifically designed to avoid — cross-multiply
	// instead of dividing: diff*10 > constant is equivalent to
	// diff/constant > 0.10 for constant > 0.
	if constant == 0 {
		t.Fatalf("%s: hardcoded constant is 0 — cannot express a 10%% band around zero; opsmetrics.%s needs a real measured value", name, name)
	}
	if diff*10 > constant {
		t.Errorf("%s: freshly measured %d bytes, hardcoded opsmetrics.%s = %d bytes — drift of %.1f%% exceeds the 10%% budget; the PR that changed this allocation shape must update the constant in the same diff (D45)",
			name, measured, name, constant, 100*float64(diff)/float64(constant))
	}
}
