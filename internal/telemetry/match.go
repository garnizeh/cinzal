package telemetry

import (
	"errors"
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// Match computes GDD §22's per-match metric set. s is the match's final
// MatchState (including every seat's SeatArchive); log is every order
// submitted across the whole match, independent of what haltMovement later
// cleared in memory; events is the caller's own accumulation of every
// round's Resolve output. This is D34's fixed signature
// (docs/decisions/D34-telemetry-package-placement.md); the field shape of
// the returned MatchSummary is this package's own.
//
// # Fails closed, but not row by row
//
// The issue's own acceptance criterion is "Match returns an error, not a
// zero summary, when the event stream is empty, when the match did not
// reach cfg.Rounds, or when a denominator is zero" — read here as the
// structural shape of the input, not every row's own content: cfg.Rounds,
// the player count, and the node count are denominators every single row
// in this package divides by somewhere, directly or through another row's
// population; an OrderLog with no entries at all means row 1 has no
// possible numerator population either. A match failing any of those
// checks did not produce a real MatchState to begin with — "every
// threshold in §22 is satisfied by a match that never happened."
//
// A single row's own population being empty this match — no incident was
// ever live, no confrontation occurred, no [C] card fired — is a different
// thing entirely: a legitimate, if unlucky, outcome of one match's random
// content, not evidence the match never happened. Rate's own N == 0
// carries that instead, and D35's own pooling rule is explicit that this
// is expected: "a match with an empty denominator is excluded from that
// row's vector, and the exclusion count is reported beside it" — the
// caller's job, across many matches, not a reason for this one match's
// entire summary to fail.
func Match(s rules.MatchState, log rules.OrderLog, events []game.Event, cfg game.Config) (MatchSummary, error) {
	if len(events) == 0 {
		return MatchSummary{}, errors.New("telemetry: Match: the event stream is empty")
	}
	if cfg.Rounds <= 0 {
		return MatchSummary{}, errors.New("telemetry: Match: cfg.Rounds is zero")
	}
	if s.Round < game.RoundNumber(cfg.Rounds) {
		return MatchSummary{}, fmt.Errorf("telemetry: Match: match reached round %d, cfg.Rounds is %d — the match did not finish", s.Round, cfg.Rounds)
	}
	if len(s.Players) == 0 {
		return MatchSummary{}, errors.New("telemetry: Match: MatchState has no players")
	}
	if len(s.Graph.Nodes) == 0 {
		return MatchSummary{}, errors.New("telemetry: Match: MatchState's graph has no nodes")
	}
	if len(log) == 0 {
		return MatchSummary{}, errors.New("telemetry: Match: OrderLog has no entries — no order was ever submitted")
	}

	groups := groupConfrontations(events)

	summary := MatchSummary{
		RoutesCancelledMidRoute:              routesCancelledMidRoute(log, events),
		DeliveriesPerPlayer:                  deliveriesPerPlayer(s, events),
		WinnerRPLeadOverLastPlace:            winnerRPLeadOverLastPlace(s),
		AnyPlayerReachedInfamy9:              anyPlayerAtOrAbove(s, 9),
		AnyPlayerStayedAtInfamy2OrBelow:      anyPlayerAtOrBelow(s, 2),
		SectorIncidentsHittingAPlayer:        sectorIncidentsHittingAPlayer(s, cfg, events),
		LiveLeasesAtFinalScoring:             liveLeasesAtFinalScoring(s),
		ShareOfMapUnderSightFinalThird:       shareOfMapUnderSightFinalThird(s, cfg),
		ConfrontationsWonAgainstEvasiveLoser: confrontationsWonAgainstEvasiveLoser(groups),
		ConfrontationsPerMatch:               len(groups),
		PlayersEndingInFlaggedSector:         playersEndingInFlaggedSector(len(s.Players), cfg, events),
		ConvergenceCardConfrontations:        convergenceCardConfrontations(s, cfg, groups),
		RoundsFlaggedLoitering:               roundsFlaggedLoitering(len(s.Players), cfg, events),
		HeatMapLowConfidenceEntries:          heatMapLowConfidenceEntries(s),
		ConfrontationsInFinal3Rounds:         confrontationsInFinal3Rounds(cfg, groups),
	}

	return summary, nil
}

// countEvents returns how many events in events have the given kind.
func countEvents(events []game.Event, kind game.EventKind) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// routesCancelledMidRoute computes MatchSummary.RoutesCancelledMidRoute —
// see that field's own doc comment for the numerator/denominator
// definition. Ranges log's two map levels only to sum a count, the same
// provably order-independent aggregation posts.go's sectorMajorityWinner
// documents for its own map-adjacent code — unlike Resolve's own pipeline
// (RFC §6.3), nothing here decides a tie or writes state from iteration
// order, and TestMatchIsDeterministic checks the result holds regardless.
func routesCancelledMidRoute(log rules.OrderLog, events []game.Event) Rate {
	n := 0
	for _, round := range log {
		for _, order := range round {
			if len(order.Route) > 0 {
				n++
			}
		}
	}
	if n == 0 {
		return Rate{}
	}
	return Rate{Value: float64(countEvents(events, game.EventRouteHalted)) / float64(n), N: n}
}

// deliveriesPerPlayer computes MatchSummary.DeliveriesPerPlayer.
func deliveriesPerPlayer(s rules.MatchState, events []game.Event) float64 {
	return float64(countEvents(events, game.EventDelivered)) / float64(len(s.Players))
}

// anyPlayerAtOrAbove reports whether any seat's final Infamy is >= threshold
// — MatchSummary.AnyPlayerReachedInfamy9's computation.
func anyPlayerAtOrAbove(s rules.MatchState, threshold int) bool {
	for _, p := range s.Players {
		if p.Infamy >= threshold {
			return true
		}
	}
	return false
}

// anyPlayerAtOrBelow reports whether any seat's final Infamy is <=
// threshold — MatchSummary.AnyPlayerStayedAtInfamy2OrBelow's computation.
func anyPlayerAtOrBelow(s rules.MatchState, threshold int) bool {
	for _, p := range s.Players {
		if p.Infamy <= threshold {
			return true
		}
	}
	return false
}
