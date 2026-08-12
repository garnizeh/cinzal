package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestMarketRefreshDueOddRoundsOnly pins D25: round 1's own Phase 3 is the
// first roll, and "every 2 rounds" from there lands on odd rounds — 1, 3,
// 5, ..., 15 — not even ones.
func TestMarketRefreshDueOddRoundsOnly(t *testing.T) {
	for round := 1; round <= 15; round++ {
		want := round%2 == 1
		if got := MarketRefreshDue(game.RoundNumber(round)); got != want {
			t.Errorf("MarketRefreshDue(%d) = %v, want %v", round, got, want)
		}
	}
}

// TestRollMarketStockDrawsThreeDistinctItems is D25's ruling: 3 distinct
// items per roll, never a repeat, via the RFC §6.4-mandated partial
// Fisher-Yates shape — and RFC §6.4's market.stock row's exact index cost.
func TestRollMarketStockDrawsThreeDistinctItems(t *testing.T) {
	rng := NewRNG(testSeed(3), 1)
	got := RollMarketStock(rng)

	if len(got) != 3 {
		t.Fatalf("len(RollMarketStock) = %d, want 3", len(got))
	}
	if consumed := rng.Consumed(PurposeMarketStock); consumed != 3 {
		t.Fatalf("Consumed(market.stock) = %d, want 3", consumed)
	}

	seen := map[game.ItemID]bool{}
	for _, item := range got {
		if seen[item] {
			t.Fatalf("RollMarketStock returned duplicate item %s", item)
		}
		seen[item] = true
	}
}

// TestRefreshMarketsConsumesThreePerMarketRefreshed is RFC §6.4's
// market.stock row: exactly 3 draws per Black Market node, none for any
// other node type, in a graph with more than one market.
func TestRefreshMarketsConsumesThreePerMarketRefreshed(t *testing.T) {
	s := MatchState{
		Graph: Graph{Nodes: []Node{
			{ID: 0, Type: game.NodeWarehouse},
			{ID: 1, Type: game.NodeBlackMarket},
			{ID: 2, Type: game.NodeAlley},
			{ID: 3, Type: game.NodeBlackMarket},
		}},
	}

	rng := NewRNG(testSeed(4), 1)
	got := RefreshMarkets(s, rng)

	if consumed := rng.Consumed(PurposeMarketStock); consumed != 6 {
		t.Fatalf("Consumed(market.stock) = %d, want 6 (2 markets x 3 each)", consumed)
	}
	if len(got.Graph.Nodes[1].Market) != 3 {
		t.Errorf("node 1 (Black Market) Market = %v, want 3 items", got.Graph.Nodes[1].Market)
	}
	if len(got.Graph.Nodes[3].Market) != 3 {
		t.Errorf("node 3 (Black Market) Market = %v, want 3 items", got.Graph.Nodes[3].Market)
	}
	if got.Graph.Nodes[0].Market != nil {
		t.Errorf("node 0 (Warehouse) Market = %v, want nil", got.Graph.Nodes[0].Market)
	}
	if got.Graph.Nodes[2].Market != nil {
		t.Errorf("node 2 (Alley) Market = %v, want nil", got.Graph.Nodes[2].Market)
	}
}

// TestRefreshMarketsDoesNotAliasInputState confirms the clone-mutate-return
// shape every other state-rebuilder in this package uses (AcceptOffer,
// DeclineOffer, contracts.go): the caller's pre-call MatchState must be
// untouched.
func TestRefreshMarketsDoesNotAliasInputState(t *testing.T) {
	s := MatchState{Graph: Graph{Nodes: []Node{{ID: 0, Type: game.NodeBlackMarket}}}}
	rng := NewRNG(testSeed(5), 1)

	_ = RefreshMarkets(s, rng)

	if s.Graph.Nodes[0].Market != nil {
		t.Fatalf("RefreshMarkets mutated the caller's original MatchState in place")
	}
}
