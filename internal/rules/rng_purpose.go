package rules

// Purpose names one RNG consumer (RFC-001 §6.4). It is recorded in the
// debug trace on every draw so a divergent replay names which draw went
// wrong rather than only that one did (RFC-001 §15.3) — it is never an
// input to the draw's distribution.
//
// Purpose values are declared as named constants below, never spelled as
// string literals at call sites, so a misspelled purpose is a compile error
// (an undefined identifier) rather than a silently distinct trace label.
type Purpose string

// The RNG consumption table (RFC-001 §6.4), completed by D03 —
// docs/decisions/D03-rng-consumption-table.md, issue #41. Every constant
// below must appear as exactly one ConsumptionTable row: see
// TestPurposeTableMatchesDeclaredConstants, which parses this file's
// constant declarations and fails if one is missing a row, or a row names a
// Purpose no constant declares.
const (
	PurposeContractOfferTier   Purpose = "contract.offer.tier"
	PurposeContractOfferPick   Purpose = "contract.offer.pick"
	PurposeMarketStock         Purpose = "market.stock"
	PurposeConfrontD6          Purpose = "confront.d6"
	PurposeConfrontTiebreak    Purpose = "confront.tiebreak"
	PurposePushbackHop         Purpose = "pushback.hop"
	PurposePushonEdge          Purpose = "pushon.edge"
	PurposeScavengeD6          Purpose = "scavenge.d6"
	PurposePressureD6          Purpose = "pressure.d6"
	PurposeIncidentSector      Purpose = "incident.sector"
	PurposeIncidentRelocate    Purpose = "incident.relocate"
	PurposeCrateNode           Purpose = "crate.node"
	PurposeItemTornMap         Purpose = "item.tornmap"
	PurposeGenLayout           Purpose = "gen.layout"
	PurposeGenSectorAssign     Purpose = "gen.sectorassign"
	PurposeGenSectorTree       Purpose = "gen.sectortree"
	PurposeGenAdjacency        Purpose = "gen.adjacency"
	PurposeGenChokepointCount  Purpose = "gen.chokepoint.count"
	PurposeGenChokepointSelect Purpose = "gen.chokepoint.select"
	PurposeGenEdgeCount        Purpose = "gen.edgecount"
	PurposeGenFillEdge         Purpose = "gen.filledge"
	PurposeGenStartSelect      Purpose = "gen.startselect"
	PurposeGenTypeAssign       Purpose = "gen.typeassign"
	PurposeEventDragnet        Purpose = "event.dragnet"
	PurposeEventBridgeDown     Purpose = "event.bridgedown"
	PurposeEventFestival       Purpose = "event.festival"
	PurposeEventScaffolding    Purpose = "event.scaffolding"
	PurposeEventShippingBoom   Purpose = "event.shippingboom"
	PurposeEventFencesWindfall Purpose = "event.fenceswindfall"
	PurposeIncidentSinkhole    Purpose = "incident.sinkhole"
	PurposeIncidentRiot        Purpose = "incident.riot"
	PurposeDeckEventSelect     Purpose = "deck.event.select"
	PurposeDeckEventOrder      Purpose = "deck.event.order"
	PurposeDeckIncidentSelect  Purpose = "deck.incident.select"
	PurposeDeckIncidentOrder   Purpose = "deck.incident.order"
)

// PurposeRow documents one row of the RNG consumption table. Indices is
// prose, not a number: several rows are input-dependent (RFC-001 §6.4), and
// the formula — not a single count — is what a replay mismatch needs to
// check itself against.
type PurposeRow struct {
	Purpose Purpose
	Indices string
	Phase   string
	Notes   string
}

// ConsumptionTable is the RNG consumption table in full: RFC-001 §6.4's
// original rows, plus D03's completion. Rotating borders (D03) is
// deliberately absent — it costs zero draws and has no purpose string, so
// it is not an RNG consumer at all. This is the source issue #77's property
// test reads to predict, per round, how many draws each Purpose should have
// consumed.
var ConsumptionTable = []PurposeRow{
	{PurposeContractOfferTier, "2 per offering seat, always", "Phase 2", "D6: slot 2 and 3's target tier — a weighted, independent draw over the eligible tier set, drawn even when only one tier is eligible"},
	{PurposeContractOfferPick, "0-3 per offering seat — one draw per filled slot", "Phase 2", "D6/D7: filled is 0-3, per-slot cascade pick; slot 1 (the guaranteed highest-eligible-tier slot) cascades first, so it gets first claim on the shared, without-replacement candidate pool"},
	{PurposeMarketStock, "3 per market refreshed", "Phase 3, odd rounds only (1, 3, ..., 15)", "Distinct items, partial Fisher-Yates (D25)"},
	{PurposeConfrontD6, "1 per participant, per confrontation", "", "Not per confrontation"},
	{PurposeConfrontTiebreak, "1, only at the fourth level", "", "GDD §15"},
	{PurposePushbackHop, "1 per hop — a second hop if Evasive", "", "GDD §15; the case r1 missed"},
	{PurposePushonEdge, "1 per blind step", "", "GDD §9.1"},
	{PurposeScavengeD6, "1 per newly explored node", "", "Zero if the node was already Known"},
	{PurposePressureD6, "1 per Legend", "Phase 7", ""},
	{PurposeIncidentSector, "1, 0 under Suppress.Incidents or Rounds < 3", "Phase 1", "Drawn where it is announced; skipped entirely, not drawn and discarded, under Config.Suppress.Incidents (D11) or when the match has no round 3 to announce it for — initialUnstableSector never draws in either case"},
	{PurposeIncidentRelocate, "1 per affected player", "Phase 7", "Snatch Job relocation"},
	{PurposeCrateNode, "1", "", "Dead Runner, Spilled Load"},
	{PurposeItemTornMap, "exactly min(4, hidden)", "", "Partial Fisher-Yates, mandated"},
	{PurposeGenLayout, "exactly n per sector — total node count over the whole map", "rules/gen, Setup only", "Partial Fisher-Yates over each sector's fixed 9-cell quadrant lattice (D10); after node-type assignment and start selection, before deck shuffles; issue #60"},
	{PurposeGenSectorAssign, "exactly Nodes-1, always", "rules/gen, Setup only", "Full shuffle assigning every node to a sector slot (GDD §6.1 constraint 3); issue #59"},
	{PurposeGenSectorTree, "2*size-3 per sector, summed over the four sectors", "rules/gen, Setup only", "Per-sector random spanning tree for internal connectivity (GDD §6.1 constraint 3); issue #59"},
	{PurposeGenAdjacency, "exactly 5, always", "rules/gen, Setup only", "Spanning tree over the four sectors choosing which 3 pairs carry chokepoints (D8 fixes sector count at four); issue #59"},
	{PurposeGenChokepointCount, "exactly 3, one per adjacent sector pair", "rules/gen, Setup only", "Chokepoint edge count per pair, 3-5 (GDD §6.1 constraint 4); issue #59"},
	{PurposeGenChokepointSelect, "candidates-1 per pair, summed over 3 pairs", "rules/gen, Setup only", "Full shuffle of each pair's cross-sector candidates, degree-tiered selection; issue #59"},
	{PurposeGenEdgeCount, "exactly 1, always", "rules/gen, Setup only", "Target total edge count within [MinEdges, MaxEdges]; issue #59"},
	{PurposeGenFillEdge, "remaining-candidates-1", "rules/gen, Setup only", "Full shuffle of every remaining valid edge, degree-tiered selection to reach the target; issue #59"},
	{PurposeGenStartSelect, "exactly Nodes-1, always", "rules/gen, Setup only", "Full shuffle of every node to select starting positions >= 4 apart, each within 2 steps of a Warehouse that itself has a Border inside Tier I's contract band (GDD §6.1 constraints 5 and 7, as strengthened by D24); issue #60"},
	{PurposeGenTypeAssign, "up to Nodes per walk (fewer on a walk that deadlocks early), summed over however many walks a graph attempt needed (bounded at 200)", "rules/gen, Setup only", "Per-node type choice satisfying D9's counts and GDD §6.1 constraint 6 (no Warehouse adjacent to a Border); retries a fresh walk against the same topology on a deadlock, before the caller discards the whole graph; issue #60"},
	{PurposeEventDragnet, "min(2, len(candidates))", "Phase 6", "Candidates: all Border nodes, sorted by NodeID. Issue #72: drawn at Resolve's round-start peek (buildGlobalEventContext, events.go), before validate — not at globalEvent's own Phase 6 call site — because Deliver's Step 0 legality needs the sealed set this same round. \"Phase 6\" here means announced, not drawn; the draw's position in the round's seq sequence is unchanged from where it would otherwise have landed."},
	{PurposeEventBridgeDown, "min(1, len(candidates))", "Phase 6", "Candidates: navigable edges, sorted by (min(NodeID), max(NodeID))"},
	{PurposeEventFestival, "1", "Phase 6", "Candidates: all nodes, sorted by NodeID. Issue #72: drawn at Resolve's round-start peek (buildGlobalEventContext, events.go), before validate — writeTrail's own-entry suppression needs the node this same round. \"Phase 6\" here means announced, not drawn."},
	{PurposeEventScaffolding, "1", "Phase 6", "Candidates: the four sectors, sorted by SectorID"},
	{PurposeEventShippingBoom, "1", "Phase 6", "Candidates: all Warehouse nodes, sorted by NodeID. Issue #72: drawn at Resolve's round-start peek (buildGlobalEventContext, events.go), before validate — a Pickup's own +Cr$5 bonus (resolveActions) needs the node this same round. \"Phase 6\" here means announced, not drawn."},
	{PurposeEventFencesWindfall, "1", "Phase 6", "Candidates: all Black Market nodes, sorted by NodeID"},
	{PurposeIncidentSinkhole, "1", "Phase 7", "Candidates: nodes in the flagged sector, sorted by NodeID"},
	{PurposeIncidentRiot, "exactly n, one per eligible entry (n = 0 in a quiet round)", "Phase 7", "D04 — sight-gated trail entries only"},
	{PurposeDeckEventSelect, "12 (3 per category x 4 categories), 0 under Suppress.Events", "Setup only", "Partial Fisher-Yates per category; the event deck is never built, not drawn and discarded, under Config.Suppress.Events (D11)"},
	{PurposeDeckEventOrder, "11, 0 under Suppress.Events", "Setup only", "Full Fisher-Yates, n-1 draws for n=12; skipped alongside PurposeDeckEventSelect under Config.Suppress.Events (D11)"},
	{PurposeDeckIncidentSelect, "13 (9 of 11 hazards + 4 of 5 boons), 0 under Suppress.Incidents", "Setup only", "Partial Fisher-Yates, two pools; the incident deck is never built, not drawn and discarded, under Config.Suppress.Incidents (D11)"},
	{PurposeDeckIncidentOrder, "12, 0 under Suppress.Incidents", "Setup only", "Full Fisher-Yates, n-1 draws for n=13; skipped alongside PurposeDeckIncidentSelect under Config.Suppress.Incidents (D11)"},
}
