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
// population. An OrderLog with no entries at all is the same shape of
// problem: no order was ever submitted, so the match this call describes
// did not happen, independent of what any single row does with the log —
// row 1 itself no longer has any population computed from it at all (D43,
// docs/decisions/D43-row-1-unmeasurable-post-d39.md). A match failing any
// of those checks did not produce a real MatchState to begin with — "every
// threshold in §22 is satisfied by a match that never happened."
//
// A fifth structural check joins those four: every OrderLog round key must
// fall within 1..cfg.Rounds. This is now a standing precondition on the
// input's own shape, not protection for any one row's population — the
// same division of labor RoundActions already applies to its own fold
// (round_action.go). It was added when row 1's own population counted
// every (round, seat) entry the caller's log happened to hold, so a log
// with a stray entry outside 1..cfg.Rounds silently inflated
// RoutesCancelledMidRoute.N while cmd/simulate's own per-round split —
// bounded from the start — did not (#263); cmd/simulate's Breakdown still
// indexes its own per-round vectors by this same bound (breakdown.go), so
// the check stays even though row 1 no longer reads log at all.
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
	for round := range log {
		if round < 1 || int(round) > cfg.Rounds {
			return MatchSummary{}, fmt.Errorf("telemetry: Match: OrderLog has round %d, outside 1-%d", round, cfg.Rounds)
		}
	}

	groups := groupConfrontations(events)

	summary := MatchSummary{
		RoutesCancelledMidRoute:              routesCancelledMidRoute(),
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

// routesCancelledMidRoute computes MatchSummary.RoutesCancelledMidRoute.
// D43 (docs/decisions/D43-row-1-unmeasurable-post-d39.md) found the row
// structurally unmeasurable by bot simulation post-D39: haltOrConvertMovement
// (confront.go) always either converts a halt's unspent budget into further
// blind Pushing On steps or has nothing left to convert, so no
// EventRouteHalted this package can read distinguishes "converted and later
// fully spent" from "genuinely never spent" any more (#267). Match no longer
// passes it an OrderLog: even the denominator (submitted non-empty routes)
// is not reported, since a population with no possible numerator is not a
// measurement. A future rule change that reintroduces a genuine mid-route
// cancellation path (D43's own "what would reopen this") makes a real
// computation here possible again.
func routesCancelledMidRoute() Rate {
	return Rate{}
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
