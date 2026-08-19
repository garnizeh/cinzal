package bots

import (
	"math"
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// OperatorOptions is the tier's own tunable dial set (issue #193's own
// precedent for RunnerOptions: "a bot parameter... belongs to the tier's
// own options struct, sweepable by cmd/simulate like any other dial" —
// never game.Config's business, since none of these numbers is a game
// rule).
type OperatorOptions struct {
	// ChokepointTrafficRate is the minimum NodeStats traffic rate
	// (TrafficRounds/ObservedRounds) that marks a node a chokepoint worth
	// leasing (GDD §7.5's Heat Map).
	ChokepointTrafficRate float64

	// ChokepointMinObserved is the minimum NodeStats.ObservedRounds before
	// a rate is trusted at all — GDD §7.5's own "anything under three
	// observations is flagged low-confidence."
	ChokepointMinObserved int

	// SectorAvoidanceWeight scales how many extra steps Operator will
	// tolerate to route around this round's Unstable Sector, at the deck's
	// current hazard fraction (HazardsRemaining/(HazardsRemaining+
	// BoonsRemaining), GDD §14.3). At hazard fraction 1.0 the full weight
	// applies; at 0 (an exhausted hazard deck) it never steers around the
	// sector at all — "weighted by the displayed deck counts" read
	// literally.
	SectorAvoidanceWeight float64

	// InfamyComfortBand mirrors RunnerOptions' own field: Vanish is a
	// candidate once Infamy climbs strictly above this value (GDD §11's
	// Feared ceiling by default).
	InfamyComfortBand int

	// InfamyCooldownMargin is how many RoundsToNextOffer count as "the
	// next offer is close": at or under this margin, Operator holds off
	// Vanishing even past InfamyComfortBand, because GDD §11.1 evaluates
	// Contact Cooldown and contract-tier gating "at Phase 2, when the offer
	// is generated" — cashing in the current tier on that specific offer
	// outweighs the exposure cost of one more round at it.
	InfamyCooldownMargin int

	// ThreatItemThreshold is the minimum threatEstimate score (see
	// threatEstimate) that makes Operator buy a combat item at a reachable
	// Black Market, rather than pocket the cash.
	ThreatItemThreshold int

	// ThreatLookback is how many rounds back through v.Archive.Trail count
	// as "recent" when scoring confrontation threat.
	ThreatLookback game.RoundNumber

	// ThreatInfamyFloor is the minimum opponent Infamy that adds to
	// threatEstimate — GDD §11's Feared combat modifier (+2) starts here,
	// so an opponent at or above this Infamy is scored as a real fight
	// risk.
	ThreatInfamyFloor int

	// BandJumpMinTier is the minimum contract tier (0-3, GDD §8.3) an
	// opponent's EventDelivered anchor must carry to count as evidence of a
	// likely credit-band jump.
	BandJumpMinTier int

	// BandJumpLookback is how many rounds back through v.Anchors count as
	// "recent" when scanning for a rival's band-jumping delivery.
	BandJumpLookback game.RoundNumber
}

// DefaultOperatorOptions is the tier's out-of-the-box tuning.
//
// ChokepointTrafficRate at 0.98 and ChokepointMinObserved at 9 sit far
// above GDD §7.5's own bare low-confidence floor (three observations, 50%):
// GDD §10.1's own worked example is explicit that a post bought for its 2
// RP alone is "clearly a loss on points" against its lease cost — "you're
// buying the vision and the sector majority, not the two points," and the
// simulation harness bears this out (bots_operator_golden_external_test.go,
// internal/rules): every threshold below this pair measurably cost Operator
// mean RP against Runner, chokepoint leasing included, because a post's
// payoff is conditional (must survive live to round 15) while its lease
// cost is certain. The bar sits high enough that only a corridor near
// certainty — traffic on essentially every observed round, backed by nine
// or more of them — clears it, so the behaviour fires rarely against real
// Heat Map data while remaining fully implemented and independently
// provable against a hand-constructed view (TestOperatorLeasesChokepoint,
// operator_test.go).
//
// SectorAvoidanceWeight at 3 steps means a fully hazard-loaded Unstable
// Sector (deck reads 100% hazard) is worth a 3-step detour to avoid, while
// an exhausted hazard deck (0%) never triggers a detour at all.
// InfamyComfortBand matches RunnerOptions.InfamyComfortBand (the Feared
// ceiling); InfamyCooldownMargin at 1 holds off Vanishing only the one
// round right before the next offer lands. ThreatItemThreshold at 1 means
// either a single opponent at or above ThreatInfamyFloor (6, GDD §11's
// Feared floor) or one recent confrontation nearby is enough to trigger a
// purchase, subject to affordability. BandJumpMinTier at 2 (Tier III) and
// BandJumpLookback at 3 rounds mirror
// GDD §8.3's own jump in payment (Cr$ 20/30 against Tier I/II's Cr$ 8/14)
// — the deliveries actually large enough to move a credit band (GDD
// §5.1).
func DefaultOperatorOptions() OperatorOptions {
	return OperatorOptions{
		ChokepointTrafficRate: 0.98,
		ChokepointMinObserved: 9,
		SectorAvoidanceWeight: 3,
		InfamyComfortBand:     8,
		InfamyCooldownMargin:  1,
		ThreatItemThreshold:   1,
		ThreatLookback:        3,
		ThreatInfamyFloor:     6,
		BandJumpMinTier:       2,
		BandJumpLookback:      3,
	}
}

// operatorItemPreference ranks the combat items Operator will reach for
// under threat (GDD §12): Muscle first (permanent, +1 in every
// confrontation from the moment it's bought), then Shiv (a one-time +3,
// cheaper), then Bolt Hole (does not affect the fight itself, but keeps the
// cargo on a loss) — a defensive ladder, not an exhaustive shopping list.
var operatorItemPreference = [...]game.ItemID{game.ItemMuscle, game.ItemShiv, game.ItemBoltHole}

// operatorBot is the Operator tier (RFC-001 §14.3): plans across rounds —
// reads the heat map for chokepoints and leases them, routes around
// unstable sectors weighted by the displayed deck counts, times the Infamy
// climb against Contact Cooldown, buys items when a confrontation looks
// likely, uses the Ledger when a rival's band jumps. Issue #190 forbids
// cross-round bot state, so "plans across rounds" here means every one of
// those five behaviours is re-derived fresh from the current
// game.PlayerView on every Decide call — nothing is carried between
// rounds. That is workable because the view itself already carries match
// history where these five behaviours need it: v.Archive (this seat's own
// observation history back to round 1) and v.Anchors (each entry stamped
// with its own Round, so the slice reads as the match-to-date public
// anchor log, not only this round's).
//
// Bounded search: Decide makes exactly one pass over Routes(v, cfg) — the
// same view- and step-bounded candidate set Drifter and Runner already
// build from — scoring each candidate with O(1) work (a known-subgraph
// distance lookup plus a sector-hazard penalty, both precomputed once per
// call). No recursion, no lookahead beyond the current round, no branching
// search: the total work is O(|Routes(v, cfg)|), the same bound Runner's
// own chooseRoute already carries, which matters once Decide runs inside
// the tick's SELECT ... FOR UPDATE transaction (RFC-001 §8).
type operatorBot struct {
	opts OperatorOptions
}

// NewOperator builds an Operator tier with opts. For(Operator) returns
// NewOperator(DefaultOperatorOptions()) — this constructor exists so
// cmd/simulate's sweeps (RFC-001 §16.4) can vary any of OperatorOptions'
// dials without touching Bot, Tier or For, the same reason
// runner.go's NewRunner exists.
func NewOperator(opts OperatorOptions) Bot {
	return operatorBot{opts: opts}
}

func (b operatorBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	objective, haveObjective, wantStake := b.chooseObjective(v)
	route := b.chooseRoute(v, Routes(v, cfg), objective, haveObjective)
	end := routeEndpoint(v, route.Nodes)

	draft := game.Order{
		Round:  v.Round,
		Route:  route.Nodes,
		Stance: runnerStance(v),
	}
	if route.EndsHidden {
		draft.PushingOn = choosePushingOn(v, objective, haveObjective)
	}

	// attemptStake is true only once this round's own route actually
	// reaches the chokepoint objective — GDD §10.4's "you declare the
	// duration up front" means a fresh Stake Post must be funded by this
	// same order's AddOns, so chooseAddOns needs to know before Action is
	// even picked, not after.
	attemptStake := wantStake && end == objective
	addOns := b.chooseAddOns(v, cfg, draft, attemptStake, objective)
	stakeFunded := attemptStake && addOns.RenewPost == objective && addOns.RenewBlocks > 0
	draft.AddOns = addOns

	dealItem, wantsDeal := b.wantsItem(v, end)
	wantsVanish := b.timeToVanish(v)

	actions, draft := legalActionsOrRelax(v, cfg, draft)
	draft.Action = pickOperatorAction(v, actions, end, objective, haveObjective, stakeFunded, wantsDeal, dealItem, wantsVanish)

	draft.ContractChoice = chooseContractChoice(v, cfg)
	return draft
}

// chooseObjective is RFC-001 §14.3's "plans across rounds," worked as a
// two-tier priority: a held contract or carried cargo (runnerObjective,
// runner.go — the money-making loop every tier shares) always outranks
// leasing a chokepoint, since a chokepoint is infrastructure for future
// rounds while a contract is this round's income. Only once neither
// applies does Operator go looking for the heat map's best corridor.
// wantStake is true only in the chokepoint case — it tells chooseAction
// this round's objective is a place to stake, not a place to deliver or
// pick up from.
func (b operatorBot) chooseObjective(v game.PlayerView) (game.NodeID, bool, bool) {
	if objective, ok := runnerObjective(v); ok {
		return objective, true, false
	}
	if node, ok := findChokepoint(v, b.opts); ok {
		return node, true, true
	}
	return 0, false, false
}

// findChokepoint is GDD §7.5's Heat Map read as a leasing decision: the
// nearest node in v.NodeStats that clears both ChokepointTrafficRate and
// ChokepointMinObserved, that this seat does not already hold a post on,
// and that is reachable at all on the known subgraph. Ties break on
// higher rate, then ascending NodeID (RFC-001 §6.5).
//
// Nearest, not highest-rate globally: issue #190's no-memory rule means
// this whole decision is re-derived from scratch every round, and
// NodeStats itself shifts as more rounds get observed. Picking the
// single best-rate node on the map, independent of distance, would have
// Operator reverse direction every time a marginally higher-rate node
// entered the qualifying set somewhere else — never actually reaching,
// let alone staking, anything. Proximity-first is what makes the target
// stable round over round: as Operator walks toward it, its own distance
// only shrinks, so nothing but a genuinely closer alternative appearing
// can redirect the walk.
func findChokepoint(v game.PlayerView, opts OperatorOptions) (game.NodeID, bool) {
	ids := make(map[game.NodeID]bool, len(v.NodeStats))
	for id := range v.NodeStats {
		ids[id] = true
	}

	dist := knownDistances(v, v.You.Position)

	best, bestDist, bestRate, found := game.NodeID(0), 0, -1.0, false
	for _, id := range sortedNodeIDs(ids) {
		stats := v.NodeStats[id]
		// ObservedRounds <= 0 is checked ahead of the ChokepointMinObserved
		// comparison, not folded into it: OperatorOptions is exported and
		// swept by cmd/simulate, so a sweep setting ChokepointMinObserved to
		// 0 or below must not let a zero-observation node divide 0/0 into
		// NaN, which compares false against every threshold below and would
		// otherwise slip through as a qualifying chokepoint.
		if stats.ObservedRounds <= 0 || stats.ObservedRounds < opts.ChokepointMinObserved {
			continue
		}
		rate := float64(stats.TrafficRounds) / float64(stats.ObservedRounds)
		if rate < opts.ChokepointTrafficRate {
			continue
		}
		if alreadyHeldBySelf(v, id) {
			continue
		}
		d, reachable := dist[id]
		if !reachable {
			continue
		}
		switch {
		case !found:
		case d < bestDist:
		case d == bestDist && rate > bestRate:
		default:
			continue
		}
		best, bestDist, bestRate, found = id, d, rate, true
	}
	return best, found
}

func alreadyHeldBySelf(v game.PlayerView, id game.NodeID) bool {
	for _, p := range v.You.Posts {
		if p.Node == id {
			return true
		}
	}
	return false
}

// chooseRoute mirrors runner.go's own chooseRoute (objective known and
// reachable on the known subgraph -> rank candidates by distance;
// otherwise explore), with one addition: nearestRoute's ranking is
// Operator's own, weighted by sectorAvoidanceScore below, in place of
// Runner's plain distance ordering.
func (b operatorBot) chooseRoute(v game.PlayerView, routes []Route, objective game.NodeID, haveObjective bool) Route {
	if !haveObjective {
		return exploreRoute(v, routes, 0, false)
	}

	dist := knownDistances(v, objective)
	if _, reachable := dist[v.You.Position]; reachable {
		return b.nearestRoute(v, routes, dist)
	}
	return exploreRoute(v, routes, objective, true)
}

// nearestRoute is chooseRoute's reachable-objective case: the same
// arg-min-over-distance runner.go's own nearestRoute performs, except the
// compared score is distance plus sectorAvoidanceScore's penalty rather
// than distance alone — "routes around unstable sectors weighted by the
// displayed deck counts" (RFC-001 §14.3) applied directly to route
// selection, not as an afterthought filter.
func (b operatorBot) nearestRoute(v game.PlayerView, routes []Route, dist map[game.NodeID]int) Route {
	hazard := sectorHazardFraction(v)

	best := routes[0]
	bestScore, bestKnown := b.routeScore(v, best, dist, hazard)

	for _, rt := range routes[1:] {
		score, known := b.routeScore(v, rt, dist, hazard)
		if !known {
			continue
		}
		switch {
		case !bestKnown:
		case score < bestScore:
		case score == bestScore && len(rt.Nodes) < len(best.Nodes):
		case score == bestScore && len(rt.Nodes) == len(best.Nodes) && slices.Compare(rt.Nodes, best.Nodes) < 0:
		default:
			continue
		}
		best, bestScore, bestKnown = rt, score, known
	}

	return best
}

// routeScore is nearestRoute's per-candidate cost: routeDistance
// (runner.go) in hops, plus sectorAvoidanceScore's own penalty when the
// route ends inside this round's Unstable Sector. Routes ending Hidden are
// excluded by routeDistance itself, exactly as Runner's own nearestRoute
// already excludes them — scouting is pointless once a known way exists.
func (b operatorBot) routeScore(v game.PlayerView, rt Route, dist map[game.NodeID]int, hazard float64) (int, bool) {
	d, known := routeDistance(v, rt, dist)
	if !known {
		return 0, false
	}
	if d == 0 {
		// This route already reaches the objective — never penalized away
		// from finishing it. Sector avoidance only steers the approach
		// (see the test named for this behaviour); once the objective
		// itself sits in the flagged sector, there was never a real
		// alternative to weigh, and refusing to arrive would just strand
		// Operator circling the sector's edge every round it stays
		// flagged.
		return 0, true
	}
	return d + sectorAvoidancePenalty(v, b.opts, rt, hazard), true
}

// sectorHazardFraction is GDD §14.3's "priced gamble," read straight off
// the always-displayed deck counts: HazardsRemaining / (HazardsRemaining +
// BoonsRemaining). Zero when there is no Unstable Sector this round
// (Headline.Sector nil) or the deck has nothing left to draw — both cases
// where there is nothing left to weigh a detour against.
func sectorHazardFraction(v game.PlayerView) float64 {
	if v.Headline.Sector == nil {
		return 0
	}
	total := v.Deck.HazardsRemaining + v.Deck.BoonsRemaining
	if total == 0 {
		return 0
	}
	return float64(v.Deck.HazardsRemaining) / float64(total)
}

// sectorAvoidancePenalty adds SectorAvoidanceWeight * hazard extra
// "distance-equivalent" hops to a route ending inside this round's
// Unstable Sector — enough to make nearestRoute prefer a longer route that
// avoids it when the deck reads mostly hazards, and to leave the ranking
// untouched (penalty 0) when the deck is mostly boons or there is no
// Unstable Sector at all this round.
func sectorAvoidancePenalty(v game.PlayerView, opts OperatorOptions, rt Route, hazard float64) int {
	if v.Headline.Sector == nil || hazard == 0 {
		return 0
	}
	end := routeEndpoint(v, rt.Nodes)
	nv, known := v.Nodes[end]
	if !known || nv.Sector != *v.Headline.Sector {
		return 0
	}
	return int(math.Round(opts.SectorAvoidanceWeight * hazard))
}

// timeToVanish is RFC-001 §14.3's "times the Infamy climb against Contact
// Cooldown": Vanish is a candidate at all only once Infamy has climbed
// past InfamyComfortBand (mirroring Runner's own comfort-band check), but
// Operator additionally holds off — stays at the higher tier one more
// round — whenever v.You.RoundsToNextOffer is at or under
// InfamyCooldownMargin. GDD §11.1 evaluates the Contact Cooldown and
// contract-tier gating "at Phase 2, when the offer is generated," so
// dropping a tier the round before that offer lands would spend the
// climb's own payoff for nothing.
func (b operatorBot) timeToVanish(v game.PlayerView) bool {
	if v.You.Infamy <= b.opts.InfamyComfortBand {
		return false
	}
	return v.You.RoundsToNextOffer > b.opts.InfamyCooldownMargin
}

// wantsItem is RFC-001 §14.3's "buys items when a confrontation looks
// likely": only a candidate at all when the route's own ending node is
// InSight and a Black Market with stock actually on offer (GDD §7.1, §12)
// — Operator does not detour purely to go shopping, it buys opportunistically
// where the round's own route already ends. threatEstimate must clear
// ThreatItemThreshold, and the highest-priority item actually on this
// node's Market (operatorItemPreference) is the one returned.
func (b operatorBot) wantsItem(v game.PlayerView, end game.NodeID) (game.ItemID, bool) {
	nv, known := v.Nodes[end]
	if !known || nv.Fog < game.FogInSight || nv.Type != game.NodeBlackMarket || len(nv.Market) == 0 {
		return 0, false
	}
	if threatEstimate(v, b.opts) < b.opts.ThreatItemThreshold {
		return 0, false
	}
	for _, want := range operatorItemPreference {
		if slices.Contains(nv.Market, want) {
			return want, true
		}
	}
	return 0, false
}

// threatEstimate is the "threat estimate from the archive" RFC-001 §14.3
// calls for, entirely re-derivable each round from game.PlayerView alone
// (issue #190's no-memory rule): one point per opponent at or above
// ThreatInfamyFloor (the Feared/Legend combat modifier, GDD §11's +2/+3
// rows) named in v.Others, plus one point per EventConfrontation entry in
// v.Archive.Trail — this seat's own persistent observation history, GDD
// §7.5 — that arrived within ThreatLookback rounds of the current one.
func threatEstimate(v game.PlayerView, opts OperatorOptions) int {
	threat := 0
	for _, o := range v.Others {
		if o.Infamy >= opts.ThreatInfamyFloor {
			threat++
		}
	}
	for _, e := range v.Archive.Trail {
		if e.Kind != game.EventConfrontation {
			continue
		}
		delta := v.Round - e.Round
		if delta >= 0 && delta <= opts.ThreatLookback {
			threat++
		}
	}
	return threat
}

// chooseAddOns combines the lease-renewal heuristic Runner already has
// (maybeRenewLease, runner.go — "about to lapse" renewal, sharing its own
// runnerReserve(cfg) floor) with wantsLedger's own decision, RFC-001
// §14.3's "uses the Ledger when a rival's band jumps." The two are checked
// for joint affordability here — v.You.Balance must cover both the
// renewal's own cost and cfg.LedgerCost while still leaving
// runnerReserve(cfg) — rather than each independently, since Legal
// validates the whole order's spend at once and Decide must never emit an
// order Legal rejects.
func (b operatorBot) chooseAddOns(v game.PlayerView, cfg game.Config, draft game.Order, attemptStake bool, stakeNode game.NodeID) game.AddOns {
	addOns := maybeRenewLease(v, cfg)

	if addOns.RenewBlocks == 0 && attemptStake {
		if funded, ok := fundNewStake(v, cfg, draft, stakeNode); ok {
			addOns = funded
		}
	}

	if !wantsLedger(v, cfg, b.opts) {
		return addOns
	}

	renewalCost := addOns.RenewBlocks * cfg.LeaseCostPerBlock
	if v.You.Balance-renewalCost-cfg.LedgerCost < runnerReserve(cfg) {
		return addOns
	}

	addOns.BuyLedger = true
	return addOns
}

// fundNewStake is GDD §10.4's "you declare the duration up front," applied
// to Operator's own chokepoint pick: a fresh Stake Post is only worth
// anything if this same order also funds it (a 0-block stake lapses
// immediately, GDD §10.4's own lease table — no sight, no round-15 RP).
// One block (Cr$ 3, 3 rounds) is enough to hold the post past this round;
// maybeRenewLease's own "about to lapse" check keeps it topped up in later
// rounds the same way it already maintains any other post. The combination
// is trialled against rules.Legal directly — the post cap (GDD §10.3) is
// exactly the kind of check this package must read from rules rather than
// re-derive (legalspace.go's own discipline) — so a stake Operator cannot
// actually afford or is capped out of is never spent on at all.
func fundNewStake(v game.PlayerView, cfg game.Config, draft game.Order, node game.NodeID) (game.AddOns, bool) {
	if cfg.LeaseCostPerBlock <= 0 || cfg.LeaseBlockRounds <= 0 {
		return game.AddOns{}, false
	}

	// One block (Cr$ 3, 3 rounds, GDD §10.4) is enough to hold the post
	// past this round without committing the larger up-front sum a
	// multi-block stake would — maybeRenewLease's own "about to lapse"
	// check already tops it up in later rounds the same way it maintains
	// any other post, so the ongoing GDD §10.1 payoff (2 RP live at round
	// 15, a shot at sector majority) is funded incrementally rather than
	// gambled on a single large purchase competing with contract cash
	// flow at the moment the chokepoint first qualifies.
	const blocks = 1
	cost := blocks * cfg.LeaseCostPerBlock
	if v.You.Balance-cost < runnerReserve(cfg) {
		return game.AddOns{}, false
	}

	addOns := game.AddOns{RenewPost: node, RenewBlocks: blocks}
	trial := draft
	trial.AddOns = addOns
	trial.Action = game.ActionOrder{Kind: game.ActionStakePost}
	if rules.Legal(v, trial, cfg) != nil {
		return game.AddOns{}, false
	}
	return addOns, true
}

// wantsLedger is RFC-001 §14.3's "uses the Ledger when a rival's band
// jumps," worked the only way it can be under issue #190's no-memory rule:
// not by comparing this round's v.Others[n].Band against a remembered
// prior value, but by scanning v.Anchors — the match-to-date public anchor
// log, always present in the view — for a high-tier EventDelivered by an
// opponent within BandJumpLookback rounds. A delivery at BandJumpMinTier or
// above (GDD §8.3's own payment jump: Cr$ 20-30 against Tier I/II's Cr$
// 8-14) is inferable evidence of a probable band change even though the
// change itself is never directly observed. GDD §5.1's own two ledger
// rules are enforced here rather than left to Legal: unaffordable, or the
// final round (the Syndicate "closes its books before the month ends").
func wantsLedger(v game.PlayerView, cfg game.Config, opts OperatorOptions) bool {
	if v.Round >= game.RoundNumber(cfg.Rounds) || cfg.LedgerCost > v.You.Balance {
		return false
	}

	for _, a := range v.Anchors {
		if a.Kind != game.EventDelivered || a.Actor == nil || a.Tier < opts.BandJumpMinTier {
			continue
		}
		delta := v.Round - a.Round
		if delta < 0 || delta > opts.BandJumpLookback {
			continue
		}
		if isOpponentSeat(v, *a.Actor) {
			return true
		}
	}
	return false
}

func isOpponentSeat(v game.PlayerView, seat game.SeatID) bool {
	for _, o := range v.Others {
		if o.Seat == seat {
			return true
		}
	}
	return false
}

// pickOperatorAction chooses among actions — always a non-empty,
// jointly-legal set already trialled against the whole draft order by
// legalActionsOrRelax (legalspace.go). Legality alone is not enough of a
// gate for Deliver and, especially, Pickup: legalAction (internal/rules/
// legal.go) deliberately leaves Pickup unconditional at any node type and
// with no held-contract check at all — Step 0's degradation rule names "a
// Pickup that loses a dropped crate" as an expected outcome, so a
// speculative Pickup is legal to submit almost everywhere. Reusing
// runner.go's own runnerCanDeliver/runnerCanPickup here is what keeps this
// from reading as "Pickup any time it happens to be legal," which would be
// every round. Priority: cash actions first (Deliver, then Pickup — the
// two ways the objective loop actually pays off), then StakePost only when
// this round's objective was a chokepoint (wantStake), then a
// threat-driven Deal, then a comfort-band-and-cooldown-driven Vanish, and
// Nothing last.
func pickOperatorAction(v game.PlayerView, actions []game.ActionOrder, end, objective game.NodeID, haveObjective, stakeFunded, wantsDeal bool, dealItem game.ItemID, wantsVanish bool) game.ActionOrder {
	if runnerCanDeliver(v, end) {
		if a, ok := findAction(actions, game.ActionDeliver); ok {
			return a
		}
	}
	if runnerCanPickup(v, end, objective, haveObjective) {
		if a, ok := findAction(actions, game.ActionPickup); ok {
			return a
		}
	}
	if stakeFunded {
		if a, ok := findAction(actions, game.ActionStakePost); ok {
			return a
		}
	}
	if wantsDeal {
		for _, a := range actions {
			if a.Kind == game.ActionDeal && a.Item == dealItem {
				return a
			}
		}
	}
	if wantsVanish {
		if a, ok := findAction(actions, game.ActionVanish); ok {
			return a
		}
	}
	return game.ActionOrder{Kind: game.ActionNothing}
}

func findAction(actions []game.ActionOrder, kind game.ActionKind) (game.ActionOrder, bool) {
	for _, a := range actions {
		if a.Kind == kind {
			return a, true
		}
	}
	return game.ActionOrder{}, false
}
