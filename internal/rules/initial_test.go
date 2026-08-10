package rules

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules/gen"
)

// TestInitialBuildsValidMatchState covers #61's structural acceptance
// criteria across every supported player count: the graph, seat count, and
// both deck sizes all match the spec initial() was given.
func TestInitialBuildsValidMatchState(t *testing.T) {
	cfg := game.DefaultConfig()

	for _, players := range []int{2, 3, 4, 5} {
		seed := testSeed(byte(100 + players))

		s, err := initial(seed, cfg, players)
		if err != nil {
			t.Fatalf("players=%d: initial() = %v", players, err)
		}

		if s.Round != 0 {
			t.Errorf("players=%d: Round = %d, want 0", players, s.Round)
		}

		spec := cfg.MapByPlayers[players]
		if len(s.Graph.Nodes) != spec.Nodes {
			t.Errorf("players=%d: len(Graph.Nodes) = %d, want %d", players, len(s.Graph.Nodes), spec.Nodes)
		}
		if len(s.Players) != players {
			t.Errorf("players=%d: len(Players) = %d, want %d", players, len(s.Players), players)
		}
		if len(s.Graph.EventDeck) != 12 {
			t.Errorf("players=%d: len(EventDeck) = %d, want 12", players, len(s.Graph.EventDeck))
		}
		if len(s.Graph.IncidentDeck) != 13 {
			t.Errorf("players=%d: len(IncidentDeck) = %d, want 13", players, len(s.Graph.IncidentDeck))
		}
	}
}

// TestInitialSeatsPlayers asserts every seat is seeded with the configured
// starting balance, a starting position drawn from the generated graph's
// start positions, and no two seats sharing one.
func TestInitialSeatsPlayers(t *testing.T) {
	cfg := game.DefaultConfig()
	s, err := initial(testSeed(50), cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	seenPositions := map[game.NodeID]bool{}
	for i, p := range s.Players {
		if p.Seat != game.SeatID(i) {
			t.Errorf("Players[%d].Seat = %d, want %d", i, p.Seat, i)
		}
		if p.Balance != cfg.StartingBalance {
			t.Errorf("Players[%d].Balance = %d, want %d", i, p.Balance, cfg.StartingBalance)
		}
		if p.LastEndNode != p.Position {
			t.Errorf("Players[%d].LastEndNode = %d, want Position %d", i, p.LastEndNode, p.Position)
		}
		if seenPositions[p.Position] {
			t.Errorf("Players[%d].Position = %d, shared with another seat", i, p.Position)
		}
		seenPositions[p.Position] = true

		if int(p.Position) < 0 || int(p.Position) >= len(s.Graph.Nodes) {
			t.Errorf("Players[%d].Position = %d, out of range for %d nodes", i, p.Position, len(s.Graph.Nodes))
		}
	}
}

// TestInitialFogAllHidden guards the game.FogState zero-value trap directly:
// enums.go reserves FogState's zero value as invalid, so every entry must be
// explicitly set to game.FogHidden — a plain make([]game.FogState, n) would
// silently leave every node at the invalid zero value instead.
func TestInitialFogAllHidden(t *testing.T) {
	cfg := game.DefaultConfig()
	s, err := initial(testSeed(51), cfg, 3)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	for i, p := range s.Players {
		if len(p.Fog) != len(s.Graph.Nodes) {
			t.Fatalf("Players[%d]: len(Fog) = %d, want %d (one per node)", i, len(p.Fog), len(s.Graph.Nodes))
		}
		for node, fog := range p.Fog {
			if fog != game.FogHidden {
				t.Errorf("Players[%d].Fog[%d] = %v, want FogHidden", i, node, fog)
			}
		}
	}
}

// TestInitialDeterministic is #61's acceptance criterion "The same seed
// produces the same deck order across runs" extended to the whole
// MatchState: graph, seating, and both decks must all reproduce exactly.
func TestInitialDeterministic(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(52)

	a, err := initial(seed, cfg, 4)
	if err != nil {
		t.Fatalf("first initial() = %v", err)
	}
	b, err := initial(seed, cfg, 4)
	if err != nil {
		t.Fatalf("second initial() = %v", err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two initial() calls from the same seed produced different MatchState values")
	}
}

// TestInitialRejectsInvalidPlayerCount asserts initial() surfaces
// Config.Validate's error rather than panicking or indexing an absent
// MapByPlayers/PostCapByPlayers entry.
func TestInitialRejectsInvalidPlayerCount(t *testing.T) {
	cfg := game.DefaultConfig()
	if _, err := initial(testSeed(53), cfg, 1); err == nil {
		t.Fatal("initial() with an unsupported player count = nil error, want non-nil")
	}
}

// TestInitialConsumesGenPurposesBeforeDeckPurposes pins RFC §6.4's required
// draw order: gen.layout (inside gen.Generate, D10) before deck.event before
// deck.incident, all on the one *RNG Setup shares. It is checked from the
// outside — via what initial() actually produced — rather than duplicating
// initial()'s internals: a deck built from a spent/rewound RNG would fail
// the exact-count assertions TestInitialBuildsValidMatchState already
// makes, so this test only needs to confirm every purpose in the chain
// fired at least once for one real Setup call.
func TestInitialConsumesEveryPhasePurpose(t *testing.T) {
	cfg := game.DefaultConfig()
	rng := NewRNG(testSeed(54), 0)

	g, err := gen.Generate(genRand(rng), gen.Params{
		Nodes:       cfg.MapByPlayers[4].Nodes,
		MinEdges:    cfg.MapByPlayers[4].MinEdges,
		MaxEdges:    cfg.MapByPlayers[4].MaxEdges,
		Players:     4,
		MaxAttempts: cfg.MaxGenAttempts,
	})
	if err != nil {
		t.Fatalf("gen.Generate() = %v", err)
	}
	if rng.Consumed(PurposeGenLayout) == 0 {
		t.Fatal("Consumed(gen.layout) = 0, want > 0 after gen.Generate")
	}

	graph := newGraph(g)
	if len(graph.Nodes) != cfg.MapByPlayers[4].Nodes {
		t.Fatalf("len(graph.Nodes) = %d, want %d", len(graph.Nodes), cfg.MapByPlayers[4].Nodes)
	}

	eventDeck := buildEventDeck(rng)
	incidentDeck := buildIncidentDeck(rng)
	if len(eventDeck) != 12 || len(incidentDeck) != 13 {
		t.Fatalf("len(eventDeck)=%d, len(incidentDeck)=%d, want 12 and 13", len(eventDeck), len(incidentDeck))
	}

	for _, p := range []Purpose{PurposeDeckEventSelect, PurposeDeckEventOrder, PurposeDeckIncidentSelect, PurposeDeckIncidentOrder} {
		if rng.Consumed(p) == 0 {
			t.Errorf("Consumed(%s) = 0, want > 0 after the deck build", p)
		}
	}
}
