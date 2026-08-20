package bots

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// runnerReserve is RFC-001 §14.3's "keeps Cr$ 4 in reserve for the
// shakedown" clause, read literally: Cr$ 4 is DefaultConfig's own
// cfg.ShakedownCost (GDD §15 — "Cr$ an Evasive loser pays to keep their
// cargo"), not a second constant duplicating it. A standing floor on
// spend, including on leases (issue #193) — maybeRenewLease is the one
// place Runner spends anything at all, and it never lets a renewal drop
// the balance below this.
func runnerReserve(cfg game.Config) int {
	return cfg.ShakedownCost
}

// RunnerOptions is Runner's own tunable dial set — parameters the design
// calls out as bot behaviour, not game rule, and therefore never
// game.Config's business (issue #193: "it is a bot parameter... belongs to
// the tier's own options struct, sweepable by cmd/simulate like any other
// dial").
type RunnerOptions struct {
	// InfamyComfortBand is RFC-001 §14.3's "Vanishes when Infamy exceeds
	// its comfort band": Runner declares Vanish, whenever nothing more
	// productive is happening at the route's ending node, once Infamy is
	// strictly above this value.
	InfamyComfortBand int
}

// DefaultRunnerOptions is the tier's out-of-the-box tuning: the comfort
// band sits at the top of GDD §9.1's Feared band (Infamy 6-8), so Runner
// works to stay out of Legend (9-10) — the tier that broadcasts its exact
// position at end of round (GDD §11) — without spending an action vanishing
// every single round it happens to sit at Feared.
func DefaultRunnerOptions() RunnerOptions {
	return RunnerOptions{InfamyComfortBand: 8}
}

// runnerBot is the Runner tier (RFC-001 §14.3): greedy pathing to the
// current contract objective, the Autopilot default (§8.2). See the
// per-clause comments below and on each of this file's helpers for how
// each phrase of the RFC's Runner sentence maps to code.
type runnerBot struct {
	opts RunnerOptions
}

// NewRunner builds a Runner tier with opts. For(Runner) returns
// NewRunner(DefaultRunnerOptions()) — this constructor exists so
// cmd/simulate's sweeps (issue #193, RFC §16.4) can build a Runner with a
// swept InfamyComfortBand without touching Bot, Tier or For.
func NewRunner(opts RunnerOptions) Bot {
	return runnerBot{opts: opts}
}

func (b runnerBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	objective, haveObjective := runnerObjective(v)
	route := b.chooseRoute(v, Routes(v, cfg), objective, haveObjective)

	order := game.Order{
		Round:  v.Round,
		Route:  route.Nodes,
		Stance: runnerStance(v),
		AddOns: maybeRenewLease(v, cfg),
	}

	if route.EndsHidden {
		order.PushingOn = choosePushingOn(v, objective, haveObjective)
	}

	// legalPushingOn (internal/rules/legal.go) forbids combining a
	// declared Pushing On with any action but Nothing — "you have no idea
	// where you'll be" (GDD §9.1). pushingDeclared is legalspace.go's own
	// zero-value check, shared rather than re-derived.
	if pushingDeclared(order.PushingOn) {
		order.Action = game.ActionOrder{Kind: game.ActionNothing}
	} else {
		order.Action = b.chooseAction(v, route, objective, haveObjective)
	}

	order.ContractChoice = chooseContractChoice(v, cfg)

	return order
}

// runnerObjective is RFC-001 §14.3's "current contract objective": the one
// node this round's route heads for, and whether one exists at all — a
// Runner holding no contract and carrying nothing has no objective, which
// is not an error case (see chooseRoute's explore fallback).
//
// Priority mirrors the physical delivery loop, not an arbitrary ranking:
// cargo already in hand has a destination waiting, which always outranks a
// not-yet-picked-up contract's origin, because picking up a second
// contract's cargo is impossible while a seat is only ever carrying one
// (GDD §5, §8.4).
func runnerObjective(v game.PlayerView) (game.NodeID, bool) {
	if cargo := v.You.Cargo; cargo != nil {
		if !cargo.Bound {
			return nearestKnownBorder(v)
		}
		for _, c := range v.You.Contracts {
			if c.ID == cargo.Contract {
				return c.Destination, true
			}
		}
		// Bound cargo whose contract is no longer in v.You.Contracts has
		// no destination this seat can name — not reachable under the
		// GDD's own contract lifecycle (a contract leaves the hand only on
		// delivery, which requires the cargo already gone), so this falls
		// through to "no objective" rather than guessing a wrong border.
		return 0, false
	}

	// Not carrying: head for the most urgent held contract's pickup
	// point. "Current" reads as most-urgent when up to two are held (GDD
	// §8.4) — soonest ExpiresRound first, lowest ContractID breaking a
	// tie, matching RFC-001 §6.5's ascending-ID tie-break discipline.
	if len(v.You.Contracts) > 0 {
		best := v.You.Contracts[0]
		for _, c := range v.You.Contracts[1:] {
			if c.ExpiresRound < best.ExpiresRound ||
				(c.ExpiresRound == best.ExpiresRound && c.ID < best.ID) {
				best = c
			}
		}
		return best.Origin, true
	}

	return 0, false
}

// nearestKnownBorder is unbound cargo's objective: "unbound cargo delivers
// to any Border" (GDD §8.4), so Runner heads for whichever Known Border is
// closest on its own known subgraph, ascending NodeID breaking a tie. When
// no Known Border is yet reachable on that subgraph (isolated by an
// unexplored gap), it falls back to the lowest-NodeID Known Border by
// identity alone — chooseRoute's explore mode then steers toward it by
// canvas coordinates instead of a walked path.
func nearestKnownBorder(v game.PlayerView) (game.NodeID, bool) {
	dist := knownDistances(v, v.You.Position)

	best, bestDist, found := game.NodeID(0), 0, false
	for _, id := range knownNodeIDs(v) {
		if v.Nodes[id].Type != game.NodeBorder {
			continue
		}
		if d, reachable := dist[id]; reachable && (!found || d < bestDist) {
			best, bestDist, found = id, d, true
		}
	}
	if found {
		return best, true
	}

	for _, id := range knownNodeIDs(v) {
		if v.Nodes[id].Type == game.NodeBorder {
			return id, true
		}
	}
	return 0, false
}

// knownDistances is a BFS over this seat's own known subgraph — exactly
// legalspace.go's distanceTwoNodeIDs walk, generalised to return every
// reachable node's hop count from an arbitrary seed rather than filtering
// for distance 2. Runner needs this shape three separate times (current
// position to today's objective, a pending offer's Origin to its
// Destination, and — indirectly — nearestKnownBorder's own search), so it
// is written once here instead of three times.
func knownDistances(v game.PlayerView, from game.NodeID) map[game.NodeID]int {
	dist := map[game.NodeID]int{from: 0}
	queue := []game.NodeID{from}

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
			// A Known node's own Edges can legitimately name a neighbour
			// still Rumoured or entirely Hidden to this seat (that is
			// exactly what a scouting step onto a Hidden node is) — gated
			// out here so dist only ever describes a walkable known
			// subgraph, never a hop count through a node no legal route
			// can enter (legalRoute, internal/rules/legal.go).
			nv, nextKnown := v.Nodes[next]
			if !nextKnown || nv.Fog < game.FogKnown {
				continue
			}
			dist[next] = dist[cur] + 1
			queue = append(queue, next)
		}
	}

	return dist
}

// chooseRoute is RFC-001 §14.3's "shortest path to the current contract
// objective" — over the view's own graph, which is not the real one
// (issue #193). Routes (legalspace.go) already bounds every candidate to
// this round's step allowance and to edges the view actually discloses;
// chooseRoute only ranks those candidates.
//
// When objective is unreachable on this seat's own known subgraph — no
// objective at all, or one whose path here has not been found yet — this
// is where the honest fallback issue #193 asks for lives: exploreRoute
// steers by the objective's own disclosed canvas coordinates and pushes
// into unmapped territory to go find a path, rather than refusing to move.
func (b runnerBot) chooseRoute(v game.PlayerView, routes []Route, objective game.NodeID, haveObjective bool) Route {
	if !haveObjective {
		return exploreRoute(v, routes, 0, false)
	}

	// Rooted at objective, not at the current position: nearestRoute scores
	// several candidate endpoints against one fixed target, so this needs
	// "distance from each candidate to objective," which a BFS rooted at
	// objective gives directly for every node in one pass (edges are
	// symmetric, so this equals "distance from objective to each
	// candidate," the more intuitive reading of the same walk).
	dist := knownDistances(v, objective)
	if _, reachable := dist[v.You.Position]; reachable {
		return nearestRoute(v, routes, dist)
	}

	return exploreRoute(v, routes, objective, true)
}

// routeEndpoint is the node a route (as built by Routes, legalspace.go)
// ends on: the current position for the empty route, its last entry
// otherwise.
func routeEndpoint(v game.PlayerView, nodes []game.NodeID) game.NodeID {
	if len(nodes) == 0 {
		return v.You.Position
	}
	return nodes[len(nodes)-1]
}

// nearestRoute is chooseRoute's case for a reachable objective: dist
// (knownDistances from the current position) gives an exact hop count for
// every route ending on a node already in the view, so this is a plain
// arg-min over that. Routes ending on a totally Hidden node (EndsHidden)
// are excluded outright — scouting is pointless once a known way already
// exists. Ties break on route length first (the shortest walk that
// reaches the best distance, never one that overshoots it) and then on the
// route's own node sequence, ascending — RFC-001 §6.5's tie-break
// discipline, the same shape legalspace.go's own helpers already use, so
// two seats facing the identical choice make the identical move.
func nearestRoute(v game.PlayerView, routes []Route, dist map[game.NodeID]int) Route {
	best := routes[0]
	bestDist, bestKnown := routeDistance(v, best, dist)

	for _, rt := range routes[1:] {
		d, known := routeDistance(v, rt, dist)
		if !known {
			continue
		}
		switch {
		case !bestKnown:
		case d < bestDist:
		case d == bestDist && len(rt.Nodes) < len(best.Nodes):
		case d == bestDist && len(rt.Nodes) == len(best.Nodes) && slices.Compare(rt.Nodes, best.Nodes) < 0:
		default:
			continue
		}
		best, bestDist, bestKnown = rt, d, known
	}

	return best
}

func routeDistance(v game.PlayerView, rt Route, dist map[game.NodeID]int) (int, bool) {
	if rt.EndsHidden {
		return 0, false
	}
	d, ok := dist[routeEndpoint(v, rt.Nodes)]
	return d, ok
}

// exploreRoute is chooseRoute's honest fallback (issue #193) for when the
// objective's distance cannot be computed on this seat's own known
// subgraph — either there is no objective this round at all
// (haveObjective false), or one exists but no known path to it has been
// found yet. Every candidate is scored by squared Euclidean distance, in
// the map's own canvas coordinates (disclosed from Rumoured onward,
// view.go), from its last node actually present in the view to the
// objective — a route that pushes onto a totally Hidden node is scored
// from the last Known waypoint before it, since the Hidden node itself
// carries no coordinates yet.
//
// Ties prefer the route that pushes furthest into unmapped territory
// (EndsHidden), since that is what actually turns "no known path" into a
// known one on a future round; a genuinely undirected walk (haveObjective
// false) has every candidate score identically and so is decided by the
// same tie-break alone, which in that case reduces to "cover the most
// ground, deterministically" — standing still teaches nothing.
func exploreRoute(v game.PlayerView, routes []Route, objective game.NodeID, haveObjective bool) Route {
	var targetX, targetY int
	if haveObjective {
		nv := v.Nodes[objective]
		targetX, targetY = nv.X, nv.Y
	}

	best := routes[0]
	bestScore := exploreScore(v, best, targetX, targetY, haveObjective)

	for _, rt := range routes[1:] {
		score := exploreScore(v, rt, targetX, targetY, haveObjective)
		switch {
		case score < bestScore:
		case score == bestScore && preferExploreTie(rt, best):
		default:
			continue
		}
		best, bestScore = rt, score
	}

	return best
}

func preferExploreTie(candidate, current Route) bool {
	if candidate.EndsHidden != current.EndsHidden {
		return candidate.EndsHidden
	}
	if len(candidate.Nodes) != len(current.Nodes) {
		return len(candidate.Nodes) > len(current.Nodes)
	}
	return slices.Compare(candidate.Nodes, current.Nodes) < 0
}

func exploreScore(v game.PlayerView, rt Route, targetX, targetY int, haveObjective bool) int {
	if !haveObjective {
		return 0
	}

	end := routeEndpoint(v, rt.Nodes)
	view, known := v.Nodes[end]
	if !known {
		// end is itself a totally Hidden scouting step — score from the
		// last node actually in the view instead: the route's
		// second-to-last entry, or the current position for a one-step
		// scout straight off it.
		if len(rt.Nodes) >= 2 {
			view = v.Nodes[rt.Nodes[len(rt.Nodes)-2]]
		} else {
			view = v.Nodes[v.You.Position]
		}
	}

	dx, dy := view.X-targetX, view.Y-targetY
	return dx*dx + dy*dy
}

// choosePushingOn declares as much blind continuation as GDD §9.1 allows
// (2 steps) once a route has already run off the edge of the map, biased
// toward the objective's own Sector when one is known — Sector, like X/Y,
// is disclosed from Rumoured onward, so an objective this seat has only
// heard of still names a direction to push toward (GDD §9.1's priority
// ladder). No bias at all — an unweighted blind walk — when there is
// nothing to steer by.
func choosePushingOn(v game.PlayerView, objective game.NodeID, haveObjective bool) game.PushingOn {
	var bias *game.Sector
	if haveObjective {
		if nv, known := v.Nodes[objective]; known {
			s := nv.Sector
			bias = &s
		}
	}
	return game.PushingOn{Steps: 2, Bias: bias}
}

// runnerStance is RFC-001 §14.3's "Evasive while carrying" clause: Evasive
// the instant there is cargo to lose, whether bound or a loose crate — the
// stance a Runner would want during any confrontation it did not seek is
// the one that keeps the cargo (GDD §9.3, §15). Neutral otherwise: a
// greedy bot never wants a confrontation for its own sake, so Aggressive,
// and the stake that comes with it, never fires.
func runnerStance(v game.PlayerView) game.StanceOrder {
	if v.You.Cargo != nil {
		return game.StanceOrder{Stance: game.StanceEvasive}
	}
	return game.StanceOrder{Stance: game.StanceNeutral}
}

// chooseAction picks GDD §9.2's action field once Route (and, when it
// applies, PushingOn) are already settled. Deliver and Pickup — the two
// halves of "shortest path to the current contract objective" actually
// paying off — outrank everything else; Vanish is what a Runner does with
// an action it would otherwise waste once Infamy has climbed past its
// comfort band (RunnerOptions.InfamyComfortBand); Nothing is the only
// remaining case. Deal is never a candidate at all — "never buys" (RFC
// §14.3) is enforced structurally, by this switch simply never naming it,
// the same way legalAction never sees a Deal from this tier to reject.
func (b runnerBot) chooseAction(v game.PlayerView, route Route, objective game.NodeID, haveObjective bool) game.ActionOrder {
	end := routeEndpoint(v, route.Nodes)

	switch {
	case runnerCanDeliver(v, end):
		return game.ActionOrder{Kind: game.ActionDeliver}
	case runnerCanPickup(v, end, objective, haveObjective):
		return game.ActionOrder{Kind: game.ActionPickup}
	case v.You.Infamy > b.opts.InfamyComfortBand:
		return game.ActionOrder{Kind: game.ActionVanish}
	default:
		return game.ActionOrder{Kind: game.ActionNothing}
	}
}

// runnerCanDeliver mirrors internal/rules' own unexported deliverableCargo
// (legal.go) plus its node-type check, computed locally: this package may
// read game.PlayerView alone, never reach into rules for a helper that
// isn't Legal, Affordances, Steps or *BotRNG (bot_test.go's own dependency
// check). Unbound (loose) cargo may deliver at any Border; bound cargo
// needs a held contract whose own Destination is end (GDD §8.1, §8.4).
func runnerCanDeliver(v game.PlayerView, end game.NodeID) bool {
	cargo := v.You.Cargo
	if cargo == nil {
		return false
	}

	nv, known := v.Nodes[end]
	if !known || nv.Type != game.NodeBorder {
		return false
	}

	if !cargo.Bound {
		return true
	}
	for _, c := range v.You.Contracts {
		if c.ID == cargo.Contract && c.Destination == end {
			return true
		}
	}
	return false
}

// runnerCanPickup declares Pickup only once the route has actually reached
// this round's objective and that objective is a held contract's own
// Origin (runnerObjective only ever names an Origin while v.You.Cargo is
// nil) — GDD §9.2's Pickup is legal at "a Warehouse matching a held
// contract," and this is the one Origin Runner currently knows to be
// exactly that. DockersStrike (GDD §14.2, issue #72) suspends Pickup
// outright, unconditionally, matching legalAction's own check.
func runnerCanPickup(v game.PlayerView, end, objective game.NodeID, haveObjective bool) bool {
	return v.You.Cargo == nil &&
		!v.You.DockersStrike &&
		haveObjective &&
		end == objective &&
		len(v.You.Contracts) > 0
}

// leaseAboutToLapse is maybeRenewLease's own lookahead: a post with this
// many rounds remaining or fewer counts as "about to lapse" (GDD §18).
//
// The window is in rounds, not blocks, so it holds at every
// cfg.LeaseBlockRounds — but only for a post the bot can still see. A lease
// that lapses before it is ever observed is outside any window's reach, and
// keeping it observable is the *staking* side's job, not this one's:
// operator.go's stakeBlocksToSurviveUpkeep funds every fresh stake with
// enough blocks to survive its own round's Upkeep, so a post always reaches
// the next Decide with at least one round remaining — inside this window by
// definition (issue #236).
const leaseAboutToLapse = 2

// maybeRenewLease is GDD §18's Autopilot heuristic ("renew leases about to
// lapse") applied under RFC-001 §14.3's own reserve clause: a lease about
// to expire is renewed by exactly one block, and only when doing so leaves
// the balance at or above runnerReserve(cfg) — the standing Cr$ 4 floor
// issue #193 states applies "including on leases." The fewest-rounds-
// remaining selection mirrors rules.SelectLeaseByFewestRounds (posts.go —
// the same selection Affordances uses for the UI's own "which lease is
// closest to lapsing" question) rather than calling it: this package may
// read internal/rules only for Legal, Affordances, Steps and *BotRNG
// (legalspace_test.go's own dependency check), the same discipline
// legalspace.go's knownSubgraphDistance duplicate already follows.
//
// One block is always enough to hold the post for another round, at every
// LeaseBlockRounds: resolveAddons tops the lease up by cfg.LeaseBlockRounds
// (addons.go) before the same round's Upkeep takes one round back off it
// (upkeepLeases, upkeep.go), so even the shortest possible block leaves the
// post live and back inside leaseAboutToLapse's window next round.
func maybeRenewLease(v game.PlayerView, cfg game.Config) game.AddOns {
	if cfg.LeaseCostPerBlock <= 0 || len(v.You.Posts) == 0 {
		return game.AddOns{}
	}

	target := v.You.Posts[0]
	for _, p := range v.You.Posts[1:] {
		if p.RoundsRemaining < target.RoundsRemaining ||
			(p.RoundsRemaining == target.RoundsRemaining && p.Node < target.Node) {
			target = p
		}
	}
	if target.RoundsRemaining > leaseAboutToLapse {
		return game.AddOns{}
	}

	if v.You.Balance-cfg.LeaseCostPerBlock < runnerReserve(cfg) {
		return game.AddOns{}
	}

	return game.AddOns{RenewPost: target.Node, RenewBlocks: 1}
}

// unknownContractDistance stands in for "this leg's length cannot be
// measured from the view" in runnerOfferReachable's total — large enough
// that it is never mistaken for a real hop count, never added into a
// budget comparison directly (only ever guarded behind the knownToOrigin/
// knownLeg2 flags that say whether it is real).
const unknownContractDistance = 1 << 30

// chooseContractChoice is RFC-001 §14.3's "Runner answers a pending offer
// ... Greedy means: take the highest-value tier it can reach inside the
// deadline given Steps(v, cfg) and its known map, decline nothing it can
// plausibly finish" (issue #193, D29). Offers are compared by tier first
// (higher always wins), then by the known portion of their own route
// distance as a tie-break between two offers of the same tier — the more
// certain choice, all else equal.
func chooseContractChoice(v game.PlayerView, cfg game.Config) *int {
	if len(v.You.PendingOffer) == 0 {
		return nil
	}

	stepsPerRound := max(rules.Steps(v, cfg), 1)
	fromCurrent := knownDistances(v, v.You.Position)

	best := -1
	bestTier := -1
	bestKnownDist := unknownContractDistance

	for i, offer := range v.You.PendingOffer {
		reachable, dist := runnerOfferReachable(v, cfg, fromCurrent, offer, stepsPerRound)
		if !reachable {
			continue
		}

		switch {
		case best < 0:
		case offer.Tier > bestTier:
		case offer.Tier == bestTier && dist < bestKnownDist:
		default:
			continue
		}
		best, bestTier, bestKnownDist = i, offer.Tier, dist
	}

	if best < 0 {
		return nil
	}
	return &best
}

// runnerOfferReachable estimates whether offer can plausibly be finished
// within cfg.Contracts[offer.Tier].Deadline rounds, using only this seat's
// own known subgraph and this round's own step rate as a stand-in for
// every future round's — "the reachability estimate is over the known
// graph again, and it will sometimes be wrong — that is a Runner, not a
// bug" (issue #193). Origin is always at least Known before an offer ever
// exists (GDD §8.1), so the current-position leg is normally measurable;
// Destination is disclosed only once a contract is accepted (GDD §8.1), so
// the Origin-to-Destination leg is frequently not. An unmeasurable leg is
// never treated as proof of infeasibility — "decline nothing it can
// plausibly finish" reads as: only a leg Runner can actually measure may
// rule an offer out, and an offer with no measurable leg at all is always
// plausible. The returned distance is the sum of whichever legs are
// known, unknownContractDistance standing in for an unmeasured leg only
// when the offer is accepted as plausible on that basis — never fed back
// into the deadline comparison itself.
func runnerOfferReachable(v game.PlayerView, cfg game.Config, fromCurrent map[game.NodeID]int, offer game.ContractOffer, stepsPerRound int) (bool, int) {
	toOrigin, knownToOrigin := fromCurrent[offer.Origin]
	originToDest, knownLeg2 := knownDistances(v, offer.Origin)[offer.Destination]

	if !knownToOrigin || !knownLeg2 {
		total := unknownContractDistance
		if knownToOrigin {
			total = toOrigin
		}
		return true, total
	}

	total := toOrigin + originToDest
	if budget := cfg.Contracts[offer.Tier].Deadline * stepsPerRound; total > budget {
		return false, total
	}
	return true, total
}
