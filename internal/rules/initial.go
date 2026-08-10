package rules

import (
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
	})
	if err != nil {
		return MatchState{}, err
	}

	graph := newGraph(g)
	graph.EventDeck = buildEventDeck(rng)
	graph.IncidentDeck = buildIncidentDeck(rng)

	return MatchState{
		Round:   0,
		Graph:   graph,
		Players: seatPlayers(g, cfg, players),
	}, nil
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
// Fog starts all game.FogHidden for every seat (GDD §7.1's zero-information
// state) — never the zero game.FogState, which enums.go reserves as
// invalid. GDD §6.1 constraint 7 additionally promises that a player's
// starting node, and a nearby Warehouse, begin Known; seeding that promise
// is left open pending D23 (issue #115) rather than guessed here.
func seatPlayers(g gen.Graph, cfg game.Config, players int) []Player {
	out := make([]Player, players)
	for i := range players {
		fog := make([]game.FogState, len(g.Nodes))
		for j := range fog {
			fog[j] = game.FogHidden
		}

		start := g.StartPositions[i]
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
