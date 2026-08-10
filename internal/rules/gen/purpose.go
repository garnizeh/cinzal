package gen

// Purpose strings for every RNG consumer in this package (GDD §6.1;
// RFC-001 §6.4, completed here for graph generation the same way D03
// completed it for cards — docs/decisions/D03-rng-consumption-table.md).
// internal/rules/rng_purpose.go declares a typed rules.Purpose constant for
// each of these, by value, so the string can never drift between the two
// packages — see that file's gen* block and
// TestPurposeTableMatchesDeclaredConstants.
const (
	// PurposeSectorAssign is the full shuffle assigning every node to a
	// sector slot: exactly Params.Nodes-1 draws, always. D8 fixes sector
	// count at four and sizes are a deterministic function of Nodes alone
	// (see sectorSizes), so this cost never varies with the seed, only
	// with Nodes.
	PurposeSectorAssign = "gen.sectorassign"

	// PurposeSectorTree is every sector's internal spanning tree — GDD
	// §6.1 constraint 3's "internally connected", built once per sector,
	// sharing one purpose because all four runs are the identical
	// selection shape (see nodeSpanningTree).
	PurposeSectorTree = "gen.sectortree"

	// PurposeAdjacency is the one spanning tree over the four sectors
	// themselves, choosing which three of the six possible sector pairs
	// carry chokepoint edges (GDD §6.1 constraint 4). Cost is fixed at 5
	// draws, always — D8 fixes sector count at exactly four, so this never
	// varies with Nodes or the seed's later choices.
	PurposeAdjacency = "gen.adjacency"

	// PurposeChokepointCount is one draw per adjacent sector pair choosing
	// how many chokepoint edges it gets, 3 to 5 (GDD §6.1 constraint 4):
	// exactly 3 draws, one per pair PurposeAdjacency's spanning tree
	// produced.
	PurposeChokepointCount = "gen.chokepoint.count"

	// PurposeChokepointSelect is the full shuffle of each pair's candidate
	// cross-sector edges, walked in degree-preferring order to select that
	// pair's count. Cost per pair is exactly (candidates-1) draws, where
	// candidates is that pair's two sectors' node-count product.
	PurposeChokepointSelect = "gen.chokepoint.select"

	// PurposeEdgeCount is the single draw choosing this attempt's target
	// total edge count within [MinEdges, MaxEdges].
	PurposeEdgeCount = "gen.edgecount"

	// PurposeFillEdge is the full shuffle of every remaining valid
	// candidate edge (intra-sector, plus the three chokepoint pairs' own
	// unused candidates up to their 5-edge cap), walked in
	// degree-preferring order until the target edge count is reached.
	PurposeFillEdge = "gen.filledge"

	// PurposeStartSelect is the full shuffle of every node used to select
	// GDD §6.1 constraints 5 and 7's starting positions (mutual distance
	// >= 4, a Warehouse within 2 steps): exactly Params.Nodes-1 draws,
	// always.
	PurposeStartSelect = "gen.startselect"

	// PurposeTypeAssign is one draw per node, in ascending NodeID order,
	// choosing that node's GDD §6.2 type among whichever of
	// Warehouse/Border/Black Market/Alley still has quota remaining (D9)
	// and would not create a GDD §6.1 constraint 6 Warehouse-Border
	// adjacency with an already-placed neighbour. One such walk costs
	// exactly Params.Nodes draws; a walk can deadlock before every node is
	// placed, in which case up to typeAssignMaxTries further walks are
	// retried against the same topology, so total cost per graph attempt is
	// Params.Nodes * (walks tried), data-dependent but bounded — see
	// nodetype.go.
	PurposeTypeAssign = "gen.typeassign"

	// PurposeLayout is the layout pass's per-sector partial Fisher-Yates
	// over the fixed 9-cell quadrant lattice (D10): exactly one draw per
	// node in that sector, so exactly Params.Nodes draws overall, always —
	// D8 caps every sector at 8 nodes, comfortably under the 9-cell
	// lattice, so this never truncates or retries. See layout.go.
	PurposeLayout = "gen.layout"
)
