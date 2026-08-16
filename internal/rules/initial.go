package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules/gen"
)

// initial is the setup half of state = fold(Resolve, initial(seed, cfg),
// orderLog) (RFC §7.1): it runs the generator, seats the players, and builds
// both card decks. Setup runs entirely on one *RNG bound to round 0 — the
// same convention TestGenerateWiredToRealRNG* already uses for generation —
// so every Setup draw shares one index stream, in the fixed order RFC §6.4
// requires: gen.layout (inside gen.Generate, D10) before deck.event before
// deck.incident.
//
// players is not read from cfg: game.Config carries no player-count field
// (see Config.Validate's own signature), matching how gen.Params and every
// other setup-time caller of Config already takes it as a sibling argument.
//
// initial performs no I/O and reads no clock — every value it produces is a
// pure function of (seed, cfg, players).
func initial(seed [32]byte, cfg game.Config, players int) (MatchState, error) {
	if err := cfg.Validate(players); err != nil {
		return MatchState{}, err
	}
	spec := cfg.MapByPlayers[players]

	rng := NewRNG(seed, 0)

	g, err := gen.Generate(genRand(rng), gen.Params{
		Nodes:       spec.Nodes,
		MinEdges:    spec.MinEdges,
		MaxEdges:    spec.MaxEdges,
		Players:     players,
		MaxAttempts: cfg.MaxGenAttempts,

		// GDD §6.1 constraint 7 needs Tier I's band to know which
		// Warehouses can originate the opening offer (D24). gen takes it
		// as a parameter rather than reading Config — which it must not
		// import — so it travels the same way MapByPlayers' own fields do.
		OpeningMinDistance: cfg.Contracts[0].MinDistance,
		OpeningMaxDistance: cfg.Contracts[0].MaxDistance,
	})
	if err != nil {
		return MatchState{}, err
	}

	graph := newGraph(g)

	// D11: under Suppress.Events / Suppress.Incidents, the corresponding
	// deck is never built at all — zero draws, not a drawn-and-discarded
	// deck (D3's RNG consumption table, ConsumptionTable in
	// rng_purpose.go, documents this per row). Everything downstream
	// (eventCardThisRound, incidentCardThisRound, projectHeadline)
	// already treats a nil deck as "nothing this round, ever" — the guard
	// belongs here, at the one place the deck is created, not sprinkled
	// through the rest of the package.
	if !cfg.Suppress.Events {
		graph.EventDeck = buildEventDeck(rng)
	}
	if !cfg.Suppress.Incidents {
		graph.IncidentDeck = buildIncidentDeck(rng)
	}

	return MatchState{
		Round:          0,
		Graph:          graph,
		Players:        seatPlayers(g, graph, cfg, players),
		UnstableSector: initialUnstableSector(rng, cfg),
	}, nil
}

// initialUnstableSector draws round 3's Unstable Sector (GDD §14.1, §14.3:
// "nothing in rounds 1-2") — the one incident-deck draw that happens at
// Setup rather than round by round, because round 3's Headline has to be
// announceable from the MatchState round 2's Resolve call returns, and
// nothing has run yet to produce that value any other way (see
// MatchState.UnstableSector's own doc, state.go). nil when the match has no
// round 3 at all (cfg.Rounds < 3) — Config.Validate already permits a
// shorter match for scenario configs (GDD §19.1), and incident() (issue
// #73, incidents.go) never fires without a live sector to check against.
//
// nil under Suppress.Incidents (D11) too, checked before the draw so it
// costs zero indices rather than a drawn-then-ignored one — the same
// subsystem as the two deck skips above, and the sector this would produce
// is what feeds projectHeadline's Unstable Sector line (fog.go), so
// suppressing the draw is what keeps that line silent for the rest of the
// match: buildIncidentContext (incidents.go) treats a nil
// s.UnstableSector as "nothing live," permanently, since incident() never
// runs to advance it without a live context.
func initialUnstableSector(rng *RNG, cfg game.Config) *game.Sector {
	if cfg.Suppress.Incidents || cfg.Rounds < 3 {
		return nil
	}
	sector := PartialFisherYates(rng, PurposeIncidentSector, slices.Clone(allSectors), 1)[0]
	return &sector
}

// newGraph converts gen's output into rules.Graph. Node.Name is left at its
// zero value: neither the GDD nor the RFC specifies how a node acquires its
// display name, and inventing one here would be a rules decision, not a
// setup-plumbing one.
func newGraph(g gen.Graph) Graph {
	nodes := make([]Node, len(g.Nodes))
	for i, n := range g.Nodes {
		nodes[i] = Node{
			ID:     n.ID,
			Type:   n.Type,
			Sector: n.Sector,
			X:      n.X,
			Y:      n.Y,
			Edges:  n.Edges,
		}
	}
	return Graph{Nodes: nodes}
}

// seatPlayers builds every seat's starting Player (GDD §5: starting
// balance; §6.1 constraint 5: starting positions already >= 4 apart, by
// construction of g). Every other per-seat field starts at its Go zero
// value, matching RFC §6.6's own framing of the eight cross-round counters
// as "end of the previous round" state — there is no previous round yet.
//
// Fog is seeded by D23's two composed mechanisms, run once per seat against
// graph (the same converted Graph MatchState.Graph holds, so distances()
// walks the identical topology this match will navigate — Bridge Down and
// Sinkhole cannot have happened yet at Setup, so the setup graph and the
// navigable graph coincide here):
//
//  1. Opening sight (GDD §7.2, applied once at round 0 with the starting
//     position standing in for "ending position"): FogInSight on the
//     starting node and every node adjacent to it.
//  2. Constraint 7 (GDD §6.1): FogKnown on every Warehouse within graph
//     distance <= 2 of the starting node that step 1 left FogHidden. At
//     least one such Warehouse has a Border inside Tier I's contract band
//     (D24), so the opening offer can always originate. Nothing already
//     FogInSight is downgraded. No tie-break is needed — every qualifying
//     Warehouse is seeded, not just the nearest.
//
// See docs/decisions/D23-starting-fog-seeding.md. g still supplies
// StartPositions; graph is what makes the distance and Warehouse-type checks
// possible without re-deriving them from gen's own node/edge shape.
func seatPlayers(g gen.Graph, graph Graph, cfg game.Config, players int) []Player {
	out := make([]Player, players)
	for i := range players {
		start := g.StartPositions[i]

		fog := make([]game.FogState, len(graph.Nodes))
		for j := range fog {
			fog[j] = game.FogHidden
		}
		fog[start] = game.FogInSight
		for _, n := range graph.Nodes[start].Edges {
			fog[n] = game.FogInSight
		}

		dist := graph.distances(start)
		for _, n := range graph.Nodes {
			if n.Type == game.NodeWarehouse && dist[n.ID] >= 0 && dist[n.ID] <= 2 && fog[n.ID] == game.FogHidden {
				fog[n.ID] = game.FogKnown
			}
		}

		out[i] = Player{
			Seat:        game.SeatID(i),
			Balance:     cfg.StartingBalance,
			Position:    start,
			LastEndNode: start,
			Fog:         fog,
		}
	}
	return out
}
