package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

func TestExhaustedErrorMessage(t *testing.T) {
	err := &ExhaustedError{
		Attempts:   50,
		MostFailed: constraintStartPlacement,
		Failures:   map[string]int{constraintStartPlacement: 47, constraintDegree: 3},
	}

	msg := err.Error()
	for _, want := range []string{"50", constraintStartPlacement, "47"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ExhaustedError.Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestParamsValidate(t *testing.T) {
	base := Params{Nodes: 15, MinEdges: 21, MaxEdges: 23, Players: 2, MaxAttempts: 100, OpeningMinDistance: 3, OpeningMaxDistance: 4}

	if err := base.validate(); err != nil {
		t.Fatalf("validate() on a well-formed Params = %v, want nil", err)
	}

	maxNodes, _ := maxSupportedNodes()

	cases := map[string]Params{
		"too few nodes": withNodes(base, minSupportedNodes-1),

		// Without this bound the failure is not an error at all: sectorSizes
		// splits the count into oversized sectors and computeLayout panics
		// on the first one wider than D10's lattice (#239).
		"too many nodes":       withNodes(base, maxNodes+1),
		"min edges zero":       withEdges(base, 0, 5),
		"max less than min":    withEdges(base, 10, 5),
		"zero players":         withPlayers(base, 0),
		"negative players":     withPlayers(base, -1),
		"zero max attempts":    withMaxAttempts(base, 0),
		"negative max attempt": withMaxAttempts(base, -1),

		// An unset band is the case that matters most: a zero-valued
		// OpeningMinDistance/OpeningMaxDistance makes constraint 7
		// unsatisfiable, and without this check it would surface as an
		// ExhaustedError — a generation failure reported for a caller
		// error (D24).
		"unset opening band":         withOpeningDistances(base, 0, 0),
		"zero opening minimum":       withOpeningDistances(base, 0, 4),
		"negative opening minimum":   withOpeningDistances(base, -1, 4),
		"opening max less than min":  withOpeningDistances(base, 4, 3),
		"opening max unset, min set": withOpeningDistances(base, 3, 0),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if err := p.validate(); err == nil {
				t.Errorf("validate() = nil, want an error")
			}
		})
	}

	// The boundaries themselves must be accepted, not just values strictly
	// inside them.
	if err := withNodes(base, minSupportedNodes).validate(); err != nil {
		t.Errorf("validate() at exactly minSupportedNodes (%d) = %v, want nil", minSupportedNodes, err)
	}
	if err := withNodes(base, maxNodes).validate(); err != nil {
		t.Errorf("validate() at exactly maxSupportedNodes (%d) = %v, want nil", maxNodes, err)
	}
	// A single-distance band (min == max) is degenerate but well-formed —
	// validate() rejects an inverted band, not a narrow one.
	if err := withOpeningDistances(base, 3, 3).validate(); err != nil {
		t.Errorf("validate() with OpeningMinDistance == OpeningMaxDistance = %v, want nil", err)
	}
}

// TestSupportedNodeRangeMatchesSectorSizes ties validate()'s two Nodes
// bounds back to the helper that actually does the splitting: inside the
// accepted range every sector sectorSizes produces must be legal under both
// D8's [3, 8] range and D10's lattice, and one node past either end must
// produce a sector that is not. This is what keeps the bounds honest if D8's
// cap or the lattice size ever moves — the old ceiling was argued in doc
// comments and asserted only at the node counts other tests happened to
// enumerate (#239).
func TestSupportedNodeRangeMatchesSectorSizes(t *testing.T) {
	maxNodes, _ := maxSupportedNodes()
	perSector := maxNodes / len(sectorOrder)

	if perSector > len(latticeCells) {
		t.Fatalf("maxSupportedNodes() = %d allows %d nodes per sector, more than the %d-cell layout lattice (D10)", maxNodes, perSector, len(latticeCells))
	}
	if perSector > maxSectorNodes {
		t.Fatalf("maxSupportedNodes() = %d allows %d nodes per sector, more than D8's cap of %d", maxNodes, perSector, maxSectorNodes)
	}

	for n := minSupportedNodes; n <= maxNodes; n++ {
		for i, size := range sectorSizes(n) {
			if size < minSectorNodes || size > perSector {
				t.Errorf("sectorSizes(%d)[%d] = %d, outside [%d, %d] at an accepted node count", n, i, size, minSectorNodes, perSector)
			}
		}
	}

	if sizes := sectorSizes(minSupportedNodes - 1); slices.Min(sizes[:]) >= minSectorNodes {
		t.Errorf("sectorSizes(%d) = %v is entirely within D8's floor — minSupportedNodes (%d) rejects a node count that did not need rejecting", minSupportedNodes-1, sizes, minSupportedNodes)
	}
	if sizes := sectorSizes(maxNodes + 1); slices.Max(sizes[:]) <= perSector {
		t.Errorf("sectorSizes(%d) = %v is entirely within the per-sector cap of %d — maxSupportedNodes (%d) rejects a node count that did not need rejecting", maxNodes+1, sizes, perSector, maxNodes)
	}
}

// TestGenerateRejectsOversizedNodeCounts is the regression the ceiling
// exists for: every node count above it must come back as a returned error
// from validate(), never as a panic from three frames down. Before #239, 37
// nodes panicked inside partialShuffle with "invalid argument to IntN" —
// rand(PurposeLayout, 0) on a 10-node sector against a 9-cell lattice — and
// 33-36 generated graphs that silently violated D8's [3, 8] range.
func TestGenerateRejectsOversizedNodeCounts(t *testing.T) {
	maxNodes, _ := maxSupportedNodes()
	cfg := game.DefaultConfig()

	// Past the ceiling, through the first node count that used to panic
	// (one node per lattice cell in all four sectors, plus one) and beyond.
	for n := maxNodes + 1; n <= len(sectorOrder)*len(latticeCells)+4; n++ {
		p := withOpeningBand(Params{
			Nodes: n,
			// The shipped ~2.8-3.2 average-degree band, the same range
			// every MapByPlayers row falls into — a well-formed request in
			// every respect except its node count.
			MinEdges:    n * 14 / 10,
			MaxEdges:    n * 16 / 10,
			Players:     5,
			MaxAttempts: cfg.MaxGenAttempts,
		}, cfg)

		if _, err := Generate(newTestRand(1), p); err == nil {
			t.Errorf("Generate at Nodes=%d = nil error, want a validation error", n)
		}
	}
}
