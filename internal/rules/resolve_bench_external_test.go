// Package rules_test (external): debug.SetGCPercent(-1) in
// measureResolveAllocationBudget is a process-wide side effect, and the
// deferred restore only undoes it after the call returns — it does not make
// the mutation itself invisible to whatever else is running in the same test
// binary meanwhile. internal/rules' own tests do no I/O and touch no ambient
// state at all (test-authoring's own rule), so this measurement lives in the
// external rules_test package instead, the same isolation
// newmatch_external_test.go and its siblings already use, reaching only
// rules.NewMatch/rules.Resolve/rules.NewRNG — everything this file needs is
// already exported.
package rules_test

import (
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
	"github.com/garnizeh/cinzal/internal/rules"
)

// resolveBenchConfig is a pinned literal snapshot of game.DefaultConfig(),
// captured at authoring time rather than read live — the same discipline
// internal/rules/gen/bench_test.go's own benchCases() already states the
// reason for: a future Config tuning pass (a MapByPlayers edit like #229's,
// a Contracts reweight) must not silently move this file's own benchmark
// baseline, or TestResolveAllocationBudget's measured constants, out from
// under an unrelated PR. A change meant to move this baseline updates this
// literal in the same diff as whatever changed DefaultConfig() (D45's own
// consequence: "the PR that changes Resolve's allocation shape is the PR
// that must update the constant, in the same diff").
func resolveBenchConfig() game.Config {
	return game.Config{
		Rounds:            15,
		StepsByTier:       [4]int{4, 4, 3, 2},
		CooldownByTier:    [4]int{4, 3, 2, 1},
		PostCapByPlayers:  map[int]int{2: 4, 3: 4, 4: 4, 5: 3},
		LeaseCostPerBlock: 3,
		LeaseBlockRounds:  3,
		ShakedownCost:     4,
		LedgerCost:        3,
		GateFee:           1,
		StartingBalance:   12,
		Contracts: [4]game.ContractTier{
			{InfamyRequired: 0, MinDistance: 3, MaxDistance: 4, Payment: 8, RP: 2, Deadline: 4, Penalty: 3, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 3, MinDistance: 4, MaxDistance: 6, Payment: 14, RP: 3, Deadline: 5, Penalty: 5, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 6, MinDistance: 5, MaxDistance: 8, Payment: 20, RP: 5, Deadline: 5, Penalty: 8, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 9, MinDistance: 6, MaxDistance: 0, Payment: 30, RP: 8, Deadline: 6, Penalty: 12, PenaltyInfamy: 2, OfferWeight: 1},
		},
		MapByPlayers: map[int]game.MapSpec{
			2: {Nodes: 15, MinEdges: 21, MaxEdges: 23},
			3: {Nodes: 22, MinEdges: 31, MaxEdges: 35},
			4: {Nodes: 28, MinEdges: 41, MaxEdges: 45},
			5: {Nodes: 28, MinEdges: 40, MaxEdges: 45},
		},
		MaxGenAttempts: 1000,
		Scavenging:     game.ScavengingTable{CashRoll: 4, CashAmount: 3, RevealRoll: 6},
		Pressure:       game.PressureConfig{Threshold: 2, CashPenalty: 5, InfamyPenalty: 1},
		// Suppress is the zero value: an ordinary match suppresses nothing —
		// same as game.DefaultConfig()'s own comment on this field.
	}
}

// resolveBenchIdleOrders returns one idle order per seat (ActionNothing,
// StanceNeutral) — the same "everybody idles" shape golden_test.go's own
// idleOrder uses. Sufficient to drive a real Resolve call every round
// without depending on any particular map topology, card draw, or contract
// state — this file measures Resolve's own baseline allocation cost, not a
// worst case or a typical case.
func resolveBenchIdleOrders(players int) map[game.SeatID]game.Order {
	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	orders := make(map[game.SeatID]game.Order, players)
	for seat := game.SeatID(0); int(seat) < players; seat++ {
		orders[seat] = idleOrder
	}
	return orders
}

// BenchmarkResolve is the human-eyeballing benchmark D45 asks for
// ("a BenchmarkResolve for human eyeballing"), one row per GDD §6.1 player
// count, mirroring internal/rules/gen/bench_test.go's own per-player-count
// pattern. NewMatch runs once per case outside the timed loop (its own cost,
// including map-generation retries, is not what this benchmark measures);
// the timed loop is Resolve alone, replaying round 1's idle orders
// repeatedly against the same starting state — b.Loop() semantics mean the
// state is not threaded round-to-round here, unlike a real fold, since the
// goal is Resolve's own per-call cost, not a full match's.
func BenchmarkResolve(b *testing.B) {
	cfg := resolveBenchConfig()
	seed := [32]byte{11}

	for _, players := range []int{2, 3, 4, 5} {
		b.Run(playersLabel(players), func(b *testing.B) {
			s, err := rules.NewMatch(seed, cfg, players)
			if err != nil {
				b.Fatalf("rules.NewMatch(players=%d) = %v", players, err)
			}
			orders := resolveBenchIdleOrders(players)

			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, 1)); err != nil {
					b.Fatalf("rules.Resolve(players=%d) = %v", players, err)
				}
			}
		})
	}
}

func playersLabel(players int) string {
	switch players {
	case 2:
		return "players=2"
	case 3:
		return "players=3"
	case 4:
		return "players=4"
	case 5:
		return "players=5"
	default:
		return "players=?"
	}
}

// measureResolveAllocationBudget is the deterministic, GC-disabled
// measurement D45 specifies: debug.SetGCPercent(-1) for the duration,
// runtime.MemStats.TotalAlloc read before/after each call, restored on
// return. Allocation count and bytes are deterministic given deterministic
// code — unlike wall-clock time, this needs no bench-compare-style noise
// tolerance (D45's own reasoning).
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
	for seat := game.SeatID(0); seat < players; seat++ {
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
// hazard."
func TestResolveAllocationBudget(t *testing.T) {
	initBytes, resolveBytes := measureResolveAllocationBudget(t)

	assertWithinTenPercent(t, "BytesPerInitialCall", initBytes, opsmetrics.BytesPerInitialCall)
	assertWithinTenPercent(t, "BytesPerResolveCall", resolveBytes, opsmetrics.BytesPerResolveCall)
}

// assertWithinTenPercent fails (never skips) when measured is more than 10%
// away from constant in either direction.
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
