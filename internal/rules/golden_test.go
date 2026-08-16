package rules

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// fogAwareRoute finds a route from `from` toward `to` that GDD §15.0's
// legality check accepts: every intermediate step must be non-Hidden to
// this seat, and only the route's last entry may be Hidden (that is how a
// node gets discovered at all). It restricts its search to the subgraph
// this seat currently knows about — Fog != FogHidden — and, when `to`
// itself is still Hidden, routes to the nearest known neighbour of `to`
// and appends `to` as the final, discovering step. Nil if nothing toward
// `to` is knowable yet from the current position (never happens here: a
// seat's own position and its edges are always non-Hidden).
func fogAwareRoute(g Graph, fog []game.FogState, from, to game.NodeID) []game.NodeID {
	if from == to {
		return nil
	}

	visitable := func(n game.NodeID) bool { return fog[n] != game.FogHidden }

	prev := map[game.NodeID]game.NodeID{}
	visited := map[game.NodeID]bool{from: true}
	queue := []game.NodeID{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if !visitable(cur) {
			continue // never expand past a Hidden node
		}
		for _, next := range g.Nodes[cur].Edges {
			if !visited[next] {
				visited[next] = true
				prev[next] = cur
				queue = append(queue, next)
			}
		}
	}

	buildPath := func(target game.NodeID) []game.NodeID {
		var path []game.NodeID
		for n := target; n != from; n = prev[n] {
			path = append([]game.NodeID{n}, path...)
		}
		return path
	}

	if visited[to] {
		return buildPath(to)
	}

	var bestNeighbour game.NodeID
	bestLen := -1
	for _, n := range g.Nodes[to].Edges {
		if !visited[n] || !visitable(n) {
			continue
		}
		if l := len(buildPath(n)); bestLen < 0 || l < bestLen {
			bestNeighbour, bestLen = n, l
		}
	}
	if bestLen < 0 {
		return nil
	}
	return append(buildPath(bestNeighbour), to)
}

// TestGoldenMatchFinalScoreLandsInGDDBands is issue #76's own acceptance
// criterion: a full 15-round, 4-player match — real Resolve() calls, real
// Phase 2 contract offers, real fog-respecting movement and delivery —
// whose winning seat's FinalScore breakdown lands inside GDD §16's four
// component reference bands (contracts 8-16, posts 4-8, majority 0-3,
// cash 2-7) and the general total band (14-34).
//
// It does not assert GDD §16's tighter winner sub-band (26-36): that
// band describes a real competitive match, where every seat plays and the
// winner's total is inflated by everyone else's activity (contested
// sectors, opponents' own crate pickups, etc.). This scenario keeps seats
// 1-3 idle for determinism (see below), so seat 0 "wins" by default at
// whatever total its own conservative single-actor loop produces — never
// a competitive high. Before D29 (#163), this script generated and
// accepted each round's contract offer inline, in the same round, via a
// test-local RNG; that shape could not exist in a real match (D29's own
// Reasoning: "There is no position inside Resolve(round N) that could
// produce round N's own offer in time for round N's own orders"), and it
// let seat 0 cycle contracts faster than the real Contact Cooldown ever
// permits. Now that ContractChoice flows through the real fold, seat 0's
// PendingOffer is staged a round ahead of when it can be answered, and
// this seed's Nobody-tier cooldown yields exactly 3 accept opportunities
// in 15 rounds (rounds 1, 6, 11) instead of the old shape's faster cycle
// — correctly slower, not a regression.
//
// Only seat 0 acts: it accepts Tier 0 contract offers (up to the
// 2-contract cap) and shuttles cargo between Origin and Destination by the
// shortest route it currently knows a path along (fogAwareRoute); it
// stakes two posts in its own starting sector once it isn't carrying
// anything, deferred until there are just enough rounds left for the
// lease to survive to round 15 — deliberately confined to one sector so
// MajorityRP stays inside its 0-3 band rather than sweeping every sector
// against opponents who never contest one; and it opportunistically
// detours for any Dead Runner or Spilled Load crate this round's events
// announce, a small genuine bonus alongside the contract cycle. Seats 1-3
// stay put the entire match (empty route, Nothing, Neutral) — this keeps
// the scenario free of confrontations entirely, which would otherwise
// inject RNG-driven balance and cargo outcomes no hand-picked seed could
// keep predictable.
//
// The route toward any given target is recomputed fresh every round from
// the seat's actual live position — never a multi-round route persisted
// and sliced across calls — so a round Legal() rejects (or degrades) for
// a reason this script didn't anticipate (GDD §16's own reference
// simulation shows Dragnet sealing this exact scenario's Border mid-match)
// self-corrects on the very next round instead of desyncing from reality.
func TestGoldenMatchFinalScoreLandsInGDDBands(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(7)

	s, err := initial(seed, cfg, 4)
	if err != nil {
		t.Fatalf("initial() error = %v", err)
	}
	homeSector := s.Graph.Nodes[s.Players[0].Position].Sector

	// The unbound-cargo branch below hardcodes node 14 as "any Border" to
	// deliver a loose crate to. Asserted once, up front, so a future
	// map-generation change that stops making node 14 a Border under this
	// seed fails here, by name, rather than as a silently degraded Deliver
	// order deep in the round loop.
	const knownBorder = 14
	if s.Graph.Nodes[knownBorder].Type != game.NodeBorder {
		t.Fatalf("node %d is %v under seed 7, not a Border — this scenario's hardcoded loose-crate delivery target needs updating", knownBorder, s.Graph.Nodes[knownBorder].Type)
	}

	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	postsStaked := 0
	const maxPosts = 2
	var crateAt *game.NodeID // announced Dead Runner / Spilled Load crate, unclaimed

	// blocksNeeded is the fewest lease blocks (GDD §10.4, capped at 4)
	// whose rounds survive from round through cfg.Rounds's own Upkeep
	// decrement — "live at the end" needs strictly more rounds than
	// decrements, not merely enough to reach zero. Staking later needs
	// fewer blocks for the same guarantee, which is exactly why staking
	// is deferred (below) rather than done as early as balance allows.
	blocksNeeded := func(round game.RoundNumber) int {
		remaining := cfg.Rounds - int(round) + 1
		for blocks := 1; blocks <= 4; blocks++ {
			if blocks*cfg.LeaseBlockRounds > remaining {
				return blocks
			}
		}
		return 4
	}

	for round := game.RoundNumber(1); round <= game.RoundNumber(cfg.Rounds); round++ {
		// Phase 2 (GDD §8.1-8.2, D29): seat 0's PendingOffer, if any, was
		// already staged by the previous round's Resolve call (or, for
		// round 1, by initial()'s own bootstrap) — real draws, on
		// Resolve's own RNG stream, no test-local phase driving. This
		// script answers it via ContractChoice on this round's order,
		// exactly the input surface a real player has; Resolve applies it
		// at its own head, before validate.
		//
		// Stops taking new work past round 13: a contract's own
		// pickup-plus-deliver round trip through only-just-discovered
		// territory can run long, and a contract still active at match
		// end pays GDD §16's -2 penalty instead of its RP — worse than
		// simply not accepting it with too few rounds left to finish.
		// Only Tier 0 is accepted at all: once accumulated Infamy makes
		// Tier I+ the offer's own highest-eligible slot, taking it would
		// commit this script to a farther, fog-slower delivery than the
		// short Tier 0 loop this scenario is built around. A round's
		// offer holds at most one accepted contract now (GDD §8.1: accept
		// one, or decline) — unlike the old test-local accept loop, which
		// could take two Tier 0 slots from the same offer in one round.
		var contractChoice *int
		if round <= 13 && len(s.Players[0].Contracts) < 2 {
			for i, o := range s.Players[0].PendingOffer {
				if o.Tier == 0 {
					idx := i
					contractChoice = &idx
					break
				}
			}
		}

		p := s.Players[0]

		var target game.NodeID
		var action game.ActionKind
		hasTarget := false
		stakeHere := false

		// Staking takes priority over contract activity for seat 0's first
		// two opportunities once it's ready to start — deliberately, so
		// PostsRP isn't left to whatever time the contract cycle happens
		// not to need. A player already carrying cargo finishes that
		// delivery first regardless (abandoning cargo mid-route is never
		// worth it here). Held off until round 10, deliberately later
		// than the earliest round a lease could still survive to round
		// 15 (round 5, at the maximum 4 blocks) — staking later needs
		// fewer blocks for the same guarantee (blocksNeeded), so this
		// leaves rounds 1-9 entirely to the delivery cycle and spends
		// less on the lease once it does happen.
		readyToStake := round >= 10 && postsStaked < maxPosts
		if p.Cargo == nil && readyToStake && s.Graph.Nodes[p.Position].Post == nil && s.Graph.Nodes[p.Position].Sector == homeSector {
			stakeHere = true
		} else if p.Cargo == nil && readyToStake {
			for _, n := range s.Graph.Nodes[p.Position].Edges {
				if s.Graph.Nodes[n].Post == nil && s.Graph.Nodes[n].Sector == homeSector {
					target, action, hasTarget = n, game.ActionStakePost, true
					break
				}
			}
		}
		if !stakeHere && !hasTarget {
			switch {
			case p.Cargo != nil && p.Cargo.Bound:
				if idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == p.Cargo.Contract }); idx >= 0 {
					target, action, hasTarget = p.Contracts[idx].Destination, game.ActionDeliver, true
				}
			case p.Cargo != nil:
				// An unbound crate (below) delivers to any Border —
				// knownBorder (asserted above) is already this scenario's
				// own well-known one.
				target, action, hasTarget = knownBorder, game.ActionDeliver, true
			case crateAt != nil:
				// A genuinely free RP source: an announced Dead Runner or
				// Spilled Load crate (GDD §14.2/§14.3), collectible by
				// anyone, takes priority over starting a fresh contract
				// pickup — it's already nearby and doesn't cost this
				// scenario's carefully-timed cycle anything extra.
				target, action, hasTarget = *crateAt, game.ActionPickup, true
				crateAt = nil
			case len(p.Contracts) > 0:
				target, action, hasTarget = p.Contracts[0].Origin, game.ActionPickup, true
			}
		}

		var route []game.NodeID
		if hasTarget {
			route = fogAwareRoute(s.Graph, p.Fog, p.Position, target)
		}

		maxSteps := cfg.StepsByTier[infamyTierIndex(p.Infamy, cfg)]
		routeThisRound := route
		if len(routeThisRound) > maxSteps {
			routeThisRound = routeThisRound[:maxSteps]
		}
		arrived := hasTarget && len(routeThisRound) == len(route)

		order := game.Order{Route: routeThisRound, Stance: game.StanceOrder{Stance: game.StanceNeutral}, ContractChoice: contractChoice}
		switch {
		case stakeHere:
			order.Action = game.ActionOrder{Kind: game.ActionStakePost}
			order.AddOns.RenewBlocks = blocksNeeded(round)
			postsStaked++
		case arrived:
			order.Action = game.ActionOrder{Kind: action}
			if action == game.ActionStakePost {
				order.AddOns.RenewBlocks = blocksNeeded(round)
				postsStaked++
			}
		default:
			order.Action = game.ActionOrder{Kind: game.ActionNothing}
		}

		orders := map[game.SeatID]game.Order{0: order, 1: idleOrder, 2: idleOrder, 3: idleOrder}

		next, events, err := Resolve(s, orders, cfg, NewRNG(seed, int(round)))
		if err != nil {
			t.Fatalf("round %d: Resolve() error = %v", round, err)
		}
		for _, e := range events {
			if e.Kind == game.EventDeadRunnerCrate || e.Kind == game.EventSpilledLoadCrate {
				node := e.Node
				crateAt = &node
			}
		}
		s = next
	}

	if s.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("s.Round = %d after the loop, want %d", s.Round, cfg.Rounds)
	}

	breakdowns := FinalScore(s)
	winner := breakdowns[0]
	t.Logf("winner breakdown: %+v", winner)
	t.Logf("seat0: Balance=%d Infamy=%d ContractsDelivered=%d Posts=%v Contracts=%+v",
		s.Players[0].Balance, s.Players[0].Infamy, s.Players[0].ContractsDelivered, s.Players[0].Posts, s.Players[0].Contracts)

	// GDD §16's reference simulation states the general spread as [14, 34]
	// and a tighter [26, 36] sub-band for a real competitive match's
	// winner. This scenario only asserts the former (see this test's own
	// doc comment for why the winner sub-band does not apply to a
	// single-actor deterministic script) plus the four component bands,
	// which the winner sub-band does not supersede.
	checks := []struct {
		name     string
		got      int
		min, max int
	}{
		{"PriorRP (delivered contracts)", winner.PriorRP, 8, 16},
		{"PostsRP", winner.PostsRP, 4, 8},
		{"MajorityRP", winner.MajorityRP, 0, 3},
		{"CashRP", winner.CashRP, 2, 7},
	}
	for _, c := range checks {
		if c.got < c.min || c.got > c.max {
			t.Errorf("%s = %d, want [%d, %d]", c.name, c.got, c.min, c.max)
		}
	}
	if winner.Total < 14 || winner.Total > 34 {
		t.Errorf("winner Total = %d, want [14, 34] (GDD §16's general band)", winner.Total)
	}
}
