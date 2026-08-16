package rules

import (
	"reflect"
	"slices"
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

// TestInitialFogSeededByD23 guards the game.FogState zero-value trap
// (enums.go reserves FogState's zero value as invalid, so every entry must
// be explicitly set, never left at a plain make([]game.FogState, n)'s zero)
// and asserts D23's exact shape: the starting node and its neighbours are
// FogInSight (GDD §7.2 applied at round 0), every Warehouse within graph
// distance <= 2 of the start is at least FogKnown, and every other node is
// still FogHidden.
func TestInitialFogSeededByD23(t *testing.T) {
	cfg := game.DefaultConfig()
	s, err := initial(testSeed(51), cfg, 3)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	for i, p := range s.Players {
		if len(p.Fog) != len(s.Graph.Nodes) {
			t.Fatalf("Players[%d]: len(Fog) = %d, want %d (one per node)", i, len(p.Fog), len(s.Graph.Nodes))
		}

		start := p.Position
		if p.Fog[start] != game.FogInSight {
			t.Errorf("Players[%d]: Fog[start=%d] = %v, want FogInSight", i, start, p.Fog[start])
		}

		inSight := map[game.NodeID]bool{start: true}
		for _, n := range s.Graph.Nodes[start].Edges {
			inSight[n] = true
			if p.Fog[n] != game.FogInSight {
				t.Errorf("Players[%d]: Fog[neighbour=%d] = %v, want FogInSight", i, n, p.Fog[n])
			}
		}

		dist := s.Graph.distances(start)
		for _, n := range s.Graph.Nodes {
			switch {
			case inSight[n.ID]:
				// already checked above
			case n.Type == game.NodeWarehouse && dist[n.ID] >= 0 && dist[n.ID] <= 2:
				if p.Fog[n.ID] < game.FogKnown {
					t.Errorf("Players[%d]: Fog[warehouse=%d, dist=%d] = %v, want >= FogKnown", i, n.ID, dist[n.ID], p.Fog[n.ID])
				}
			default:
				if p.Fog[n.ID] != game.FogHidden {
					t.Errorf("Players[%d]: Fog[%d] = %v, want FogHidden (not start, not a neighbour, not a Warehouse within 2)", i, n.ID, p.Fog[n.ID])
				}
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

// TestInitialUnstableSectorSkipsDrawUnderSuppressIncidents is D11's
// consequence for Suppress.Incidents, checked with direct RNG access: the
// round-3 Unstable Sector draw never happens — zero PurposeIncidentSector
// draws, not a drawn-and-discarded value — so nil is returned without
// touching rng at all (issue #158).
func TestInitialUnstableSectorSkipsDrawUnderSuppressIncidents(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Incidents = true
	rng := NewRNG(testSeed(62), 0)

	got := initialUnstableSector(rng, cfg)

	if got != nil {
		t.Errorf("initialUnstableSector() = %v, want nil under Suppress.Incidents", *got)
	}
	if consumed := rng.Consumed(PurposeIncidentSector); consumed != 0 {
		t.Errorf("Consumed(incident.sector) = %d, want 0 — the draw must be skipped, not drawn and discarded", consumed)
	}
}

// TestInitialSkipsEventDeckUnderSuppressEvents is D11's consequence for
// Suppress.Events: the event deck is never built — not drawn and
// discarded — so it costs zero deck.event.select/order draws, not 12 and
// 11 (issue #158). This is checked by divergence rather than a direct
// Consumed() read (initial() does not expose its internal RNG): under
// Suppress.Events, deck.incident.select/order and incident.sector are the
// very next real draws on that RNG, so their values are reproduced here on
// a reference RNG started from the same seed. If deck.event.* had
// consumed any draws before them — drawn and discarded rather than
// skipped — those later draws would land on different indices and the
// reference would diverge from initial()'s actual output.
func TestInitialSkipsEventDeckUnderSuppressEvents(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Events = true
	seed := testSeed(60)

	rng := NewRNG(seed, 0)
	if _, err := gen.Generate(genRand(rng), gen.Params{
		Nodes:              cfg.MapByPlayers[4].Nodes,
		MinEdges:           cfg.MapByPlayers[4].MinEdges,
		MaxEdges:           cfg.MapByPlayers[4].MaxEdges,
		Players:            4,
		MaxAttempts:        cfg.MaxGenAttempts,
		OpeningMinDistance: cfg.Contracts[0].MinDistance,
		OpeningMaxDistance: cfg.Contracts[0].MaxDistance,
	}); err != nil {
		t.Fatalf("reference gen.Generate() = %v", err)
	}
	wantIncidentDeck := buildIncidentDeck(rng)
	wantSector := initialUnstableSector(rng, cfg)

	s, err := initial(seed, cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	if s.Graph.EventDeck != nil {
		t.Errorf("EventDeck = %v, want nil (never built)", s.Graph.EventDeck)
	}
	if !slices.Equal(s.Graph.IncidentDeck, wantIncidentDeck) {
		t.Errorf("IncidentDeck = %v, want %v (the reference sequence run on the same seed, with deck.event.* consuming zero draws)", s.Graph.IncidentDeck, wantIncidentDeck)
	}
	if (s.UnstableSector == nil) != (wantSector == nil) || (s.UnstableSector != nil && *s.UnstableSector != *wantSector) {
		t.Errorf("UnstableSector = %v, want %v", s.UnstableSector, wantSector)
	}
}

// TestInitialSkipsIncidentDeckUnderSuppressIncidents is D11's consequence
// for Suppress.Incidents: the incident deck is never built. Round 0's RNG
// has no further consumer once the incident subsystem is skipped (nothing
// in initial() reads it afterward), so this checks the structural result
// — a nil deck rather than a populated one — with the zero-draws property
// itself covered by TestInitialUnstableSectorSkipsDrawUnderSuppressIncidents
// and the sibling flag's divergence proof above pinning the same guard
// shape (issue #158).
func TestInitialSkipsIncidentDeckUnderSuppressIncidents(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Incidents = true

	s, err := initial(testSeed(61), cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	if s.Graph.IncidentDeck != nil {
		t.Errorf("IncidentDeck = %v, want nil (never built)", s.Graph.IncidentDeck)
	}
	if s.UnstableSector != nil {
		t.Errorf("UnstableSector = %v, want nil (never drawn)", *s.UnstableSector)
	}
}

// TestInitialFollowsSetupPhaseOrder pins RFC §6.4's required draw order:
// gen.layout (inside gen.Generate, D10) before deck.event before
// deck.incident, all on the one *RNG Setup shares. It builds a reference
// deck pair by hand, in that exact sequence, and asserts initial()'s own
// output matches it card-for-card — checking initial() itself, not a
// reimplementation of it, so a bug that reorders the phases, spins up a
// second RNG partway through, or skips a deck build would show up as a
// mismatch here rather than passing silently (CodeRabbit, PR #116: an
// earlier version of this test only re-ran the sequence standalone and
// never called initial at all, so it could not have caught any of that).
func TestInitialFollowsSetupPhaseOrder(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(54)

	rng := NewRNG(seed, 0)
	g, err := gen.Generate(genRand(rng), gen.Params{
		Nodes:              cfg.MapByPlayers[4].Nodes,
		MinEdges:           cfg.MapByPlayers[4].MinEdges,
		MaxEdges:           cfg.MapByPlayers[4].MaxEdges,
		Players:            4,
		MaxAttempts:        cfg.MaxGenAttempts,
		OpeningMinDistance: cfg.Contracts[0].MinDistance,
		OpeningMaxDistance: cfg.Contracts[0].MaxDistance,
	})
	if err != nil {
		t.Fatalf("reference gen.Generate() = %v", err)
	}
	if rng.Consumed(PurposeGenLayout) == 0 {
		t.Fatal("Consumed(gen.layout) = 0, want > 0 after gen.Generate")
	}
	wantEventDeck := buildEventDeck(rng)
	wantIncidentDeck := buildIncidentDeck(rng)

	s, err := initial(seed, cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}
	if len(g.Nodes) != len(s.Graph.Nodes) {
		t.Fatalf("reference graph has %d nodes, initial() produced %d", len(g.Nodes), len(s.Graph.Nodes))
	}

	if !slices.Equal(s.Graph.EventDeck, wantEventDeck) {
		t.Errorf("initial()'s EventDeck = %v, want %v (the reference sequence run on the same seed)", s.Graph.EventDeck, wantEventDeck)
	}
	if !slices.Equal(s.Graph.IncidentDeck, wantIncidentDeck) {
		t.Errorf("initial()'s IncidentDeck = %v, want %v (the reference sequence run on the same seed)", s.Graph.IncidentDeck, wantIncidentDeck)
	}
}
