package telemetry

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestMatchAgainstHandComputedEmptyPopulations is
// TestMatchAgainstHandComputedFixture's complement: fixture_test.go's
// fixture exercises every row's populated branch, but never exercises what
// Rate's own doc comment promises for a match whose row-level population
// was legitimately empty — no incident ever live, no confrontation, no
// [C] card, nobody ever observed a node. This is a second, much smaller,
// entirely hand-authored MatchState built specifically to hit those
// branches: every field below is worked out from this data by hand, the
// same discipline as the main fixture.
func TestMatchAgainstHandComputedEmptyPopulations(t *testing.T) {
	cfg := game.Config{Rounds: 3}

	// Both seats: Infamy 5 (neither >= 9 nor <= 2), RP 0, Balance 0, no
	// posts, no contracts, no sight archive at all — FinalScore gives both
	// a Total of 0, so WinnerRPLeadOverLastPlace has no positive winner to
	// express a lead as a share of.
	s := rules.MatchState{
		Round: 3,
		Graph: rules.Graph{
			Nodes: []rules.Node{{ID: 0, Sector: game.SectorOldDocks}},
			// EventDeck and IncidentDeck are both nil: no round has a
			// live card of either kind, so every card-derived Rate
			// (rows 6, 14) has an empty population.
		},
		Players: []rules.Player{
			{Seat: 0, Infamy: 5},
			{Seat: 1, Infamy: 5},
		},
	}

	// Every submitted route is empty — GDD §22's "submitted routes"
	// (row 1) reads as non-empty routes only, so the denominator
	// population here is zero even though the log itself has entries.
	log := rules.OrderLog{
		1: {0: {}, 1: {}},
		2: {0: {}, 1: {}},
		3: {0: {}, 1: {}},
	}

	// One event, so the event-stream-is-empty check does not fire, and
	// nothing else: no confrontation, no incident hit, no loitering.
	events := []game.Event{
		{Kind: game.EventDelivered, Round: 1, Seat: 0},
	}

	got, err := Match(s, log, events, cfg)
	if err != nil {
		t.Fatalf("Match() error = %v, want nil", err)
	}

	want := MatchSummary{
		RoutesCancelledMidRoute:              Rate{},    // D43: no measurement, on every match, not only this one
		DeliveriesPerPlayer:                  1.0 / 2.0, // 1 EventDelivered / 2 players
		WinnerRPLeadOverLastPlace:            Rate{},    // winner's Total is 0, not > 0
		AnyPlayerReachedInfamy9:              false,
		AnyPlayerStayedAtInfamy2OrBelow:      false,
		SectorIncidentsHittingAPlayer:        Rate{}, // IncidentDeck is empty: no round is ever live
		LiveLeasesAtFinalScoring:             0.0,    // no Post anywhere
		ShareOfMapUnderSightFinalThird:       0.0,    // no seat ever had sight of anything
		ConfrontationsWonAgainstEvasiveLoser: Rate{}, // no confrontation this match
		ConfrontationsPerMatch:               0,
		PlayersEndingInFlaggedSector:         Rate{Value: 0, N: 6}, // 0 EventIncidentExposed / (2 players * 3 rounds)
		ConvergenceCardConfrontations:        Rate{},               // EventDeck is empty: no round is ever live
		RoundsFlaggedLoitering:               Rate{Value: 0, N: 6}, // 0 EventLoitering / (2 players * 3 rounds)
		HeatMapLowConfidenceEntries:          Rate{},               // no seat ever produced a Heat Map entry
		ConfrontationsInFinal3Rounds:         Rate{},               // no confrontation this match
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match() = %+v, want %+v", got, want)
	}
}
