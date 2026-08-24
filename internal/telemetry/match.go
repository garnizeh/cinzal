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
// A fifth structural check joins those four: every OrderLog round key must
// fall within 1..cfg.Rounds. nonEmptyRoutes ranges log's own map keys with
// no bound of its own, trusting this check to have already run — the same
// division of labor RoundActions already applies to its own fold
// (round_action.go). Before this check existed, that trust was misplaced:
// row 1's own population counted every (round, seat) entry the caller's
// log happened to hold, so a log with a stray entry outside 1..cfg.Rounds
// silently inflated RoutesCancelledMidRoute.N while
// cmd/simulate's own per-round split — bounded from the start — did not
// (#263). Rejecting it here, once, keeps that bound a single fact both
// readers agree on rather than two independent guesses at it.
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

// routeHaltKey identifies one (round, seat) pair — the unit D39 defines
// both RoutesCancelledMidRoute's population (a seat's submitted route that
// round) and its numerator (that same seat's first halt that round) over.
type routeHaltKey struct {
	round game.RoundNumber
	seat  game.SeatID
}

// nonEmptyRoutes returns every (round, seat) pair whose OrderLog entry
// declared a non-empty Route — RoutesCancelledMidRoute's denominator
// population, unchanged by D39: an order that never declared a route
// cannot be "cancelled mid-route" by definition. It trusts log's round
// keys are already within 1..cfg.Rounds — Match's own precondition checks
// enforce that once, rather than this helper re-checking a bound its
// caller already guarantees (#263).
func nonEmptyRoutes(log rules.OrderLog) map[routeHaltKey]bool {
	routes := make(map[routeHaltKey]bool)
	for round, orders := range log {
		for seat, order := range orders {
			if len(order.Route) > 0 {
				routes[routeHaltKey{round: round, seat: seat}] = true
			}
		}
	}
	return routes
}

// firstHaltUnspent maps every (round, seat) pair with at least one
// EventRouteHalted to that round's *first* halt's HaltStepsUnspent. A route
// can be halted more than once in a round — RFC §6.5's own worked case: "a
// pushed loser is caught again by someone else's later movement step" — so
// only the earliest catch could have cancelled anything; events later in
// the stream against an already-halted (round, seat) are ignored here,
// exactly as D39 specifies ("first halt that round").
func firstHaltUnspent(events []game.Event) map[routeHaltKey]int {
	unspent := make(map[routeHaltKey]int)
	seen := make(map[routeHaltKey]bool)
	for _, e := range events {
		if e.Kind != game.EventRouteHalted {
			continue
		}
		key := routeHaltKey{round: e.Round, seat: e.Seat}
		if seen[key] {
			continue
		}
		seen[key] = true
		unspent[key] = e.HaltStepsUnspent
	}
	return unspent
}

// routesCancelledMidRoute computes MatchSummary.RoutesCancelledMidRoute —
// see that field's own doc comment for the numerator/denominator
// definition. Ranges log's and events' own map/slice shapes only to sum a
// count, the same provably order-independent aggregation posts.go's
// sectorMajorityWinner documents for its own map-adjacent code — unlike
// Resolve's own pipeline (RFC §6.3), nothing here decides a tie or writes
// state from iteration order, and TestMatchIsDeterministic checks the
// result holds regardless. firstHaltUnspent's own single linear pass over
// events is the one place order matters, and that order is events' own
// slice order, not a map's.
func routesCancelledMidRoute(log rules.OrderLog, events []game.Event) Rate {
	submitted := nonEmptyRoutes(log)
	if len(submitted) == 0 {
		return Rate{}
	}

	halts := firstHaltUnspent(events)
	n := 0
	for key := range submitted {
		if unspent, ok := halts[key]; ok && unspent > 0 {
			n++
		}
	}
	return Rate{Value: float64(n) / float64(len(submitted)), N: len(submitted)}
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
