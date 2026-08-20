package telemetry

import "github.com/garnizeh/cinzal/internal/game"

// Rate is one §22 row's per-match ratio, already reduced to the single
// number D35 (docs/decisions/D35-simulation-sample-size-and-verdict-rule.md)
// pools across matches: "the match is the sampling unit, always... reduce
// each match to one number first."
//
// N is the size of the population that ratio was computed over — not a
// fixed constant, but this specific match's own count, which every field
// using Rate documents in its own comment. N == 0 means this match produced
// no members of that population at all (no incidents were ever live, no
// confrontation occurred, ...): Value is meaningless in that case, and this
// is exactly D35's own "a match with an empty denominator is excluded from
// that row's vector, and the exclusion count is reported beside it" — the
// caller's job, not Match's. Match itself never errors for a single row's
// N == 0; see Match's own doc comment for which zero denominators do fail
// closed instead.
type Rate struct {
	Value float64
	N     int
}

// MatchSummary is GDD §22's per-match metric set, computed once by Match
// from one match's final MatchState, order log, and event stream. Every
// field's own comment states its GDD §22 target band, its failure
// threshold, and — for every Rate field — exactly what N counts, quoting
// §22's own wording for the denominator rather than leaving it to be
// inferred from the implementation.
//
// Fields are grouped by GDD §22 row number in the order the table lists
// them; the row number is named in each comment so a reader can find the
// source line in docs/project/cinzal-gdd.md directly. Rows 15, 16 and 18
// have no field here at all — see this package's own doc comment for why.
// Row 13 has no field either, for a different reason: D33 found no precise
// source for it in M2's first pass, and recommended shipping open rather
// than approximate — it stays open here, not silently zero.
type MatchSummary struct {
	// Solo is never set by Match — it has no way to know, from a
	// MatchState, an OrderLog and an event stream alone, whether this
	// match's opponents were bots. The caller sets it after Match returns
	// (RFC §14.4): "solo results... are also tagged separately in
	// telemetry, because bot-opponent data would otherwise contaminate the
	// human-versus-human balance numbers" — cmd/simulate always sets this
	// true for its own sweeps; M4's analytics write path sets it from the
	// match's own solo flag.
	Solo bool

	// RoutesCancelledMidRoute is row 1: "< 15% of submitted routes" fails
	// if > 15% (R1: "the confrontation rule gets softened"). Value is
	// len(EventRouteHalted) / N. N counts every OrderLog entry across the
	// whole match whose Route has at least one step — an order that never
	// declared a route cannot be "cancelled mid-route" by definition, so
	// GDD §22's own phrase ("of submitted routes") is read here as
	// submitted *non-empty* routes, the population EventRouteHalted could
	// possibly have fired against (D33 row 1: the numerator is
	// EventRouteHalted; the denominator is order-log-shaped, not an
	// event).
	RoutesCancelledMidRoute Rate

	// DeliveriesPerPlayer is row 2: target 4-6, fails if < 3 ("cooldown too
	// long or map too large"). len(EventDelivered) / len(Players) — a
	// per-player metric reduced to this match's own mean across its
	// players (D35), never excluded: every completed match has at least
	// one player.
	DeliveriesPerPlayer float64

	// WinnerRPLeadOverLastPlace is row 3: target < 40%, fails if > 40%
	// ("comeback mechanisms insufficient"). Computed once from
	// FinalScore(s) — GDD §16's own scoring, sector-majority RP and live-
	// lease RP included, not merely Player.RP (D33 row 3). Value is
	// (winner.Total - lastPlace.Total) / winner.Total; N is 1 when
	// winner.Total > 0 and 0 otherwise — a completed match whose top score
	// is not positive has no lead worth expressing as a share of it, so it
	// is excluded rather than divided by zero or reported as a nonsensical
	// negative-denominator ratio.
	WinnerRPLeadOverLastPlace Rate

	// AnyPlayerReachedInfamy9 is row 4: target > 20%, fails if < 10% ("step
	// gradient too steep", R7). A final-state read (D33 row 4):
	// EventTierLegend fires for the whole 9-10 band, not 9 specifically, so
	// only Player.Infamy at final scoring answers this. True iff any
	// seat's final Infamy >= 9. cmd/simulate reduces many matches' bools to
	// a share; this field is this match's own 0/1 indicator (D35).
	AnyPlayerReachedInfamy9 bool

	// AnyPlayerStayedAtInfamy2OrBelow is row 5: target < 25%, fails if >
	// 40% ("anonymity still dominant", R3). A final-state read (D33 row 5)
	// — no event exists below the Feared tier at all, so this reads final
	// Infamy directly, exactly as D33 classified it: this match's own
	// ending value, not a claim about every round along the way. True iff
	// any seat's final Infamy <= 2.
	AnyPlayerStayedAtInfamy2OrBelow bool

	// SectorIncidentsHittingAPlayer is row 6: target 30-60% of incidents,
	// fails if < 20% ("players route around trivially") or > 70%
	// ("unavoidable, feels unfair", R6). Numerator: count of rounds with at
	// least one EventIncidentHit. N: count of rounds with a live sector
	// incident card at all (GDD §14.3, "From round 3 onward") — an
	// incident that never had an eligible seat to hit (or, like Sinkhole,
	// structurally never hits a seat at all) still counts as a round the
	// incident deck spent, and correctly reads as a miss rather than being
	// excluded from the population. "Live" is re-derived from
	// MatchState.Graph.IncidentDeck's own length and GDD §14.3's
	// round-3-onward rule, the same seed-derivable-card-identity move D33
	// already used for row 14 — see liveIncidentRounds.
	SectorIncidentsHittingAPlayer Rate

	// LiveLeasesAtFinalScoring is row 7: target 2-4 per player, fails if <
	// 1 ("lease rate too high", §10.4). A final-state read (D33 row 7):
	// EventLeaseExpired only fires on expiry, never on what is still live.
	// Count of Nodes with a non-nil Post, divided by len(Players) — never
	// excluded, same reasoning as row 2.
	LiveLeasesAtFinalScoring float64

	// ShareOfMapUnderSightFinalThird is row 8: target 30-55%, fails if >
	// 65% ("post sight still too generous", §7.2). With row 19 it is also
	// the headless guardrail on R9's remedy — see that field's comment and
	// D38. Note the asymmetry a reader of this row needs: its only stated
	// action is on the high side, so a map raised far enough to push this
	// below 30% leaves the target band without tripping anything (D37 read
	// 0.281 at 46 nodes and had to say so explicitly). A final-state,
	// fog-private read (D33 row 8): each seat's own SeatArchive.Sight,
	// aggregated across every seat — something no Project/PlayerView call
	// ever hands out together, which is exactly why this row needs the
	// full MatchState. For each seat, the share of s.Graph.Nodes with a
	// Sight round-bit set for any round in the match's final third
	// (rounds cfg.Rounds - cfg.Rounds/3 + 1 .. cfg.Rounds); this field is
	// the mean of that share across every seat (D35's per-player-metric
	// reduction). Never excluded: a completed match always has at least
	// one node and one seat.
	ShareOfMapUnderSightFinalThird float64

	// ConfrontationsWonAgainstEvasiveLoser is row 9: target 20-40% of all
	// confrontations, fails if > 55% ("Evasive is still the default
	// insurance", §9.3). Confrontations are grouped by (Round, Node), the
	// same grouping row 10/11 need (a tie emits K-1 EventConfrontation for
	// one confrontation; a multi-loser decisive result emits one per
	// loser) — counting raw events would overcount either shape.
	// Numerator: groups with at least one Decisive EventConfrontation
	// whose Target Stance is StanceEvasive. N: every confrontation group,
	// decisive or tied.
	ConfrontationsWonAgainstEvasiveLoser Rate

	// ConfrontationsPerMatch is rows 10 and 11 — one count, read against
	// two different bands depending on player count (target 4-12 either
	// way; row 10 fails if > 12 at 4-5 players, "map too small", R9; row
	// 11 fails if < 4 at 2 players, "rotating borders not converging hard
	// enough"). Which band applies is the caller's decision, from
	// len(MatchState.Players) — this field does not duplicate itself per
	// band. Count of distinct (Round, Node) EventConfrontation groups,
	// same grouping as row 9.
	ConfrontationsPerMatch int

	// PlayersEndingInFlaggedSector is row 12: target 25-50% of
	// player-rounds, fails if < 15% ("boon share too low, the flag is
	// still just 'stay out'", §14.3). Numerator: len(EventIncidentExposed)
	// — one per seat incidentEligible() already identified, independent of
	// what the incident then did to them (that's row 6). N: player-rounds,
	// len(Players) * cfg.Rounds — plain arithmetic over setup facts, the
	// same denominator shape D33 row 17 already established for Loitering,
	// read literally rather than restricted to rounds an incident could
	// actually fire in.
	PlayersEndingInFlaggedSector Rate

	// ConvergenceCardConfrontations is row 14: target > 40%, fails if <
	// 20% ("the convergence tag is decorative"). D33's loose reading: "a
	// confrontation occurred in a round a [C] card was live" — not
	// causation. Numerator: count of rounds with a live, TagConvergence
	// event card (via MatchState.Graph.EventDeck, re-derived the same
	// seed-derivable way as row 6's incident liveness) that also had at
	// least one confrontation. N: count of rounds with a live
	// TagConvergence event card at all, whether or not it produced one.
	ConvergenceCardConfrontations Rate

	// RoundsFlaggedLoitering is row 17: target < 8% of player-rounds,
	// fails if > 15% ("camping is outcompeting contracts", R11).
	// Numerator: len(EventLoitering) — already scoped to one seat, one
	// round per event, so no grouping is needed the way confrontations
	// need it. N: player-rounds, len(Players) * cfg.Rounds (D33 row 17).
	RoundsFlaggedLoitering Rate

	// HeatMapLowConfidenceEntries is row 19: target < 40%, fails if > 60%
	// ("observation coverage too thin for the tool to be usable"). With
	// row 8 (ShareOfMapUnderSightFinalThird) this is the headless guardrail
	// on R9's remedy: R9 says to raise node count when confrontations run
	// hot, and a map raised far enough leaves the Board's own data too thin
	// to deduce from — "the tool" in that failing text is §7.5's Heat Map
	// (D38). R9's own leading indicator, whether players still open the
	// Board at all, is rows 15/16 and has no field here; see this package's
	// doc comment.
	//
	// A final-state, fog-private read, the same shape as row 8 (D33 row 19):
	// game.NodeStats.ObservedRounds — Sight[node].Count(), by that type's
	// own definition — read from every seat's own SeatArchive, pooled
	// across every (seat, node) pair this match ever produced a genuine
	// Heat Map entry for. Numerator: pairs with an observed-rounds count
	// in [1, 3) — observed at all, but under three observations. N: every
	// (seat, node) pair with a positive observed-rounds count — a node
	// never observed has no Heat Map entry to be low-confidence about, so
	// it is not part of this population at all.
	HeatMapLowConfidenceEntries Rate

	// ConfrontationsInFinal3Rounds is row 20: target < 30% of all
	// confrontations, fails if > 45% ("endgame farming is real", R11).
	// Numerator: (Round, Node) confrontation groups (same grouping as row
	// 9/10/11) with Round >= cfg.Rounds - 2. N: every confrontation group,
	// the same population as ConfrontationsPerMatch.
	ConfrontationsInFinal3Rounds Rate
}

// finalThirdStart returns the first round of s.Round's final third, for a
// match sized to rounds total rounds — rounds - rounds/3 + 1, so a 15-round
// match's final third is rounds 11-15 and a 6-round match's is rounds 5-6.
// Shared by row 8's per-seat sight share; kept here beside MatchSummary
// rather than duplicated at its one call site because the arithmetic is
// exactly the kind of thing a reader should be able to check against §22's
// "final third" wording without also reading sight.go's own control flow.
func finalThirdStart(rounds int) game.RoundNumber {
	return game.RoundNumber(rounds - rounds/3 + 1)
}
