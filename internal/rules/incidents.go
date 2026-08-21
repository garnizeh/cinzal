package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// allSectors is the four sectors, ascending SectorID — the fixed candidate
// order D3 mandates for Scaffolding (events.go's resolveScaffolding) and
// this issue's own Unstable Sector draw (initial.go, nextUnstableSector
// below).
var allSectors = []game.Sector{
	game.SectorOldDocks, game.SectorIronLow, game.SectorMistHeights, game.SectorNorthVale,
}

// Fixed Cr$/RP payouts and losses GDD §14.3 states as flat numbers, the
// same convention events.go's own constant block already uses for GDD
// §14.2's flat figures.
const (
	snatchJobLoss  = 6 // "Lose Cr$ 6 and your cargo"
	guardSweepLoss = 5 // "Lose Cr$ 5 and −1 Infamy"
	shakedownCost  = 4 // "Every player ending here pays Cr$ 4"
	turfWarTarget  = 9 // "Roll D6 + your confrontation modifiers against a flat 9"

	spilledLoadPayout = 10 // "deliverable... for Cr$ 10 and 2 RP"
	spilledLoadRP     = 2
)

// incidentContext is Resolve's round-start peek at Phase 7's sector and card
// (GDD §14.3), mirroring globalEventContext (events.go): built once, before
// validate, and threaded read-only through every pipeline step that needs to
// know this round's incident before incident()'s own Phase 7 call site pops
// it — advance() (Gas Leak, movement.go) and writeTrail() (Riot, Distracted
// Guard's own-trace suppression, trail.go).
//
// Unlike globalEventContext, nothing here is drawn: s.UnstableSector was
// already decided a round ago (see MatchState.UnstableSector's own doc,
// state.go — GDD §14.1's Headline is announced before this round's orders
// are even collected), and incidentCardThisRound is the identical
// non-consuming peek eventCardThisRound (trail.go) already established for
// the event deck.
type incidentContext struct {
	sector *game.Sector
	card   IncidentCardID
	live   bool
}

// buildIncidentContext peeks s.Round's sector incident card against
// s.UnstableSector — the sector decided a round ago. live is false both
// before round 3 (s.UnstableSector is nil, or holds a future round's
// pre-drawn value that incidentCardThisRound correctly reports as not yet
// live) and once the match has resolved every incident round it was sized
// for.
func buildIncidentContext(s MatchState) incidentContext {
	if s.UnstableSector == nil {
		return incidentContext{}
	}
	card, ok := incidentCardThisRound(s.Round, s.Graph.IncidentDeck)
	if !ok {
		return incidentContext{}
	}
	return incidentContext{sector: s.UnstableSector, card: card, live: true}
}

// incidentCardThisRound returns the sector incident deck's card for round,
// and whether one is live at all — GDD §14.3: "From round 3 onward."
// buildIncidentDeck (cards.go) builds exactly those rounds' cards, in
// order, so round r's card is deck[r-3] — a peek, never a pop, the same
// shape eventCardThisRound (trail.go) already established for the event
// deck's deck[r-4].
func incidentCardThisRound(round game.RoundNumber, deck []IncidentCardID) (IncidentCardID, bool) {
	idx := int(round) - 3
	if idx < 0 || idx >= len(deck) {
		return 0, false
	}
	return deck[idx], true
}

// IncidentCardForRound is incidentCardThisRound for callers outside this
// package: given a match's own MatchState.Graph.IncidentDeck, which round
// r's card is, and whether one is live at all.
//
// It exists because the deck is never popped — Resolve peeks it — so "which
// card was live in round r" stays reconstructible from the final MatchState
// forever, which is what D33's seed-derivable-card-identity argument rests
// on. internal/telemetry's row-6 liveness and cmd/simulate's per-card
// breakdown both need exactly that, and both previously carried their own
// copy of the deck[r-3] arithmetic and its bounds rule. Three copies of a
// rule whose whole purpose is that two independent readers agree on it is
// one copy too many: telemetry's own comment argued the duplication was
// safe because the arithmetic is pure over exported fields, which is true
// and is also an argument for exporting it once.
func IncidentCardForRound(round game.RoundNumber, deck []IncidentCardID) (IncidentCardID, bool) {
	return incidentCardThisRound(round, deck)
}

// incidentEligible returns, in seats order, every seat whose live Position
// this round lies in sector and who did not declare Circulation Permit as a
// Field-4 discard this round (GDD §12: "immune to this round's Sector
// Incident") — the shared "ending in the sector" filter GDD §14.3's own
// framing states once for both hazards and boons ("Effect on players
// ending in the unstable sector"). Circulation Permit is declared blind,
// before the round's card is known (TimingArmed, items.go: "spent whether
// or not it fires"), so it blocks a boon exactly as symmetrically as a
// hazard — nothing in GDD §12's text carves out a one-way exception.
//
// Not used by Torched (sector-wide, not player-scoped), Sinkhole
// (node-wide), Gas Leak (its own immunity check lives in advance(),
// movement.go, since truncation happens during movement, before this
// function's live Position would even reflect the attempted entry), or
// Riot (D4 is explicit: "the generic 'ending in the sector' filter isn't
// reflexively applied to this one card too" — Riot's scope is entry-based).
//
// The second return value is one EventIncidentExposed per eligible seat —
// GDD §22's "players ending a route in a flagged unstable sector" (D33
// row 12), independent of what the incident then does to them (that's
// EventIncidentHit, row 6) — built here, once, so every caller shares the
// same source of truth for "ending in the sector" rather than each
// resolver re-deriving it.
func incidentEligible(s MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, sector game.Sector) ([]game.SeatID, []game.Event) {
	var eligible []game.SeatID
	var exposed []game.Event
	for _, seat := range seats {
		if s.Graph.Nodes[s.Players[seat].Position].Sector != sector {
			continue
		}
		if hasDiscard(validated[seat], game.ItemCirculationPermit) {
			continue
		}
		eligible = append(eligible, seat)
		exposed = append(exposed, game.Event{Kind: game.EventIncidentExposed, Round: s.Round, Node: s.Players[seat].Position, Seat: seat})
	}
	return eligible, exposed
}

// retreatTowardSectorEdge returns the lowest-NodeID neighbor of node whose
// own sector differs from sector and which is currently passable — Flood's
// and a losing Turf War's "retreat 1 step toward the sector edge" (GDD
// §14.3). No RNG purpose is reserved for this draw (D3's table has none),
// so — per D3's own "prefer deterministic when nothing forces randomness"
// precedent (Rotating Borders) — this is a plain, stable pick: Node.Edges
// is already NodeID-ascending (state.go), so the first qualifying neighbor
// found is already the lowest, the same tie-break convention every other
// unreserved selection in this package already uses. false when every
// neighbor is still inside sector or Sinkholed (a fully boxed-in node):
// nothing in GDD text forces an exit that doesn't exist, so the seat simply
// does not move.
func retreatTowardSectorEdge(g Graph, node game.NodeID, sector game.Sector) (game.NodeID, bool) {
	for _, n := range g.Nodes[node].Edges {
		if g.Nodes[n].Sector != sector && g.Nodes[n].SinkholeRounds == 0 {
			return n, true
		}
	}
	return 0, false
}

// nodesInSector returns every node whose Sector is sector, ascending
// NodeID (Graph.Nodes is already that order) — the candidate set Sinkhole
// and Spilled Load both draw from (D3).
func nodesInSector(g Graph, sector game.Sector) []game.NodeID {
	var out []game.NodeID
	for _, n := range g.Nodes {
		if n.Sector == sector {
			out = append(out, n.ID)
		}
	}
	return out
}

// nodesOutsideSector returns every node whose Sector is not sector,
// ascending NodeID — Snatch Job's relocation candidate set (GDD §14.3:
// "dumped at a random node in a different sector").
func nodesOutsideSector(g Graph, sector game.Sector) []game.NodeID {
	var out []game.NodeID
	for _, n := range g.Nodes {
		if n.Sector != sector {
			out = append(out, n.ID)
		}
	}
	return out
}

// incident is Resolve's Phase 7 sector-incident resolution (GDD §14.3, RFC
// §6.7): resolves whichever of the 16 cards ctx names against
// *ctx.sector — the sector decided a round ago, see
// MatchState.UnstableSector's own doc (state.go) — then advances
// s.UnstableSector to next round's draw. Gas Leak (movement.go, advance())
// and Riot (trail.go, writeTrail()) are already fully resolved by the time
// this runs — see incidentContext's own doc — so their cases here are
// no-ops; walks is threaded through only for Turf War's Shiv/Muscle reuse
// (turfWarTotal, below).
func incident(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) []game.Event {
	if !ctx.live {
		return nil
	}

	var events []game.Event
	switch ctx.card {
	case IncidentFlood:
		events = resolveFlood(s, ctx, validated, seats)
	case IncidentSnatchJob:
		events = resolveSnatchJob(s, ctx, validated, seats, r)
	case IncidentGuardSweep:
		events = resolveGuardSweep(s, ctx, validated, seats)
	case IncidentTorched:
		events = resolveTorched(s, *ctx.sector)
	case IncidentTurfWar:
		events = resolveTurfWar(s, ctx, validated, seats, walks, cfg, r)
	case IncidentStreetsBlocked:
		events = resolveStreetsBlocked(s, ctx, validated, seats)
	case IncidentGasLeak, IncidentRiot:
		// Already resolved this round — see doc above.
	case IncidentSinkhole:
		resolveSinkhole(s, *ctx.sector, r)
	case IncidentShakedown:
		events = resolveShakedown(s, ctx, validated, seats)
	case IncidentInformantRing:
		events = resolveInformantRing(s, ctx, validated, seats)
	case IncidentSpilledLoad:
		events = resolveSpilledLoad(s, *ctx.sector, r)
	case IncidentLocalInformant:
		events = resolveLocalInformant(s, ctx, validated, seats)
	case IncidentDistractedGuard:
		events = resolveDistractedGuard(s, ctx, validated, seats)
	case IncidentOpenDoors:
		events = resolveOpenDoors(s, ctx, validated, seats)
	case IncidentWordOfWork:
		events = resolveWordOfWork(s, ctx, validated, seats)
	}

	s.UnstableSector = nextUnstableSector(*s, cfg, r)

	return events
}

// nextUnstableSector draws the sector to flag for round s.Round+1 (GDD
// §14.3: "The same sector cannot be flagged two rounds running"),
// excluding whichever sector *s.UnstableSector already held entering this
// call. Only ever called when s.UnstableSector != nil (incident's own
// ctx.live guard). nil when there is no further round to announce it for.
func nextUnstableSector(s MatchState, cfg game.Config, r *RNG) *game.Sector {
	if int(s.Round) >= cfg.Rounds {
		return nil
	}
	candidates := make([]game.Sector, 0, len(allSectors)-1)
	for _, sector := range allSectors {
		if sector != *s.UnstableSector {
			candidates = append(candidates, sector)
		}
	}
	sector := PartialFisherYates(r, PurposeIncidentSector, candidates, 1)[0]
	return &sector
}

// resolveFlood applies GDD §14.3's Flood: "Carried cargo drops at your
// node. Retreat 1 step toward the sector edge." Emits EventIncidentHit for
// a seat iff cargo was actually dropped or the retreat actually succeeded
// (D33 row 6) — a boxed-in seat with no cargo is genuinely unaffected.
func resolveFlood(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		p := &s.Players[seat]
		origin := p.Position
		hadCargo := p.Cargo != nil
		if hadCargo {
			dropForfeitCargo(s, seat, p.Position)
		}
		retreated := false
		if dest, ok := retreatTowardSectorEdge(s.Graph, p.Position, *ctx.sector); ok {
			p.Position = dest
			markNodeKnown(p, dest) // GDD §7.2 / §15: Known however you got there.
			retreated = true
		}
		if hadCargo || retreated {
			events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: origin, Seat: seat})
		}
	}
	return events
}

// resolveSnatchJob applies GDD §14.3's Snatch Job: "Lose Cr$ 6 and your
// cargo (gone, not dropped). Dumped at a random node in a different
// sector." Cargo is destroyed outright — no append to Graph.Cargo,
// contrast Flood's genuine drop — and the seat's own Position is relocated
// (not the cargo: "gone, not dropped" already disposes of it), one
// PurposeIncidentRelocate draw per affected player, in seat order (RFC
// §6.5 names "Snatch Job relocation" directly as a seat-index batch).
// candidates is guarded before indexing even though nodesOutsideSector is
// safe by construction here (D8's floor guarantees every sector has >= 3
// nodes, so the pool outside any one flagged sector is never smaller than
// the other three sectors combined) — guarded anyway, for the same
// symmetric defense-in-depth the Sinkhole/Spilled Load guards below need
// for real (D3's own Sinkhole row is explicit that candidate-pool safety
// there is "conditional on generation," not fully proven). The debt event
// — a penalty that pushes a seat into Debt can surrender a lease — is
// captured and returned, matching resolveOneDelivery's own precedent
// (deliveries.go) for every other applyDebt call site in this package.
// applyDebt's own event only fires on that rarer lease-surrender case, not
// on the unconditional Cr$6-and-cargo hit GDD §14.3's text states as
// mandatory — so, unlike resolveInformantRing/resolveOpenDoors, every
// eligible seat here also gets its own EventIncidentHit (D33 row 6; #221
// review — D33's own audit missed that this resolver's "already emits an
// event" only covers the surrender edge case, not the ordinary hit).
func resolveSnatchJob(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID, r *RNG) []game.Event {
	candidates := nodesOutsideSector(s.Graph, *ctx.sector)
	if len(candidates) == 0 {
		return nil
	}

	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		p := &s.Players[seat]
		origin := p.Position
		if _, e := applyDebt(s, seat, snatchJobLoss, s.Round); e != nil {
			events = append(events, *e)
		}
		p.Cargo = nil
		p.Position = PartialFisherYates(r, PurposeIncidentRelocate, candidates, 1)[0]
		markNodeKnown(p, p.Position) // GDD §7.2 / §15: Known however you got there.
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: origin, Seat: seat})
	}
	return events
}

// resolveGuardSweep applies GDD §14.3's Guard Sweep: "Lose Cr$ 5 and −1
// Infamy. Carrying cargo, you lose it too." Cargo, if any, is destroyed —
// no GDD text says it lands anywhere — and the underlying Contract, if
// bound, is left untouched: the crate is gone, but the deal itself can
// still be re-sourced from its Warehouse. The debt event is captured and
// returned — see resolveSnatchJob's own doc — and, for the same reason
// given there, every eligible seat also gets its own EventIncidentHit
// (D33 row 6; #221 review).
func resolveGuardSweep(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		p := &s.Players[seat]
		if _, e := applyDebt(s, seat, guardSweepLoss, s.Round); e != nil {
			events = append(events, *e)
		}
		p.Infamy = ApplyInfamyDelta(p.Infamy, -1)
		p.Cargo = nil
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: p.Position, Seat: seat})
	}
	return events
}

// resolveTorched applies GDD §14.3's Torched: "Every post in the sector
// loses 3 rounds of lease, present or not." Sector-wide, not player-scoped
// (posts, not players present) — no eligibility filter, no Circulation
// Permit interaction (so, unlike every other row-6 resolver, it doesn't
// call incidentEligible — a Post's Owner needn't be standing in the sector
// at all). Per D14 §1 (decided): only ever decrements, never floors or
// closes a lease itself — Upkeep's own step 2 (issue #74) is the sole
// place a lease transitions to expired, its check already widened from
// "== 0" to "<= 0" in anticipation of this. Emits EventIncidentHit for
// every affected Post's Owner (D33 row 6).
func resolveTorched(s *MatchState, sector game.Sector) []game.Event {
	var events []game.Event
	for i := range s.Graph.Nodes {
		n := &s.Graph.Nodes[i]
		if n.Sector == sector && n.Post != nil {
			n.Post.RoundsRemaining -= 3
			events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: n.ID, Seat: n.Post.Owner})
		}
	}
	return events
}

// turfWarTotal computes Turf War's own TOTAL (GDD §14.3: "Roll D6 + your
// confrontation modifiers against a flat 9") — a documented, explicit
// choice: no D-doc or existing call site settles this, so this reuses
// confrontationTotal's (confront.go) per-participant terms — infamy tier,
// stance, Alley-aggressive, Shiv (via walk.ShivFired), Muscle, stake — but
// omits the "underestimated" (infamy == lowest) term, since Turf War has
// no opposing group to be relatively lowest against.
func turfWarTotal(s *MatchState, seat game.SeatID, node game.NodeID, o game.Order, walk *seatWalk, cfg game.Config, r *RNG) int {
	roll := r.Next(PurposeConfrontD6, 6) + 1
	total := roll + infamyTierIndex(s.Players[seat].Infamy, cfg) + stanceModifier(o.Stance.Stance)

	if o.Stance.Stance == game.StanceAggressive && s.Graph.Nodes[node].Type == game.NodeAlley {
		total++
	}
	if hasShivDiscard(o) && !walk.ShivFired {
		total += ShivBonus
		walk.ShivFired = true
	}
	if slices.Contains(s.Players[seat].Items, game.ItemMuscle) {
		total += MuscleBonus
	}
	total += min(o.Stance.Stake/3, 2)

	return total
}

// resolveTurfWar applies GDD §14.3's Turf War: each eligible seat rolls
// turfWarTotal against the flat turfWarTarget; total <= target loses (drop
// cargo, retreat 1 step toward the sector edge) — the package's existing
// "strictly greater wins, a tie goes to nobody" convention
// (confront.go's determineOutcome) applied against a fixed value instead
// of a rival's own total. Emits EventIncidentHit only for a seat that
// actually loses (D33 row 6: "a Turf War loss") — eligibility alone isn't
// enough, unlike the unconditional resolvers below.
func resolveTurfWar(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		p := &s.Players[seat]
		origin := p.Position
		total := turfWarTotal(s, seat, p.Position, validated[seat], walks[seat], cfg, r)
		if total > turfWarTarget {
			continue
		}
		if p.Cargo != nil {
			dropForfeitCargo(s, seat, p.Position)
		}
		if dest, ok := retreatTowardSectorEdge(s.Graph, p.Position, *ctx.sector); ok {
			p.Position = dest
			markNodeKnown(p, dest) // GDD §7.2 / §15: Known however you got there.
		}
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: origin, Seat: seat})
	}
	return events
}

// resolveStreetsBlocked applies GDD §14.3's Streets Blocked: "Your route
// next round is capped at 1 step" — sets Player.StreetsBlocked, consumed
// next round via SeatSnapshot/legalView/Steps (validate.go, steps.go).
// Every eligible seat is unconditionally affected, so EventIncidentHit
// fires for the whole eligible set (D33 row 6).
func resolveStreetsBlocked(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		s.Players[seat].StreetsBlocked = true
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: s.Players[seat].Position, Seat: seat})
	}
	return events
}

// resolveSinkhole applies GDD §14.3's Sinkhole: "One random node in the
// sector is impassable for 3 rounds" (PurposeIncidentSinkhole, D3:
// candidates nodes in the flagged sector, sorted NodeID). Node-wide, no
// eligibility filter. candidates is guarded before indexing: D3's own
// Sinkhole row inherits D8's candidate-pool argument only "conditional on
// generation" — D8's own tightest-config follow-up (a 12-node, four
// 3-node-sector map) is explicitly still open — so, unlike the whole-map
// pools every other card in this package draws from, an empty sector here
// is not yet a fully closed question. A no-op when it happens keeps
// PurposeIncidentSinkhole's draw lazy (RFC §6.4) rather than panicking
// Resolve for the whole table.
//
// Deliberately not a D33 row-6 EventIncidentHit source, despite being
// named in that row's resolver list: this card affects a node, not a
// player — no incidentEligible call, no player-position check at all at
// the moment it fires — so there is no seat to attribute a hit to. A
// player already standing on the chosen node isn't displaced or otherwise
// touched; only future movement through it is affected, which is a
// pathfinding fact (advance(), movement.go), not an incident-round event.
func resolveSinkhole(s *MatchState, sector game.Sector, r *RNG) {
	candidates := nodesInSector(s.Graph, sector)
	if len(candidates) == 0 {
		return
	}
	node := PartialFisherYates(r, PurposeIncidentSinkhole, candidates, 1)[0]
	s.Graph.Nodes[node].SinkholeRounds = 3
}

// resolveShakedown applies GDD §14.3's Shakedown: "Every player ending
// here pays Cr$ 4" — routed through applyDebt like every other mandatory
// Cr$ obligation in this package (resolvePayrollDay, events.go, states the
// same reasoning). The debt event is captured and returned — see
// resolveSnatchJob's own doc — and, for the same reason given there, every
// eligible seat also gets its own EventIncidentHit (D33 row 6; #221
// review).
func resolveShakedown(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		if _, e := applyDebt(s, seat, shakedownCost, s.Round); e != nil {
			events = append(events, *e)
		}
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: s.Players[seat].Position, Seat: seat})
	}
	return events
}

// resolveInformantRing applies GDD §14.3's Informant Ring: "Every player
// ending here has their position revealed publicly" — the same
// position-reveal shape as EventInformants (events.go's resolveInformants,
// RFC §9.1 row 11), but its own row, 15, per D26: a distinct Kind so
// recap/telemetry can attribute the reveal to the incident.
func resolveInformantRing(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		events = append(events, game.Event{
			Kind: game.EventInformantRing, Round: s.Round, Seat: seat, Node: s.Players[seat].Position,
		})
	}
	return events
}

// resolveSpilledLoad applies GDD §14.3's Spilled Load: draws one node in
// the flagged sector (PurposeCrateNode — shared with Dead Runner per D3's
// table; the candidate *set* differs, sector-scoped here vs whole-map
// there, which purpose never governs) and drops an unbound loose crate
// there, tagged SpilledLoad so resolveOneDelivery (deliveries.go) pays
// GDD §14.3's Cr$10/2RP rather than Dead Runner's Cr$12/3RP. "Announced
// publicly" — GDD §14.3 — so this fires an Anchor-shaped kind, the
// incident-deck twin of EventDeadRunnerCrate (RFC §9.1 row 13) — its own
// row, 16, per D26. candidates is guarded before
// indexing — no crate, no announcement, no draw — for the same reason
// resolveSinkhole's own doc gives: this candidate pool's safety is not yet
// fully closed (D3, D8).
func resolveSpilledLoad(s *MatchState, sector game.Sector, r *RNG) []game.Event {
	candidates := nodesInSector(s.Graph, sector)
	if len(candidates) == 0 {
		return nil
	}
	node := PartialFisherYates(r, PurposeCrateNode, candidates, 1)[0]
	s.Graph.Cargo = append(s.Graph.Cargo, Cargo{Node: node, Bound: false, SpilledLoad: true})
	return []game.Event{{Kind: game.EventSpilledLoadCrate, Round: s.Round, Node: node}}
}

// resolveLocalInformant applies GDD §14.3's Local Informant: "gains sight
// of every node within 2 steps of wherever they end their route next
// round" — sets Player.LocalInformant, consumed and cleared next round by
// seatSight/distributeTrail (trail.go). Every eligible seat is
// unconditionally affected, so EventIncidentHit fires for the whole
// eligible set (D33 row 6).
func resolveLocalInformant(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		s.Players[seat].LocalInformant = true
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: s.Players[seat].Position, Seat: seat})
	}
	return events
}

// resolveDistractedGuard applies the step half of GDD §14.3's Distracted
// Guard: "gains +1 step next round" — sets Player.DistractedGuard,
// consumed next round via SeatSnapshot/legalView/Steps. The card's other
// half, "leaves no trace this round," is resolved earlier the same round,
// inside writeTrail() (trail.go) — see incidentContext's own doc for why.
// Every eligible seat is unconditionally affected, so EventIncidentHit
// fires for the whole eligible set (D33 row 6).
func resolveDistractedGuard(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		s.Players[seat].DistractedGuard = true
		events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: s.Players[seat].Position, Seat: seat})
	}
	return events
}

// resolveOpenDoors applies GDD §14.3's Open Doors via D14 §4's mechanism: a
// pre-declared Black Market node and item (game.AddOns.OpenDoorsMarket/
// .OpenDoorsItem), re-checked live at resolution — degrading silently (no
// purchase, no event) when: nothing was declared; the declared node isn't
// currently a Black Market this seat has sight of; the declared item isn't
// in that node's current stock (an earlier seat's own Deal this same round
// may have already bought it); price exceeds balance; or the hand, after
// this round's own discards, is already at the cap. Price rounds down
// (RFC §6.3, D14 §4). The resulting EventItemPurchased is historical-record
// only — unlike an ordinary Deal it cannot enter anyone's sight-gated
// Archive.Trail, since writeTrail() (trail.go) already ran this same round,
// strictly before incident() — see incidentContext's own doc.
//
// OpenDoorsMarket is a client-declared NodeID that Legal accepts
// unconditionally at submission (D14 §4: "there is nothing to check
// against at submission time") — that unconditional acceptance is about
// semantic validity (whether the market still makes sense once Open Doors
// is known to have fired), not about basic bounds safety, so this function
// is the only remaining check before an out-of-range value would index
// Graph.Nodes. GDD §15.0 frames exactly this shape — a malformed field on
// an otherwise-legal order — as "the obvious attack surface," and Resolve
// processes every seat's order in one call, so one seat's out-of-range
// value must not panic resolution for the whole table.
func resolveOpenDoors(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		o := validated[seat]
		market := o.AddOns.OpenDoorsMarket
		if market == nil {
			continue
		}
		node := *market
		if node < 0 || int(node) >= len(s.Graph.Nodes) {
			continue
		}
		p := &s.Players[seat]

		if s.Graph.Nodes[node].Type != game.NodeBlackMarket || p.Fog[node] < game.FogInSight {
			continue
		}

		item := o.AddOns.OpenDoorsItem
		idx := slices.Index(s.Graph.Nodes[node].Market, item)
		if idx < 0 {
			continue
		}

		price := itemPrice(item) / 2
		if p.Balance < price || len(p.Items) >= handLimit {
			continue
		}

		s.Graph.Nodes[node].Market = slices.Delete(slices.Clone(s.Graph.Nodes[node].Market), idx, idx+1)
		p.Balance -= price
		p.Items = append(p.Items, item)

		events = append(events, game.Event{Kind: game.EventItemPurchased, Round: s.Round, Node: node, Seat: seat, Item: item})
	}
	return events
}

// resolveWordOfWork applies GDD §14.3's Word of Work: "immediately
// receives a contract offer, ignoring the Contact Cooldown" — identical
// mechanism to resolveOldFavour (events.go), scoped to the eligible
// subset instead of every seat. Not a D33 row-6 resolver — a contract
// offer is a boon, not a hit — so the only events here are the shared
// EventIncidentExposed batch from incidentEligible.
func resolveWordOfWork(s *MatchState, ctx incidentContext, validated map[game.SeatID]game.Order, seats []game.SeatID) []game.Event {
	eligible, events := incidentEligible(*s, validated, seats, *ctx.sector)
	for _, seat := range eligible {
		s.Players[seat].LastOfferRound = 0
	}
	return events
}

// pressure is Resolve's Phase 7 Legend-tier check (GDD §14.4) — unlike
// incident(), gated by nothing but Infamy: "every round, from round 1,"
// and independent of the sector incident deck entirely
// (game.PressureConfig's own doc: "There is no separate suppression flag
// for Pressure"). Re-peeks eventCardThisRound directly (trail.go's own
// non-consuming shape) rather than threading globalEventContext through:
// Shift Change's "no Pressure roll this round" (GDD §14.2) is the one card
// this function needs to know about, matching the exact re-peek precedent
// resolveShiftChange's own comment already names (events.go).
func pressure(s *MatchState, cfg game.Config, r *RNG) []game.Event {
	if card, ok := eventCardThisRound(s.Round, s.Graph.EventDeck); ok && card == EventShiftChange {
		return nil
	}

	var events []game.Event
	for _, seat := range bySeat(*s) {
		p := &s.Players[seat]
		// Suppress.InfamyTiers (D11) makes Legend unreachable through the
		// gated tier lookup — see SubsystemSuppression's own doc
		// (game/config.go): "Pressure is already fully suppressed as a
		// consequence." This is that consequence, checked directly since
		// TierOf itself stays cfg-free (fog.go's exposure functions still
		// call it ungated, D01/D27).
		if cfg.Suppress.InfamyTiers || TierOf(p.Infamy) != game.TierLegend {
			continue
		}
		if roll := r.Next(PurposePressureD6, 6) + 1; roll <= cfg.Pressure.Threshold {
			if _, e := applyDebt(s, seat, cfg.Pressure.CashPenalty, s.Round); e != nil {
				events = append(events, *e)
			}
			p.Infamy = ApplyInfamyDelta(p.Infamy, -cfg.Pressure.InfamyPenalty)
		}
	}
	return events
}
