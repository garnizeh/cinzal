package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// Fixed Cr$ payouts GDD §14.2 states as flat numbers rather than a
// Config-tunable dial (unlike GateFee, LedgerCost, ShakedownCost, ...) —
// none of these appears in game.Config, so a named constant here is the
// only home for each.
const (
	newBossPayout       = 10 // "The lowest-RP player takes Cr$ 10"
	bountyPayout        = 10 // "takes Cr$ 10 from the bank"
	deadRunnerPayout    = 12 // "deliverable... for Cr$ 12 and 3 RP"
	deadRunnerRP        = 3
	fenceWindfallPayout = 12 // "buys any cargo outright for Cr$ 12"

	// permitAuctionCostPerBlock is Permit Auction's discounted rate (GDD
	// §14.2: "Lease blocks cost Cr$ 2 instead of 3, next round only") —
	// not derived from cfg.LeaseCostPerBlock, which is itself the tunable
	// dial the card is discounting away from.
	permitAuctionCostPerBlock = 2
)

// leaseCostPerBlock returns this round's Cr$-per-lease-block rate: Permit
// Auction's flat discount (GDD §14.2) when live, cfg.LeaseCostPerBlock
// otherwise. Card text says "lease blocks," not "renewals" — so this is
// shared by both a fresh Stake Post's own block cost (resolveStakePost,
// actions.go) and an ordinary renewal's (resolveRenewal, addons.go),
// rather than discounting only one of the two places GDD §10.4's per-block
// rate is spent.
func leaseCostPerBlock(cfg game.Config, ctx globalEventContext) int {
	if ctx.live && ctx.permitAuctionActive {
		return permitAuctionCostPerBlock
	}
	return cfg.LeaseCostPerBlock
}

// globalEventContext is Resolve's round-start peek at Phase 6's card (issue
// #72, GDD §14.2), built once by buildGlobalEventContext before validate
// runs and threaded, read-only, through every pipeline step that needs to
// know this round's card before globalEvent's own Phase 6 call site pops
// it. Most of the 24 cards need no entry here at all — they resolve
// entirely inside globalEvent, using live MatchState alone. This struct
// exists only for the handful whose GDD text is "this round" but whose
// earliest consumer runs before Phase 6:
//
//   - curfewActive, gatesClosedActive, permitAuctionActive: no RNG draw
//     needed — the card's identity alone is the whole effect, so these are
//     a free re-read of eventCardThisRound's peek (the same non-consuming
//     shape Rain already uses, trail.go), reused at validate (Curfew),
//     resolveDeliveries (Gates Closed) and resolveAddons (Permit Auction).
//   - sealedBorders, shippingBoomNode, festivalNode: Dragnet, Shipping
//     Boom and Festival each need a genuine RNG draw to know *which*
//     target, and each target gates something an earlier pipeline step
//     consumes (Deliver legality, a Pickup's payment, writeTrail's own-
//     entry suppression) — so the draw itself moves here, consumed once,
//     at the point the card is first known to be live. globalEvent reuses
//     the already-drawn value rather than drawing again.
type globalEventContext struct {
	card EventCardID
	live bool

	curfewActive        bool
	gatesClosedActive   bool
	permitAuctionActive bool

	sealedBorders    []game.NodeID // Dragnet; nil unless card == EventDragnet
	shippingBoomNode game.NodeID   // meaningful only when card == EventShippingBoom
	festivalNode     game.NodeID   // meaningful only when card == EventFestival
}

// buildGlobalEventContext peeks s.Round's global event card (eventCardThisRound,
// trail.go — free, since the deck's order was fixed at initial(seed, cfg)
// and never reshuffled) and, for the three cards whose target an earlier
// pipeline step needs this same round, draws it now. See
// globalEventContext's own doc for why only these three cards draw early.
func buildGlobalEventContext(s MatchState, r *RNG) globalEventContext {
	card, live := eventCardThisRound(s.Round, s.Graph.EventDeck)
	ctx := globalEventContext{card: card, live: live}
	if !live {
		return ctx
	}

	switch card {
	case EventCurfew:
		ctx.curfewActive = true
	case EventGatesClosed:
		ctx.gatesClosedActive = true
	case EventPermitAuction:
		ctx.permitAuctionActive = true
	case EventDragnet:
		ctx.sealedBorders = drawDragnetBorders(s, r)
	case EventShippingBoom:
		ctx.shippingBoomNode = drawShippingBoomWarehouse(s, r)
	case EventFestival:
		ctx.festivalNode = drawFestivalNode(s, r)
	}
	return ctx
}

// drawDragnetBorders draws Dragnet's 2 sealed Borders (PurposeEventDragnet,
// D3: "min(2, len(candidates))", candidates every Border node, sorted
// NodeID — borderNodeIDs, borders.go, shared with two-player rotation).
func drawDragnetBorders(s MatchState, r *RNG) []game.NodeID {
	candidates := borderNodeIDs(s.Graph)
	return slices.Clone(PartialFisherYates(r, PurposeEventDragnet, candidates, 2))
}

// drawShippingBoomWarehouse draws Shipping Boom's one overloaded Warehouse
// (PurposeEventShippingBoom, D3: candidates every Warehouse node, sorted
// NodeID).
func drawShippingBoomWarehouse(s MatchState, r *RNG) game.NodeID {
	var candidates []game.NodeID
	for _, n := range s.Graph.Nodes {
		if n.Type == game.NodeWarehouse {
			candidates = append(candidates, n.ID)
		}
	}
	return PartialFisherYates(r, PurposeEventShippingBoom, candidates, 1)[0]
}

// drawFestivalNode draws Festival's one crowd-drawing node
// (PurposeEventFestival, D3: candidates every node, sorted NodeID).
func drawFestivalNode(s MatchState, r *RNG) game.NodeID {
	candidates := make([]game.NodeID, len(s.Graph.Nodes))
	for i, n := range s.Graph.Nodes {
		candidates[i] = n.ID
	}
	return PartialFisherYates(r, PurposeEventFestival, candidates, 1)[0]
}

// globalEvent is Resolve's Phase 6 (GDD §14.2, RFC §6.7): pops this round's
// event card (the same peek buildGlobalEventContext already performed,
// re-read rather than re-drawn, matching Rain's own precedent) and resolves
// whichever of the 24 cards it is. roundEvents is every event the pipeline
// has produced so far this round — Bounty's only consumer, to find which
// seat(s) defeated this round's target in an already-resolved confrontation
// (GDD §14.2: "Whoever defeats them... this round"). Curfew, Gates Closed,
// Permit Auction, Dragnet, Shipping Boom, Festival's own suppression and
// Rain were already fully applied earlier this same round via ctx or the
// existing rainActive peek (trail.go); nothing further to do for them here
// except, for Festival, the +1 Infamy award, which has no earlier consumer
// and is just as correct resolved here.
func globalEvent(s *MatchState, ctx globalEventContext, roundEvents []game.Event, r *RNG) []game.Event {
	if !ctx.live {
		return nil
	}

	switch ctx.card {
	case EventRaid:
		return resolveRaid(s)
	case EventCurfew, EventGatesClosed, EventPermitAuction, EventDragnet, EventShippingBoom, EventRain:
		return nil
	case EventPayrollDay:
		return resolvePayrollDay(s)
	case EventShiftChange:
		return resolveShiftChange(s)

	case EventCurrencySlide:
		return resolveCurrencySlide(s)
	case EventMarketSurge:
		return resolveMarketSurge(s)
	case EventFencesWindfall:
		return resolveFencesWindfall(s, r)

	case EventInformants:
		return resolveInformants(s)
	case EventAmnesty:
		return resolveAmnesty(s)
	case EventNewBoss:
		return resolveNewBoss(s, r)
	case EventOldFavour:
		return resolveOldFavour(s)
	case EventDeadRunner:
		return resolveDeadRunner(s, r)
	case EventBounty:
		return resolveBounty(s, roundEvents, r)

	case EventBlackout:
		return resolveBlackout(s)
	case EventBridgeDown:
		return resolveBridgeDown(s, r)
	case EventDockersStrike:
		return resolveDockersStrike(s)
	case EventFestival:
		return resolveFestivalAward(s, ctx.festivalNode)
	case EventScaffolding:
		return resolveScaffolding(s, r)
	case EventRetainer:
		return resolveRetainer(s)
	}
	return nil
}

// resolveRaid applies GDD §14.2's Raid: every seat at this round's highest
// live Infamy (bySeat order; RFC §6.5 — no fairness tie-break, since GDD
// hits every tied player, not one selected target) loses carried cargo,
// dropped at their own node exactly like a confrontation loss
// (dropForfeitCargo, confront.go — bound cargo keeps its origin/destination
// pair, a loose crate stays loose), and −2 Infamy.
func resolveRaid(s *MatchState) []game.Event {
	seats := bySeat(*s)
	highest := s.Players[seats[0]].Infamy
	for _, seat := range seats[1:] {
		if inf := s.Players[seat].Infamy; inf > highest {
			highest = inf
		}
	}

	for _, seat := range seats {
		if s.Players[seat].Infamy != highest {
			continue
		}
		p := &s.Players[seat]
		if p.Cargo != nil {
			dropForfeitCargo(s, seat, p.Position)
		}
		p.Infamy = ApplyInfamyDelta(p.Infamy, -2)
	}
	return nil
}

// resolvePayrollDay applies GDD §14.2's Payroll Day: every seat pays Cr$ 1
// per post held, routed through applyDebt (debt.go) like every other
// mandatory Cr$ obligation in this package — an unaffordable payroll bill
// runs GDD §13's ordinary Debt cascade, including a possible lease
// surrender, rather than a bespoke shortfall rule of its own.
func resolvePayrollDay(s *MatchState) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		owed := len(s.Players[seat].Posts)
		if _, event := applyDebt(s, seat, owed, s.Round); event != nil {
			events = append(events, *event)
		}
	}
	return events
}

// resolveShiftChange applies GDD §14.2's Shift Change: every seat −1
// Infamy. The card's other half — "no Pressure roll this round" — needs no
// code here: pressure() (issue #73, still a stub) can re-peek
// eventCardThisRound itself when it exists, the same non-consuming read
// this file already relies on for Curfew/Gates Closed/Permit Auction.
func resolveShiftChange(s *MatchState) []game.Event {
	for _, seat := range bySeat(*s) {
		p := &s.Players[seat]
		p.Infamy = ApplyInfamyDelta(p.Infamy, -1)
	}
	return nil
}

// resolveCurrencySlide applies GDD §14.2's Currency Slide: every seat loses
// 25% of their balance, rounded down — `bal - bal/4`, integer division,
// per RFC §6.3's "rounding is specified as 'down' everywhere it appears."
func resolveCurrencySlide(s *MatchState) []game.Event {
	for _, seat := range bySeat(*s) {
		p := &s.Players[seat]
		p.Balance -= p.Balance / 4
	}
	return nil
}

// resolveMarketSurge applies GDD §14.2's Market Surge: every seat's *next*
// delivery pays +50%, however many rounds away that is — sets
// Player.MarketSurgeActive, consumed and cleared by resolveOneDelivery
// (deliveries.go) the moment it applies.
func resolveMarketSurge(s *MatchState) []game.Event {
	for _, seat := range bySeat(*s) {
		s.Players[seat].MarketSurgeActive = true
	}
	return nil
}

// resolveFencesWindfall applies GDD §14.2's Fence's Windfall: draws one
// Black Market (PurposeEventFencesWindfall, D3: candidates every Black
// Market node, sorted NodeID) and opens its standing offer
// (Node.FenceWindfallActive — state.go), claimed later by
// resolveFenceWindfallClaim. "Announced publicly" — GDD §14.2 — so this
// fires the one Anchor-shaped kind this issue adds for it.
func resolveFencesWindfall(s *MatchState, r *RNG) []game.Event {
	var candidates []game.NodeID
	for _, n := range s.Graph.Nodes {
		if n.Type == game.NodeBlackMarket {
			candidates = append(candidates, n.ID)
		}
	}
	node := PartialFisherYates(r, PurposeEventFencesWindfall, candidates, 1)[0]
	s.Graph.Nodes[node].FenceWindfallActive = true
	return []game.Event{{Kind: game.EventFenceWindfallAnnounced, Round: s.Round, Node: node}}
}

// resolveFenceWindfallClaim claims Fence's Windfall's standing offer (GDD
// §14.2: "First arrival only") — called once per round, right after
// resolveActions, for whichever cargo-carrying seat ends this round at the
// flagged Black Market. Multiple simultaneous claimants are contested by
// the same fairness-tie-break chain New Boss's and Bounty's own target
// selection already reuses (resolveFairnessTie, ordering.go; D14 §5) — a
// new call site, not a new RNG consumer. No-op when no node is currently
// flagged or nobody eligible ended there this round; leaves the flag set
// for a future round in the latter case.
func resolveFenceWindfallClaim(s *MatchState, seats []game.SeatID, r *RNG) []game.Event {
	flagged, found := game.NodeID(0), false
	for _, n := range s.Graph.Nodes {
		if n.FenceWindfallActive {
			flagged, found = n.ID, true
			break
		}
	}
	if !found {
		return nil
	}

	var candidates []game.SeatID
	for _, seat := range seats {
		p := s.Players[seat]
		if p.Position == flagged && p.Cargo != nil {
			candidates = append(candidates, seat)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	winner := resolveFairnessTie(*s, candidates, r)
	p := &s.Players[winner]
	p.Balance += fenceWindfallPayout
	// GDD §14.2: "buys any cargo outright... no contract needed" — bound
	// cargo is eligible too, not only a loose crate (Dead Runner's own
	// framing). Selling it destroys the cargo the same way a delivery
	// consumes it, so the matching contract is discarded here exactly as
	// resolveOneDelivery discards a fulfilled one (deliveries.go) — left
	// standing, it could never be fulfilled again, and would still charge
	// its own deadline-miss penalty later at Upkeep: paying twice for one
	// lost crate.
	if p.Cargo.Bound {
		if idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == p.Cargo.Contract }); idx >= 0 {
			p.Contracts = slices.Delete(p.Contracts, idx, idx+1)
		}
	}
	p.Cargo = nil
	s.Graph.Nodes[flagged].FenceWindfallActive = false
	return nil
}

// resolveInformants applies GDD §14.2's Informants: every seat's current
// position, revealed to everyone (RFC §9.1 row 11) — the one card whose
// EventKind (game.EventInformants) already existed before this issue.
func resolveInformants(s *MatchState) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		events = append(events, game.Event{
			Kind: game.EventInformants, Round: s.Round, Seat: seat, Node: s.Players[seat].Position,
		})
	}
	return events
}

// resolveAmnesty applies GDD §14.2's Amnesty: every seat −3 Infamy, floor 0
// (ApplyInfamyDelta's own clamp).
func resolveAmnesty(s *MatchState) []game.Event {
	for _, seat := range bySeat(*s) {
		p := &s.Players[seat]
		p.Infamy = ApplyInfamyDelta(p.Infamy, -3)
	}
	return nil
}

// rpExtremeTarget returns the seat selected by GDD §14.2's New Boss
// (lowest=true) or Bounty (lowest=false) — the seat(s) tied at the extreme
// RP value, tie-broken by the fairness key (ascending Infamy, then
// Balance, then RP, then a seeded coin) exactly as D14 §5 decided for both
// cards: "the identical chain RFC §6.5 already defines... the same key New
// Boss uses one row up in the same table." s.Players is never empty in a
// real match (2-5 seats), so this never receives an empty candidate set.
func rpExtremeTarget(s MatchState, lowest bool, r *RNG) game.SeatID {
	seats := bySeat(s)
	extreme := s.Players[seats[0]].RP
	for _, seat := range seats[1:] {
		rp := s.Players[seat].RP
		if (lowest && rp < extreme) || (!lowest && rp > extreme) {
			extreme = rp
		}
	}

	var candidates []game.SeatID
	for _, seat := range seats {
		if s.Players[seat].RP == extreme {
			candidates = append(candidates, seat)
		}
	}
	return resolveFairnessTie(s, candidates, r)
}

// resolveNewBoss applies GDD §14.2's New Boss: the lowest-RP seat takes
// Cr$ 10 and +1 Infamy.
func resolveNewBoss(s *MatchState, r *RNG) []game.Event {
	target := rpExtremeTarget(*s, true, r)
	p := &s.Players[target]
	p.Balance += newBossPayout
	p.Infamy = ApplyInfamyDelta(p.Infamy, 1)
	return nil
}

// resolveOldFavour applies GDD §14.2's Old Favour: every seat's Contact
// Cooldown resets to due-immediately (LastOfferRound 0 — RoundsToNextOffer,
// contracts.go: "0 unambiguously means 'never offered'... reports due
// immediately"). Contract offer generation and acceptance both happen
// outside Resolve entirely (Phase 2, driven by a caller this package has no
// visibility into) — this is the whole of what "ignoring the Contact
// Cooldown" can mean from inside a pure Resolve call: the *next* Phase 2
// this seat sees generates a fresh offer regardless of how recently their
// last one was.
func resolveOldFavour(s *MatchState) []game.Event {
	for _, seat := range bySeat(*s) {
		s.Players[seat].LastOfferRound = 0
	}
	return nil
}

// resolveDeadRunner applies GDD §14.2's Dead Runner: draws one node
// (PurposeCrateNode, shared with the Spilled Load incident per D3's own
// table — candidates every node, sorted NodeID) and drops an unbound loose
// crate there (Cargo{Bound: false} — cargo.go's CanCollect already treats
// this as collectible by anyone, no contract required). "Announced
// publicly" — GDD §14.2 — so this fires the one Anchor-shaped kind this
// issue adds for it. The Cr$ 12/3 RP payout on delivery lives in
// deliveries.go's resolveOneDelivery, the only place an unbound cargo's
// contract-free payout can be computed.
func resolveDeadRunner(s *MatchState, r *RNG) []game.Event {
	candidates := make([]game.NodeID, len(s.Graph.Nodes))
	for i, n := range s.Graph.Nodes {
		candidates[i] = n.ID
	}
	node := PartialFisherYates(r, PurposeCrateNode, candidates, 1)[0]
	s.Graph.Cargo = append(s.Graph.Cargo, Cargo{Node: node, Bound: false})
	return []game.Event{{Kind: game.EventDeadRunnerCrate, Round: s.Round, Node: node}}
}

// resolveBounty applies GDD §14.2's Bounty: the highest-RP seat is marked;
// roundEvents — every event this round's pipeline has produced so far,
// including Phase 5's own confrontations — is scanned for
// game.EventConfrontation rows naming the target as Target (Seat is always
// the winner — decisiveEvents, confront.go), and each such winner takes
// Cr$ 10 from the bank. Per D14 §5, this scales with however many times
// the target is actually defeated this round, not capped to one.
func resolveBounty(s *MatchState, roundEvents []game.Event, r *RNG) []game.Event {
	target := rpExtremeTarget(*s, false, r)
	for _, e := range roundEvents {
		if e.Kind == game.EventConfrontation && e.Target == target {
			s.Players[e.Seat].Balance += bountyPayout
		}
	}
	return nil
}

// resolveBlackout applies GDD §14.2's Blackout: sets the next-round
// modifier consumed by writeTrail's own suppression (trail.go) and Legal's
// sight cap (via Project, #75, not this issue's concern) the round after
// this one.
func resolveBlackout(s *MatchState) []game.Event {
	s.NextRound.Blackout = true
	return nil
}

// resolveDockersStrike applies GDD §14.2's Dockers' Strike: sets the
// next-round modifier Legal's legalAction reads (legal.go) to reject a
// declared Pickup the round after this one.
func resolveDockersStrike(s *MatchState) []game.Event {
	s.NextRound.DockersStrike = true
	return nil
}

// resolveRetainer applies GDD §14.2's Retainer: sets the next-round
// modifier legalView reads (validate.go) to grant +2 steps, for players
// carrying no cargo, the round after this one.
func resolveRetainer(s *MatchState) []game.Event {
	s.NextRound.Retainer = true
	return nil
}

// resolveScaffolding applies GDD §14.2's Scaffolding: draws one sector
// (PurposeEventScaffolding, D3: candidates the four sectors, sorted
// SectorID) and sets it as the next-round modifier legalView reads
// (validate.go) to grant +1 step, for players inside that sector, the
// round after this one.
func resolveScaffolding(s *MatchState, r *RNG) []game.Event {
	candidates := []game.Sector{
		game.SectorOldDocks, game.SectorIronLow, game.SectorMistHeights, game.SectorNorthVale,
	}
	sector := PartialFisherYates(r, PurposeEventScaffolding, candidates, 1)[0]
	s.NextRound.Scaffolding = &sector
	return nil
}

// resolveFestivalAward applies the half of GDD §14.2's Festival with no
// earlier consumer: +1 Infamy for every seat ending this round at node —
// already drawn early by buildGlobalEventContext, since the card's other
// half ("leaves no trace") is writeTrail's own concern (trail.go) and needs
// the same node known before Phase 6.
func resolveFestivalAward(s *MatchState, node game.NodeID) []game.Event {
	for _, seat := range bySeat(*s) {
		if s.Players[seat].Position == node {
			p := &s.Players[seat]
			p.Infamy = ApplyInfamyDelta(p.Infamy, 1)
		}
	}
	return nil
}

// bridgeEdge is one navigable, undirected edge of Graph — Node.Edges is a
// directed adjacency list (each edge appears in both endpoints' lists), so
// navigableEdges deduplicates by keeping only the A < B direction, which
// also happens to already be D3's mandated candidate order:
// "(min(NodeID), max(NodeID))", since Graph.Nodes and each Node.Edges are
// both NodeID-ascending.
type bridgeEdge struct{ A, B game.NodeID }

func navigableEdges(g Graph) []bridgeEdge {
	var edges []bridgeEdge
	for _, n := range g.Nodes {
		for _, to := range n.Edges {
			if n.ID < to {
				edges = append(edges, bridgeEdge{n.ID, to})
			}
		}
	}
	return edges
}

// resolveBridgeDown applies GDD §14.2's Bridge Down: draws one navigable
// edge (PurposeEventBridgeDown, D3) and removes it from both endpoints'
// Node.Edges — permanent, and correct under refold for free, since it is
// an ordinary mutation of the same live adjacency list every route/distance
// computation in this package already reads (Node's own comment, state.go).
func resolveBridgeDown(s *MatchState, r *RNG) []game.Event {
	edges := navigableEdges(s.Graph)
	picked := PartialFisherYates(r, PurposeEventBridgeDown, edges, 1)[0]

	s.Graph.Nodes[picked.A].Edges = slices.DeleteFunc(s.Graph.Nodes[picked.A].Edges, func(n game.NodeID) bool { return n == picked.B })
	s.Graph.Nodes[picked.B].Edges = slices.DeleteFunc(s.Graph.Nodes[picked.B].Edges, func(n game.NodeID) bool { return n == picked.A })

	return nil
}
