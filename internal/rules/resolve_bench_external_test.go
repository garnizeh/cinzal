// Package rules_test (external): the allocation-budget self-check this file
// used to hold (debug.SetGCPercent(-1) in measureResolveAllocationBudget is a
// process-wide side effect, and the deferred restore only undoes it after
// the call returns — it does not make the mutation itself invisible to
// whatever else is running in the same test binary meanwhile) lives in
// resolve_alloc_budget_amd64_test.go now, gated to amd64 only — issue #352
// found the hardcoded opsmetrics.BytesPerInitialCall/BytesPerResolveCall
// constants are a fact about one build, not portable across GOARCH, so that
// file's own build tag keeps the self-check off any other architecture
// rather than asserting an equality nobody has measured. BenchmarkResolve
// below carries no such assertion — it is unaffected and stays here,
// runnable on any GOARCH.
//
// This file, like that one, uses the external rules_test package for the
// same isolation newmatch_external_test.go and its siblings already use,
// reaching only rules.NewMatch/rules.Resolve/rules.NewRNG — everything this
// file needs is already exported.
package rules_test

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
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
