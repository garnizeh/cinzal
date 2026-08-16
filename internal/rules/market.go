package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// MarketRefreshDue reports whether round is one of GDD §12/RFC §6.4's Phase 3
// market-refresh rounds — the odd rounds 1, 3, 5, ..., cfg.Rounds (D25).
// Round 1's own Phase 3 is the only mechanism that populates a market's
// first stock: Setup does not (GDD §4 lists only map generation, starting
// positions, and the opening contract offer), and RFC §6.4's consumption
// table has no separate at-Setup row for market.stock the way deck.event
// and deck.incident each get one — so round 1 must be a refresh round, and
// "every 2 rounds" measured from there lands on odd rounds, not even ones.
// The upper bound is cfg.Rounds, not a literal 15: GDD §16.2 rules out a
// longer table today only on deck arithmetic, so a config that ever clears
// that bar must not have markets silently stop refreshing partway through
// (RFC §6.6 — "a rule simply stops firing, and nobody notices").
func MarketRefreshDue(round game.RoundNumber, cfg game.Config) bool {
	return round >= 1 && int(round) <= cfg.Rounds && round%2 == 1
}

// RollMarketStock draws 3 distinct items for one Black Market's refreshed
// stock (GDD §12: "3 rolled items"), via partial Fisher-Yates over the fixed
// 8-item catalog (items.go), sorted by ItemID — its declaration order
// already is. D25 settles the one thing RFC §6.4's market.stock row left
// unstated: distinct items, the same without-replacement shape every other
// multi-pick RNG consumer in this package uses (Torn Map, Dragnet), never
// independent draws that could repeat and undercut the market's own
// scarcity design.
func RollMarketStock(rng *RNG) []game.ItemID {
	candidates := make([]game.ItemID, len(allItems))
	for i, it := range allItems {
		candidates[i] = it.Item
	}
	return PartialFisherYates(rng, PurposeMarketStock, candidates, 3)
}

// RefreshMarkets rolls fresh stock for every Black Market node in s, in
// NodeID-ascending order (Graph.Nodes's own invariant — Node's comment),
// consuming exactly 3 PurposeMarketStock draws per market refreshed (RFC
// §6.4's market.stock row). It refreshes unconditionally whenever called —
// callers must gate this on MarketRefreshDue themselves (Resolve, #67,
// Phase 3) — and replaces each market's stock wholesale, matching the
// clone-mutate-return shape this package's other state-rebuilders use
// (AcceptOffer, DeclineOffer, contracts.go).
func RefreshMarkets(s MatchState, rng *RNG) MatchState {
	nodes := slices.Clone(s.Graph.Nodes)
	for i, n := range nodes {
		if n.Type != game.NodeBlackMarket {
			continue
		}
		n.Market = RollMarketStock(rng)
		nodes[i] = n
	}
	s.Graph.Nodes = nodes
	return s
}
