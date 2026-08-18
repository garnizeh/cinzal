package bots

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// This file is the toolkit issue #191 asks for: the legal order space,
// computed from a game.PlayerView alone. rules exposes three view-only
// functions — Legal (a predicate over a *complete* order), Affordances
// (what a partial draft may still do), and Steps (this round's budget) —
// and nothing that generates a candidate. Routes and Actions below are that
// generator's two reusable stages; Sample composes them, plus five smaller
// fields, into one legal game.Order per BotRNG draw.
//
// Every field here reads game.PlayerView and game.Config only — never
// rules.MatchState, which this package may not even name (bot_test.go's
// TestPackageNamesNoMatchStateNowhere). Route enumeration walks the same
// way the UI will (RFC-001 §11.1: click by click, re-validating each time),
// using Affordances as the step function, exactly as issue #191 specifies.

// Route is one legal route candidate from v.You.Position, bounded by this
// round's step allowance. Nodes is nil for the empty (stay-put) route,
// which is always included (GDD §9.1) — Routes never returns an empty
// slice. EndsHidden mirrors Affordance.RouteBlocked: true when Nodes' last
// entry is a node absent from the view entirely, the only kind of ending a
// Pushing On declaration may extend from.
type Route struct {
	Nodes      []game.NodeID
	EndsHidden bool
}

// Routes enumerates every route rules.Legal would accept for o.Route alone,
// from v.You.Position, bounded by Steps(v, cfg). It walks exactly the path
// legalRoute (internal/rules/legal.go) validates: a step must leave a node
// this seat has at Known fog or better, over an edge that node's own view
// discloses, and may never land on a node merely Rumoured — a Rumoured node
// "has no edges, so you cannot plot a route through or into it" (GDD §7.1).
// A step onto a node absent from v.Nodes (Hidden) is legal only as the
// route's last entry, which falls out here for free: the next iteration's
// Affordances call reports RouteBlocked and the walk does not extend past
// it.
//
// Every prefix along a legal walk is itself a legal, independently
// submittable route (GDD §9.1's "an empty route — staying put — is legal"
// generalises to every partial route, not only the empty one), so Routes
// records one at every step, not only at maximal depth.
func Routes(v game.PlayerView, cfg game.Config) []Route {
	var routes []Route

	var walk func(prefix []game.NodeID)
	walk = func(prefix []game.NodeID) {
		a := rules.Affordances(v, cfg, game.Order{Round: v.Round, Route: prefix})
		routes = append(routes, Route{
			Nodes:      append([]game.NodeID(nil), prefix...),
			EndsHidden: a.RouteBlocked,
		})

		if a.RouteBlocked || a.StepsRemaining == 0 {
			return
		}

		node := v.You.Position
		if len(prefix) > 0 {
			node = prefix[len(prefix)-1]
		}
		view, known := v.Nodes[node]
		if !known || view.Fog < game.FogKnown {
			return
		}

		for _, to := range view.Edges {
			if toView, toKnown := v.Nodes[to]; toKnown && toView.Fog < game.FogKnown {
				continue // Rumoured — cannot route through or into it (GDD §7.1)
			}
			walk(append(append([]game.NodeID(nil), prefix...), to))
		}
	}
	walk(nil)

	return routes
}

// endsOnHidden reports whether route terminates on a node absent from v —
// the route.EndsHidden condition, recomputed here so PushingOns can be
// called directly against a route a caller already has in hand, without
// requiring a Route value from Routes.
func endsOnHidden(v game.PlayerView, route []game.NodeID) bool {
	node := v.You.Position
	if len(route) > 0 {
		node = route[len(route)-1]
	}
	_, known := v.Nodes[node]
	return !known
}

// pushingDeclared mirrors rules' own unexported pushingOnDeclared
// (internal/rules/legal.go): the zero value (Steps: 0, Bias: nil) means
// none was declared.
func pushingDeclared(o game.PushingOn) bool {
	return o.Steps != 0 || o.Bias != nil
}

var allSectors = [...]game.Sector{
	game.SectorOldDocks,
	game.SectorIronLow,
	game.SectorMistHeights,
	game.SectorNorthVale,
}

// PushingOns enumerates every legal Pushing On declaration for a route
// (GDD §9.1): the "not declared" zero value always, plus — only when route
// ends on a Hidden node, the sole kind of ending Pushing On may extend from
// — every (blind step count, sector bias) pair legalPushingOn accepts:
// Steps 0-2, Bias nil or one of the four sectors. (Steps 0 with a nil Bias
// is the zero value itself — GDD §9.1 gives no way to declare "Pushing On,
// zero blind steps, no bias" as distinct from "not declared" — so that one
// combination is not double-listed.)
func PushingOns(v game.PlayerView, route []game.NodeID) []game.PushingOn {
	if !endsOnHidden(v, route) {
		return []game.PushingOn{{}}
	}

	biases := []*game.Sector{nil}
	for _, s := range allSectors {
		biases = append(biases, new(s))
	}

	options := []game.PushingOn{{}}
	for steps := 0; steps <= 2; steps++ {
		for _, bias := range biases {
			if steps == 0 && bias == nil {
				continue
			}
			options = append(options, game.PushingOn{Steps: steps, Bias: bias})
		}
	}
	return options
}

// Actions enumerates every ActionOrder rules.Legal accepts for draft, with
// every other field of draft (Route, PushingOn, Stance, Items, AddOns)
// exactly as the caller has already decided them — Legal validates an
// order jointly, so a Deal that only becomes affordable after draft.Items'
// own discards, or a Stake Post at the post cap, are correctly included or
// excluded by trialling the real order rather than a hand-rolled copy of
// legalAction's rules. Always includes ActionNothing.
//
// When draft.PushingOn declares a continuation, GDD §9.1/§15.0 forbid
// combining it with any action ("you have no idea where you'll be"), so
// Actions returns only {ActionNothing} without trialling anything else.
//
// ActionDeal is offered once per item on the ending node's disclosed
// Market — only populated when that node is InSight and a Black Market
// (GDD §7.1, §12) — never a blind guess at an undisclosed stock.
func Actions(v game.PlayerView, cfg game.Config, draft game.Order) []game.ActionOrder {
	if pushingDeclared(draft.PushingOn) {
		return []game.ActionOrder{{Kind: game.ActionNothing}}
	}

	node := v.You.Position
	if len(draft.Route) > 0 {
		node = draft.Route[len(draft.Route)-1]
	}

	candidates := []game.ActionOrder{
		{Kind: game.ActionNothing},
		{Kind: game.ActionVanish},
		{Kind: game.ActionSurveil},
		{Kind: game.ActionPickup},
		{Kind: game.ActionStakePost},
		{Kind: game.ActionDeliver},
	}
	if view, known := v.Nodes[node]; known {
		for _, item := range view.Market {
			candidates = append(candidates, game.ActionOrder{Kind: game.ActionDeal, Item: item})
		}
	}

	var legal []game.ActionOrder
	for _, c := range candidates {
		trial := draft
		trial.Action = c
		if rules.Legal(v, trial, cfg) == nil {
			legal = append(legal, c)
		}
	}
	return legal
}

// sortedNodeIDs collects ids, ranged from a map by the caller, into
// ascending order — the RFC-001 §6.3 discipline every map-derived candidate
// list in this file follows, so a BotRNG index draw against the result is
// reproducible regardless of Go's randomised map iteration order.
func sortedNodeIDs(ids map[game.NodeID]bool) []game.NodeID {
	out := make([]game.NodeID, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// knownNodeIDs returns every node at Fog >= Known, ascending — Decoy's own
// target requirement (GDD §9.4, D12/#50: "Decoy's target node is not Known
// to this seat" is legalItemTargets' rejection for anything less).
func knownNodeIDs(v game.PlayerView) []game.NodeID {
	ids := make(map[game.NodeID]bool, len(v.Nodes))
	for id, nv := range v.Nodes {
		if nv.Fog >= game.FogKnown {
			ids[id] = true
		}
	}
	return sortedNodeIDs(ids)
}

// heardOfNodeIDs returns every node present in v.Nodes regardless of fog —
// Police Band's own target requirement (D25): "any node this seat has at
// least heard of qualifies."
func heardOfNodeIDs(v game.PlayerView) []game.NodeID {
	ids := make(map[game.NodeID]bool, len(v.Nodes))
	for id := range v.Nodes {
		ids[id] = true
	}
	return sortedNodeIDs(ids)
}

// distanceTwoNodeIDs returns every Known node exactly graph-distance 2 from
// v.You.Position on this seat's own known subgraph — Bolt Hole's own target
// requirement (D25), computed the same way legalItemTargets' unexported
// knownSubgraphDistance does (internal/rules/legal.go), but locally: this
// package may read only game.PlayerView, never reach into rules for a
// helper that isn't Legal, Affordances, Steps or *BotRNG.
func distanceTwoNodeIDs(v game.PlayerView) []game.NodeID {
	dist := map[game.NodeID]int{v.You.Position: 0}
	queue := []game.NodeID{v.You.Position}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		view, known := v.Nodes[cur]
		if !known || view.Fog < game.FogKnown {
			continue
		}
		for _, next := range view.Edges {
			if _, seen := dist[next]; seen {
				continue
			}
			dist[next] = dist[cur] + 1
			queue = append(queue, next)
		}
	}

	at2 := make(map[game.NodeID]bool)
	for id, d := range dist {
		if d != 2 {
			continue
		}
		// legalItemTargets checks Bolt Hole's target against Fog >= Known
		// *and* distance == 2 as two separate conditions — a node can sit
		// at distance 2 on the BFS above (reached through a Known
		// predecessor's own Edges) while still being merely Rumoured
		// itself, which distance alone would wrongly admit here.
		if view, known := v.Nodes[id]; known && view.Fog >= game.FogKnown {
			at2[id] = true
		}
	}
	return sortedNodeIDs(at2)
}

// sampleStance draws one of Neutral, Evasive, or Aggressive at a stake
// uniform in 0..Affordances.MaxStake (GDD §9.3's Cr$ 0-6 cap, clamped to
// this seat's own balance) — the range Affordances computes for exactly
// this purpose (RFC-001 §10.2 row 6), even though Legal itself does not
// enforce the upper bound (deliberately: "an affordance clamp, not a
// submission-time reject", internal/rules/affordance.go). Sample follows
// the affordance-intended range regardless, the same discipline the real
// UI's stake control would.
func sampleStance(v game.PlayerView, cfg game.Config, draft game.Order, r *rules.BotRNG) game.StanceOrder {
	a := rules.Affordances(v, cfg, draft)

	n := 2 + a.MaxStake + 1 // Neutral, Evasive, Aggressive(0..MaxStake)
	switch i := r.NextBot("bot.sample.stance", n); i {
	case 0:
		return game.StanceOrder{Stance: game.StanceNeutral}
	case 1:
		return game.StanceOrder{Stance: game.StanceEvasive}
	default:
		return game.StanceOrder{Stance: game.StanceAggressive, Stake: i - 2}
	}
}

// sampleItems decides, independently per held item instance, whether to
// discard it this round (GDD §9.4: "discard any number of items you hold")
// — Muscle excluded outright, since it is "not a discard" and may never be
// declared regardless of whether it is held (legalItemTargets). A discard
// requiring a target (Decoy, Police Band, Bolt Hole) draws one uniformly
// from its own legal candidate set; an item with no legal target this round
// is kept rather than declared with an invalid one.
func sampleItems(v game.PlayerView, r *rules.BotRNG) []game.ItemDiscard {
	var discards []game.ItemDiscard

	for _, item := range v.You.Items {
		if item == game.ItemMuscle {
			continue
		}
		if r.NextBot("bot.sample.discard", 2) == 0 {
			continue // keep
		}

		d := game.ItemDiscard{Item: item}
		switch item {
		case game.ItemDecoy:
			targets := knownNodeIDs(v)
			if len(targets) == 0 {
				continue
			}
			d.Target = targets[r.NextBot("bot.sample.discard.target", len(targets))]
		case game.ItemPoliceBand:
			targets := heardOfNodeIDs(v)
			if len(targets) == 0 {
				continue
			}
			d.Target = targets[r.NextBot("bot.sample.discard.target", len(targets))]
		case game.ItemBoltHole:
			targets := distanceTwoNodeIDs(v)
			if len(targets) == 0 {
				continue
			}
			d.Target = targets[r.NextBot("bot.sample.discard.target", len(targets))]
		}
		discards = append(discards, d)
	}

	return discards
}

// renewTargets is AddOns.RenewPost's candidate set: every post this seat
// already holds, plus — only when draft.Action is this same order's own
// Stake Post — the node it is staking (Affordances' own comment: "most
// commonly the node this same order is staking new"). Ascending by NodeID.
func renewTargets(v game.PlayerView, draft game.Order) []game.NodeID {
	seen := make(map[game.NodeID]bool, len(v.You.Posts)+1)
	for _, p := range v.You.Posts {
		seen[p.Node] = true
	}
	if draft.Action.Kind == game.ActionStakePost {
		node := v.You.Position
		if len(draft.Route) > 0 {
			node = draft.Route[len(draft.Route)-1]
		}
		seen[node] = true
	}
	return sortedNodeIDs(seen)
}

// sampleAddOns decides GDD §9.5's two add-ons. Buy Ledger is offered only
// when affordable and not the match's final round (GDD §5.1); the lease
// block count is drawn against Affordances.MaxLeaseBlocks computed with
// BuyLedger's own decision already reserved (Affordances.go's own comment:
// "so the two clamps stay jointly affordable"), and a nonzero draw needs a
// legal target to attach to or it is dropped back to zero rather than left
// pointing at nothing.
func sampleAddOns(v game.PlayerView, cfg game.Config, draft game.Order, r *rules.BotRNG) game.AddOns {
	var addOns game.AddOns

	canBuyLedger := int(draft.Round) < cfg.Rounds && cfg.LedgerCost <= v.You.Balance
	if canBuyLedger && r.NextBot("bot.sample.addons.ledger", 2) == 1 {
		addOns.BuyLedger = true
	}

	withLedger := draft
	withLedger.AddOns = addOns
	a := rules.Affordances(v, cfg, withLedger)

	if a.MaxLeaseBlocks > 0 {
		if targets := renewTargets(v, draft); len(targets) > 0 {
			if blocks := r.NextBot("bot.sample.addons.blocks", a.MaxLeaseBlocks+1); blocks > 0 {
				addOns.RenewBlocks = blocks
				addOns.RenewPost = targets[r.NextBot("bot.sample.addons.renewtarget", len(targets))]
			}
		}
	}

	return addOns
}

// sampleAbandonCargo draws GDD §9.3's free cargo-abandon flag — meaningful,
// and so only ever drawn true, when this seat is actually carrying
// something.
func sampleAbandonCargo(v game.PlayerView, r *rules.BotRNG) bool {
	if v.You.Cargo == nil {
		return false
	}
	return r.NextBot("bot.sample.abandon", 2) == 1
}

// sampleContractChoice draws GDD §8.1-8.2/D29's Phase 2 answer: nil
// (decline) when no offer is pending, else uniformly among decline or one
// of the pending offer's own indices.
func sampleContractChoice(v game.PlayerView, r *rules.BotRNG) *int {
	n := len(v.You.PendingOffer)
	if n == 0 {
		return nil
	}
	i := r.NextBot("bot.sample.contractchoice", n+1)
	if i == n {
		return nil
	}
	return &i
}

// Sample draws one order from the legal space this file enumerates: a
// route from Routes, a Pushing On declaration from PushingOns when that
// route ends Hidden, a stance, this round's item discards, an action from
// Actions, the two add-ons, the abandon-cargo flag, and this round's
// contract choice — each stage drawn uniformly from its own legal set via
// r, in the order the fields depend on each other (item discards before
// the action they can unlock a Deal's hand-limit room for; the action
// before a Stake Post's node becomes a renewal target).
//
// This is the generator issue #191 asks for, not yet the exact statistical
// claim issue #192 wires into Drifter: "uniform within each stage" is not
// the same distribution as "uniform over the whole order space" — a route
// with many legal actions is not over- or under-weighted against one with
// few by this composition, which #192's own acceptance criteria states
// outright ("definition, not an algorithm").
func Sample(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	routes := Routes(v, cfg)
	route := routes[r.NextBot("bot.sample.route", len(routes))].Nodes

	draft := game.Order{
		Round:  v.Round,
		Route:  route,
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
		Action: game.ActionOrder{Kind: game.ActionNothing},
	}

	pushings := PushingOns(v, route)
	draft.PushingOn = pushings[r.NextBot("bot.sample.pushingon", len(pushings))]

	draft.Stance = sampleStance(v, cfg, draft, r)
	draft.Items = sampleItems(v, r)

	actions := Actions(v, cfg, draft)
	draft.Action = actions[r.NextBot("bot.sample.action", len(actions))]

	draft.AddOns = sampleAddOns(v, cfg, draft, r)
	draft.AbandonCargo = sampleAbandonCargo(v, r)
	draft.ContractChoice = sampleContractChoice(v, r)

	return draft
}
