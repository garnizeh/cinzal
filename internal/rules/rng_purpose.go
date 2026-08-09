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
	PurposeContractOffer       Purpose = "contract.offer"
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
	{PurposeContractOffer, "3 per offering seat", "Phase 2", ""},
	{PurposeMarketStock, "3 per market refreshed", "Phase 3, every 2 rounds", ""},
	{PurposeConfrontD6, "1 per participant, per confrontation", "", "Not per confrontation"},
	{PurposeConfrontTiebreak, "1, only at the fourth level", "", "GDD §15"},
	{PurposePushbackHop, "1 per hop — a second hop if Evasive", "", "GDD §15; the case r1 missed"},
	{PurposePushonEdge, "1 per blind step", "", "GDD §9.1"},
	{PurposeScavengeD6, "1 per newly explored node", "", "Zero if the node was already Known"},
	{PurposePressureD6, "1 per Legend", "Phase 7", ""},
	{PurposeIncidentSector, "1", "Phase 1", "Drawn where it is announced"},
	{PurposeIncidentRelocate, "1 per affected player", "Phase 7", "Snatch Job relocation"},
	{PurposeCrateNode, "1", "", "Dead Runner, Spilled Load"},
	{PurposeItemTornMap, "exactly min(4, hidden)", "", "Partial Fisher-Yates, mandated"},
	{PurposeGenLayout, "exactly n per sector — total node count over the whole map", "rules/gen, Setup only", "After node-type assignment (D9), before deck shuffles"},
	{PurposeEventDragnet, "min(2, len(candidates))", "Phase 6", "Candidates: all Border nodes, sorted by NodeID"},
	{PurposeEventBridgeDown, "min(1, len(candidates))", "Phase 6", "Candidates: navigable edges, sorted by (min(NodeID), max(NodeID))"},
	{PurposeEventFestival, "1", "Phase 6", "Candidates: all nodes, sorted by NodeID"},
	{PurposeEventScaffolding, "1", "Phase 6", "Candidates: the four sectors, sorted by SectorID"},
	{PurposeEventShippingBoom, "1", "Phase 6", "Candidates: all Warehouse nodes, sorted by NodeID"},
	{PurposeEventFencesWindfall, "1", "Phase 6", "Candidates: all Black Market nodes, sorted by NodeID"},
	{PurposeIncidentSinkhole, "1", "Phase 7", "Candidates: nodes in the flagged sector, sorted by NodeID"},
	{PurposeIncidentRiot, "exactly n, one per eligible entry (n = 0 in a quiet round)", "Phase 7", "D04 — sight-gated trail entries only"},
	{PurposeDeckEventSelect, "12 (3 per category x 4 categories)", "Setup only", "Partial Fisher-Yates per category"},
	{PurposeDeckEventOrder, "11", "Setup only", "Full Fisher-Yates, n-1 draws for n=12"},
	{PurposeDeckIncidentSelect, "13 (9 of 11 hazards + 4 of 5 boons)", "Setup only", "Partial Fisher-Yates, two pools"},
	{PurposeDeckIncidentOrder, "12", "Setup only", "Full Fisher-Yates, n-1 draws for n=13"},
}
