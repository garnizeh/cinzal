package gen

// CanvasSize is D10's fixed canvas: every match, every player count, every
// node count (12-28), draws onto the same 1000x1000 square. The SVG viewBox
// this feeds (RFC §11.2) is the literal constant "0 0 1000 1000" — never a
// function of node count, player count, or fog state, so it discloses no
// information about map extent (docs/decisions/D10-map-layout.md).
const CanvasSize = 1000

// quadrantSize is half of CanvasSize: each of the four sectors owns one
// CanvasSize/2 x CanvasSize/2 quadrant (D10).
const quadrantSize = CanvasSize / 2

// point is one integer canvas coordinate — RFC §6.3 forbids float64 in
// rules, and layout is no exception (D10).
type point struct{ X, Y int }

// quadrantOrigins are the four quadrants' top-left corners, in reading
// order — top-left, top-right, bottom-left, bottom-right — index-aligned
// with sectorOrder: sectorOrder[i] always renders inside quadrantOrigins[i]
// (D10). The mapping is fixed, not seeded: no player-facing information
// depends on which corner of the screen a sector happens to occupy.
var quadrantOrigins = [4]point{
	{X: 0, Y: 0},
	{X: quadrantSize, Y: 0},
	{X: 0, Y: quadrantSize},
	{X: quadrantSize, Y: quadrantSize},
}

// latticeCells is the fixed 3x3 grid of candidate points inside one
// quadrant, margin 75 units from the quadrant's own edges and spaced 175
// units apart, in row-major order (D10). Its length is one of the two limits
// maxSupportedNodes takes the tighter of, so Params.validate rejects any
// node count whose sectors could outgrow it before generation starts — the
// lattice never runs short of a graph that got as far as being built.
var latticeCells = [9]point{
	{X: 75, Y: 75}, {X: 250, Y: 75}, {X: 425, Y: 75},
	{X: 75, Y: 250}, {X: 250, Y: 250}, {X: 425, Y: 250},
	{X: 75, Y: 425}, {X: 250, Y: 425}, {X: 425, Y: 425},
}

// computeLayout assigns every node in b a canvas coordinate under D10:
// sectorOrder's sectors fill quadrantOrigins in the same order, and within
// each sector a partial Fisher-Yates shuffle of that quadrant's fixed
// 9-cell lattice (PurposeLayout) picks which cell each of the sector's
// nodes — sorted ascending NodeID, the canonical order nodesInSector
// already returns — lands on. Cost is exactly one draw per node in that
// sector, so exactly b.n draws overall, always: no rejection loop and no
// data-dependent cost, because the lattice never runs short of a sector
// (maxSupportedNodes, enforced by Params.validate).
func computeLayout(rand Rand, b *builder) (x, y []int) {
	x = make([]int, b.n)
	y = make([]int, b.n)

	for qi, s := range sectorOrder {
		origin := quadrantOrigins[qi]
		nodes := b.nodesInSector(s)

		cells := latticeCells // array copy — never mutates the shared original
		partialShuffle(rand, PurposeLayout, cells[:], len(nodes))

		for i, node := range nodes {
			x[node] = origin.X + cells[i].X
			y[node] = origin.Y + cells[i].Y
		}
	}
	return x, y
}
