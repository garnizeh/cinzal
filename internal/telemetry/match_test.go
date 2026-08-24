package telemetry

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestMatchAgainstHandComputedFixture is #197's own acceptance criterion:
// every field is tested against a value worked out by reading
// fixture_test.go's data by hand, not by recording what Match happens to
// return. Every want below is commented with the arithmetic it comes from;
// fixture_test.go's own comments show where each input number comes from.
func TestMatchAgainstHandComputedFixture(t *testing.T) {
	s := fixtureState()
	log := fixtureOrderLog()
	events := fixtureEvents()
	cfg := fixtureConfig()

	got, err := Match(s, log, events, cfg)
	if err != nil {
		t.Fatalf("Match() error = %v, want nil", err)
	}

	want := MatchSummary{
		// Row 1: #267 — haltOrConvertMovement (confront.go, D39) always
		// either converts a halt's unspent budget into further blind
		// Pushing On steps or has nothing left to convert, so no
		// EventRouteHalted signals a genuine mid-route cancellation any
		// more; see routesCancelledMidRoute's own doc comment. 0 / 15
		// non-empty submitted routes.
		RoutesCancelledMidRoute: Rate{Value: 0, N: 15},

		// Row 2: 5 EventDelivered / 3 players.
		DeliveriesPerPlayer: 5.0 / 3.0,

		// Row 3: (27 - 3) / 27 — seat 0's Total 27, seat 2's Total 3.
		WinnerRPLeadOverLastPlace: Rate{Value: 24.0 / 27.0, N: 1},

		// Row 4: seat 0's final Infamy is 9.
		AnyPlayerReachedInfamy9: true,

		// Row 5: seat 1's final Infamy is 2.
		AnyPlayerStayedAtInfamy2OrBelow: true,

		// Row 6: EventIncidentHit fires in rounds 3 and 5, of 3 live
		// incident rounds (3, 4, 5 — IncidentDeck has length 3).
		SectorIncidentsHittingAPlayer: Rate{Value: 2.0 / 3.0, N: 3},

		// Row 7: 3 live posts (nodes 0, 1, 2) / 3 players.
		LiveLeasesAtFinalScoring: 1.0,

		// Row 8: seat shares 2/4, 1/4, 2/4 — mean 1.25/3.
		ShareOfMapUnderSightFinalThird: 1.25 / 3.0,

		// Row 9: groups (2,1) and (3,3) each have a Decisive win over an
		// Evasive Target, of 4 confrontation groups total.
		ConfrontationsWonAgainstEvasiveLoser: Rate{Value: 2.0 / 4.0, N: 4},

		// Rows 10/11: 4 distinct (Round, Node) confrontation groups —
		// (2,1), (3,3), (4,2), (5,0).
		ConfrontationsPerMatch: 4,

		// Row 12: 5 EventIncidentExposed / (3 players * 6 rounds).
		PlayersEndingInFlaggedSector: Rate{Value: 5.0 / 18.0, N: 18},

		// Row 14: round 4's live event card (Dragnet) is Convergence and
		// had a confrontation; round 6's (Festival) is Convergence and
		// did not. 1 of 2 live Convergence rounds produced one.
		ConvergenceCardConfrontations: Rate{Value: 1.0 / 2.0, N: 2},

		// Row 17: 2 EventLoitering / (3 players * 6 rounds).
		RoundsFlaggedLoitering: Rate{Value: 2.0 / 18.0, N: 18},

		// Row 19: 6 low-confidence (seat, node) entries / 8 total entries.
		HeatMapLowConfidenceEntries: Rate{Value: 6.0 / 8.0, N: 8},

		// Row 20: groups (4,2) and (5,0) fall in the final 3 rounds
		// (>= cfg.Rounds-2 == 4), of 4 groups total.
		ConfrontationsInFinal3Rounds: Rate{Value: 2.0 / 4.0, N: 4},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match() = %+v, want %+v", got, want)
	}
}

// TestMatchSetsNoSoloFlag documents that Match never sets Solo itself —
// MatchSummary's own doc comment: the caller sets it, from information
// Match's own arguments do not carry (RFC §14.4).
func TestMatchSetsNoSoloFlag(t *testing.T) {
	got, err := Match(fixtureState(), fixtureOrderLog(), fixtureEvents(), fixtureConfig())
	if err != nil {
		t.Fatalf("Match() error = %v, want nil", err)
	}
	if got.Solo {
		t.Errorf("Match() set Solo = true; Match has no way to know this, and must leave it false for the caller to set")
	}
}

// TestMatchIsDeterministic calls Match twice against byte-identical input
// and requires byte-identical output — RFC §6.3's determinism discipline
// applies here exactly as it does to Resolve itself: nothing about this
// package's own map-keyed grouping (confrontations.go) may leak into the
// result.
func TestMatchIsDeterministic(t *testing.T) {
	s, log, events, cfg := fixtureState(), fixtureOrderLog(), fixtureEvents(), fixtureConfig()

	first, err := Match(s, log, events, cfg)
	if err != nil {
		t.Fatalf("Match() error = %v, want nil", err)
	}
	second, err := Match(s, log, events, cfg)
	if err != nil {
		t.Fatalf("Match() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Match() is not deterministic: first = %+v, second = %+v", first, second)
	}
}

// TestMatchFailsClosed is the issue's own acceptance criterion: Match
// returns an error, not a zero summary, for every structurally degenerate
// input — see Match's own doc comment for why these four, specifically,
// and not a single row's own empty population.
func TestMatchFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		state  rules.MatchState
		log    rules.OrderLog
		events []game.Event
		cfg    game.Config
	}{
		{
			name:   "empty event stream",
			state:  fixtureState(),
			log:    fixtureOrderLog(),
			events: nil,
			cfg:    fixtureConfig(),
		},
		{
			name:   "cfg.Rounds is zero",
			state:  fixtureState(),
			log:    fixtureOrderLog(),
			events: fixtureEvents(),
			cfg:    game.Config{Rounds: 0},
		},
		{
			name: "match did not reach cfg.Rounds",
			state: func() rules.MatchState {
				s := fixtureState()
				s.Round = 3
				return s
			}(),
			log:    fixtureOrderLog(),
			events: fixtureEvents(),
			cfg:    fixtureConfig(),
		},
		{
			name: "no players",
			state: func() rules.MatchState {
				s := fixtureState()
				s.Players = nil
				return s
			}(),
			log:    fixtureOrderLog(),
			events: fixtureEvents(),
			cfg:    fixtureConfig(),
		},
		{
			name: "no nodes",
			state: func() rules.MatchState {
				s := fixtureState()
				s.Graph.Nodes = nil
				return s
			}(),
			log:    fixtureOrderLog(),
			events: fixtureEvents(),
			cfg:    fixtureConfig(),
		},
		{
			name:   "empty order log",
			state:  fixtureState(),
			log:    nil,
			events: fixtureEvents(),
			cfg:    fixtureConfig(),
		},
		{
			// #263: nonEmptyRoutes ranged log with no bound of its own, so
			// a stray entry outside 1..cfg.Rounds inflated
			// RoutesCancelledMidRoute.N against cmd/simulate's own
			// per-round split, which was already bounded.
			name:  "OrderLog has a round outside 1..cfg.Rounds",
			state: fixtureState(),
			log: func() rules.OrderLog {
				log := fixtureOrderLog()
				log[7] = map[game.SeatID]game.Order{0: {Route: []game.NodeID{1}}}
				return log
			}(),
			events: fixtureEvents(),
			cfg:    fixtureConfig(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Match(tc.state, tc.log, tc.events, tc.cfg)
			if err == nil {
				t.Fatalf("Match() error = nil, want an error")
			}
			if !reflect.DeepEqual(got, MatchSummary{}) {
				t.Errorf("Match() = %+v on error, want the zero MatchSummary", got)
			}
		})
	}
}

// TestFinalThirdStart pins finalThirdStart's own arithmetic against a
// handful of round counts, independent of the fixture — the formula GDD
// §22's "final third" wording is read against.
func TestFinalThirdStart(t *testing.T) {
	tests := []struct {
		rounds int
		want   game.RoundNumber
	}{
		{rounds: 15, want: 11}, // GDD §4's default: final third is 11-15
		{rounds: 6, want: 5},   // this package's own fixture: final third is 5-6
		{rounds: 3, want: 3},   // final third is exactly round 3
		{rounds: 2, want: 3},   // degenerate: start exceeds rounds, no round qualifies
		{rounds: 1, want: 2},   // degenerate: start exceeds rounds, no round qualifies
	}
	for _, tc := range tests {
		if got := finalThirdStart(tc.rounds); got != tc.want {
			t.Errorf("finalThirdStart(%d) = %d, want %d", tc.rounds, got, tc.want)
		}
	}
}

// TestRoutesCancelledMidRouteDenominatorOnly is #267's own acceptance
// criterion: routesCancelledMidRoute no longer takes an events argument at
// all (see its own doc comment for why no EventRouteHalted this package can
// read still signals a genuine cancellation under haltOrConvertMovement,
// confront.go) — only the denominator, driven by OrderLog's own submitted
// non-empty routes, still varies.
func TestRoutesCancelledMidRouteDenominatorOnly(t *testing.T) {
	tests := []struct {
		name string
		log  rules.OrderLog
		want Rate
	}{
		{
			name: "no orders at all",
			log:  rules.OrderLog{},
			want: Rate{},
		},
		{
			name: "every submitted route is empty",
			log: rules.OrderLog{
				1: {0: game.Order{}, 1: game.Order{}},
			},
			want: Rate{},
		},
		{
			// GDD §15 evaluates every player's position after each step
			// "whether or not they moved" — a stationary seat that
			// submitted no route this round is still a confrontation
			// participant, but it is not a member of "submitted routes"
			// to begin with.
			name: "one non-empty route among several seats",
			log: rules.OrderLog{
				1: {
					0: game.Order{},                        // no route declared
					1: game.Order{Route: []game.NodeID{5}}, // the match's only submitted route
				},
			},
			want: Rate{Value: 0, N: 1},
		},
		{
			name: "non-empty routes across two rounds",
			log: rules.OrderLog{
				1: {0: game.Order{Route: []game.NodeID{5}}},
				2: {0: game.Order{Route: []game.NodeID{6}}, 1: game.Order{Route: []game.NodeID{7}}},
			},
			want: Rate{Value: 0, N: 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := routesCancelledMidRoute(tc.log); got != tc.want {
				t.Errorf("routesCancelledMidRoute() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
