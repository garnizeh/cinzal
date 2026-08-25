package telemetry

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// This file builds one small, entirely hand-authored fixture — not a
// played-out match — shared by every test in this package. #197's own
// acceptance criterion is a fixture "whose expected numbers were worked
// out by reading the events, not by recording the implementation's
// output": every want value in match_test.go was computed by hand against
// this data, before Match was run against it, and is annotated with that
// arithmetic in match_test.go's own comments.
//
// Match takes a MatchState, an OrderLog and an event stream as pure data —
// it never re-derives one from the others or checks them for in-game
// legality — so this fixture does not need to be a legal, playable match.
// It needs only to be internally consistent enough for every row's formula
// to have a known answer.
//
// cfg.Rounds is 6, small enough to compute every row by hand while still
// giving row 8 ("final third") more than one round to work with:
// finalThirdStart(6) == 5, so the final third is rounds 5-6.

func fixtureConfig() game.Config {
	return game.Config{Rounds: 6}
}

// fixtureGraph is 4 nodes across 3 of the 4 sectors — North Vale is left
// empty on purpose, so SectorMajority (internal/rules/posts.go) has a
// sector with no live post at all to skip, exercising that branch.
//
// Node 0, Node 1: Old Docks, both leased to seat 0 — an uncontested,
// two-post majority.
// Node 2: Iron Low, leased to seat 1 — a one-post, uncontested majority.
// Node 3: Mist Heights, unleased.
func fixtureGraph() rules.Graph {
	return rules.Graph{
		Nodes: []rules.Node{
			{ID: 0, Sector: game.SectorOldDocks, Post: &rules.Post{Owner: 0, RoundsRemaining: 3}},
			{ID: 1, Sector: game.SectorOldDocks, Post: &rules.Post{Owner: 0, RoundsRemaining: 2}},
			{ID: 2, Sector: game.SectorIronLow, Post: &rules.Post{Owner: 1, RoundsRemaining: 1}},
			{ID: 3, Sector: game.SectorMistHeights},
		},
		// EventDeck: round 4 -> deck[0], round 5 -> deck[1], round 6 ->
		// deck[2] (eventCardThisRound, internal/rules/trail.go: deck[r-4]).
		// Dragnet and Festival are both Tag Convergence (cards.go); Shift
		// Change is Tag Opportunity, not Convergence — round 5 is
		// deliberately not a live Convergence round, so
		// ConvergenceCardConfrontations has a round in its denominator
		// that does not also appear in its numerator (row 14).
		EventDeck: []rules.EventCardID{rules.EventDragnet, rules.EventShiftChange, rules.EventFestival},
		// IncidentDeck: round 3 -> deck[0], round 4 -> deck[1], round 5 ->
		// deck[2] (incidentCardThisRound, internal/rules/incidents.go:
		// deck[r-3]); length 3 deliberately leaves round 6 with no live
		// incident card, so SectorIncidentsHittingAPlayer's denominator
		// (rounds 3-5, N=3) is smaller than cfg.Rounds.
		IncidentDeck: []rules.IncidentCardID{rules.IncidentFlood, rules.IncidentTorched, rules.IncidentSinkhole},
	}
}

// fixtureSight sets seat's Sight archive for the nodes and rounds given, r
// pairs of (NodeID, round...) — a small helper so fixtureState's per-seat
// archive stays readable as a table instead of a wall of RoundSet.With
// chains.
func fixtureSight(entries map[game.NodeID][]game.RoundNumber) map[game.NodeID]game.RoundSet {
	sight := make(map[game.NodeID]game.RoundSet, len(entries))
	for node, rounds := range entries {
		var set game.RoundSet
		for _, r := range rounds {
			set = set.With(r)
		}
		sight[node] = set
	}
	return sight
}

// fixtureState is the match's final MatchState, s.Round == cfg.Rounds == 6.
//
// Seat 0: Infamy 9 (reaches Legend — row 4's true case), RP 10, Balance 40
// (CashRP 40/4=10), no active contracts, 2 live posts at Old Docks
// (PostsRP 2*2=4, MajorityRP 3). Total = 10+4+3+10+0 = 27.
//
// Seat 1: Infamy 2 (stays at/below 2 — row 5's true case), RP 5, Balance 8
// (CashRP 8/4=2), 1 active contract (PenaltyRP -2), 1 live post at Iron
// Low (PostsRP 2, MajorityRP 3). Total = 5+2+3+2-2 = 10.
//
// Seat 2: Infamy 5 (neither row 4 nor row 5), RP 3, Balance 0 (CashRP 0),
// no contracts, no posts. Total = 3+0+0+0+0 = 3.
//
// FinalScore ranks these 27, 10, 3 — seat 0 wins, seat 2 is last place:
// WinnerRPLeadOverLastPlace = (27-3)/27 = 24/27.
func fixtureState() rules.MatchState {
	return rules.MatchState{
		Round: 6,
		Graph: fixtureGraph(),
		Players: []rules.Player{
			{
				Seat: 0, Infamy: 9, RP: 10, Balance: 40,
				Archive: game.SeatArchive{Sight: fixtureSight(map[game.NodeID][]game.RoundNumber{
					0: {1, 2, 5}, // has a final-third round (5) -> counted, row 8; Count()==3, not low-confidence, row 19
					1: {1},       // no final-third round -> not counted, row 8; Count()==1, low-confidence, row 19
					3: {6},       // final-third round (6) -> counted, row 8; Count()==1, low-confidence, row 19
				})},
			},
			{
				Seat: 1, Infamy: 2, RP: 5, Balance: 8,
				Contracts: []rules.Contract{{ID: 1}},
				Archive: game.SeatArchive{Sight: fixtureSight(map[game.NodeID][]game.RoundNumber{
					1: {5, 6}, // final-third -> counted, row 8; Count()==2, low-confidence, row 19
					2: {2, 3}, // no final-third round -> not counted, row 8; Count()==2, low-confidence, row 19
				})},
			},
			{
				Seat: 2, Infamy: 5, RP: 3, Balance: 0,
				Archive: game.SeatArchive{Sight: fixtureSight(map[game.NodeID][]game.RoundNumber{
					0: {1, 2, 3, 4}, // no final-third round -> not counted, row 8; Count()==4, not low-confidence, row 19
					2: {5},          // final-third -> counted, row 8; Count()==1, low-confidence, row 19
					3: {6},          // final-third -> counted, row 8; Count()==1, low-confidence, row 19
				})},
			},
		},
	}
}

// fixtureOrderLog is 18 submitted orders (3 seats x 6 rounds). Three of
// them declare an empty route (round 1 seat 2, round 3 seat 1, round 6
// seat 2) — 15 non-empty submitted routes, not 18. This count no longer
// feeds a telemetry.MatchSummary row (D43,
// docs/decisions/D43-row-1-unmeasurable-post-d39.md, RoutesCancelledMidRoute
// reports no value on any input); it still feeds
// cmd/simulate's Breakdown.RoutesSubmittedByRound (breakdown_test.go).
//
// Every order also carries a real Stance and Action — GDD §9.2/§9.3 make
// both mandatory fields of a legal turn even when the route is empty — so
// this same fixture answers round_action_test.go's hand-computed rows
// too, RoundActionSummary.StanceDistribution's own doc comment being
// explicit that "choices" means every order submitted, not merely
// confrontation participants. round_action_test.go's own comment block
// tallies every one of the 18 orders below by hand; a change here must be
// re-tallied there.
func fixtureOrderLog() rules.OrderLog {
	route := func(nodes ...game.NodeID) game.Order { return game.Order{Route: nodes} }
	empty := game.Order{}

	// noItem is ItemID's reserved-invalid zero value, named here to state
	// "this order buys nothing" explicitly — only a Deal order names a
	// real item.
	const noItem = game.ItemID(0)

	// order attaches Action/Stance/AddOns to a route- or empty-shaped
	// order without disturbing its Route, so match_test.go's existing
	// Route-only expectations stay exactly as they were.
	order := func(base game.Order, action game.ActionKind, item game.ItemID, stance game.Stance, buyLedger bool) game.Order {
		base.Action = game.ActionOrder{Kind: action, Item: item}
		base.Stance = game.StanceOrder{Stance: stance}
		base.AddOns.BuyLedger = buyLedger
		return base
	}

	return rules.OrderLog{
		1: {
			0: order(route(1), game.ActionPickup, noItem, game.StanceAggressive, false),
			1: order(route(2), game.ActionSurveil, noItem, game.StanceNeutral, false),
			2: order(empty, game.ActionNothing, noItem, game.StanceEvasive, false),
		},
		2: {
			0: order(route(2, 3), game.ActionDeal, game.ItemShiv, game.StanceAggressive, false),
			1: order(route(0), game.ActionDeliver, noItem, game.StanceEvasive, true),
			2: order(route(1), game.ActionStakePost, noItem, game.StanceNeutral, false),
		},
		3: {
			0: order(route(1), game.ActionVanish, noItem, game.StanceEvasive, false),
			1: order(empty, game.ActionNothing, noItem, game.StanceNeutral, false),
			2: order(route(3), game.ActionDeal, game.ItemShiv, game.StanceAggressive, false),
		},
		4: {
			0: order(route(2), game.ActionDeal, game.ItemMuscle, game.StanceNeutral, true),
			1: order(route(1), game.ActionSurveil, noItem, game.StanceEvasive, false),
			2: order(route(0), game.ActionPickup, noItem, game.StanceAggressive, false),
		},
		5: {
			0: order(route(0), game.ActionDeliver, noItem, game.StanceEvasive, true),
			1: order(route(3), game.ActionDeal, game.ItemPoliceBand, game.StanceAggressive, true),
			2: order(route(2), game.ActionStakePost, noItem, game.StanceNeutral, true),
		},
		6: {
			0: order(route(3), game.ActionSurveil, noItem, game.StanceAggressive, false),
			1: order(route(2), game.ActionPickup, noItem, game.StanceEvasive, false),
			2: order(empty, game.ActionVanish, noItem, game.StanceEvasive, false),
		},
	}
}

// fixtureEvents is the match's whole accumulated event stream, 26 events.
// Every group below is annotated with which MatchSummary row(s) it exists
// for; match_test.go's own comments show the arithmetic each group feeds.
func fixtureEvents() []game.Event {
	var events []game.Event

	// EventRouteHalted sample: not read by any MatchSummary row post-D43
	// (docs/decisions/D43-row-1-unmeasurable-post-d39.md) — row 1 reports no
	// measurement on any input, and no other row reads this event kind.
	// Kept as realistic stream content exercising all three HaltCause
	// values and both HaltStepsUnspent shapes, the fields' real consumers
	// being RFC §11.3's narrated resolution list (M5) and §15.1's debug
	// panel, neither of which this package computes.
	events = append(events,
		game.Event{Kind: game.EventRouteHalted, Round: 2, Node: 1, Seat: 1, HaltCause: game.HaltCauseDecisiveLoser, HaltStepsUnspent: 1},
		game.Event{Kind: game.EventRouteHalted, Round: 3, Node: 3, Seat: 1, HaltCause: game.HaltCauseTie, HaltStepsUnspent: 2},
		game.Event{Kind: game.EventRouteHalted, Round: 4, Node: 2, Seat: 2, HaltCause: game.HaltCauseDecisiveLoser, HaltStepsUnspent: 0},
		game.Event{Kind: game.EventRouteHalted, Round: 4, Node: 3, Seat: 2, HaltCause: game.HaltCauseDecisiveLoser, HaltStepsUnspent: 2},
		game.Event{Kind: game.EventRouteHalted, Round: 5, Node: 0, Seat: 0, HaltCause: game.HaltCauseCorrectedWinner, HaltStepsUnspent: 0},
		game.Event{Kind: game.EventRouteHalted, Round: 6, Node: 3, Seat: 1, HaltCause: game.HaltCauseDecisiveLoser, HaltStepsUnspent: 1},
	)

	// Row 2: 5 EventDelivered.
	events = append(events,
		game.Event{Kind: game.EventDelivered, Round: 1, Seat: 0},
		game.Event{Kind: game.EventDelivered, Round: 2, Seat: 1},
		game.Event{Kind: game.EventDelivered, Round: 3, Seat: 0},
		game.Event{Kind: game.EventDelivered, Round: 4, Seat: 2},
		game.Event{Kind: game.EventDelivered, Round: 6, Seat: 1},
	)

	// Rows 9, 10, 11, 14, 20: four confrontation groups, (Round, Node) —
	// (2,1), (3,3), (4,2), (5,0).
	events = append(events,
		// Group (2,1): decisive, winner 0 over target 1, Evasive.
		game.Event{Kind: game.EventConfrontation, Round: 2, Node: 1, Seat: 0, Target: 1, Stance: game.StanceEvasive, Decisive: true},

		// Group (3,3): decisive, winner 1, two losers (0 Evasive, 2
		// Neutral) — decisiveEvents' own one-event-per-loser shape.
		game.Event{Kind: game.EventConfrontation, Round: 3, Node: 3, Seat: 1, Target: 0, Stance: game.StanceEvasive, Decisive: true},
		game.Event{Kind: game.EventConfrontation, Round: 3, Node: 3, Seat: 1, Target: 2, Stance: game.StanceNeutral, Decisive: true},

		// Group (4,2): a tie among all three seats — tieEvents' own
		// K-1-events-for-K-participants shape, Decisive left false. Round
		// 4's live event card is Dragnet (Convergence), so this group is
		// the one feeding row 14's numerator.
		game.Event{Kind: game.EventConfrontation, Round: 4, Node: 2, Seat: 0, Target: 1, Stance: game.StanceAggressive, Decisive: false},
		game.Event{Kind: game.EventConfrontation, Round: 4, Node: 2, Seat: 1, Target: 2, Stance: game.StanceNeutral, Decisive: false},

		// Group (5,0): decisive, winner 2 over target 0, Aggressive — not
		// Evasive, does not feed row 9's numerator. Round 5's live event
		// card (Shift Change) is not Convergence-tagged, so this
		// confrontation does not feed row 14 either way.
		game.Event{Kind: game.EventConfrontation, Round: 5, Node: 0, Seat: 2, Target: 0, Stance: game.StanceAggressive, Decisive: true},
	)

	// Row 6 numerator: EventIncidentHit in 2 of the 3 live incident rounds
	// (3, 4, 5 per fixtureGraph's IncidentDeck) — round 3 and round 5 hit,
	// round 4 does not.
	events = append(events,
		game.Event{Kind: game.EventIncidentHit, Round: 3, Node: 0, Seat: 0},
		game.Event{Kind: game.EventIncidentHit, Round: 5, Node: 2, Seat: 2},
	)

	// Row 12: 5 EventIncidentExposed, spread across rounds 3-5.
	events = append(events,
		game.Event{Kind: game.EventIncidentExposed, Round: 3, Node: 0, Seat: 0},
		game.Event{Kind: game.EventIncidentExposed, Round: 3, Node: 1, Seat: 1},
		game.Event{Kind: game.EventIncidentExposed, Round: 4, Node: 3, Seat: 2},
		game.Event{Kind: game.EventIncidentExposed, Round: 5, Node: 0, Seat: 0},
		game.Event{Kind: game.EventIncidentExposed, Round: 5, Node: 2, Seat: 2},
	)

	// Row 17: 2 EventLoitering.
	events = append(events,
		game.Event{Kind: game.EventLoitering, Round: 2, Node: 2, Seat: 1},
		game.Event{Kind: game.EventLoitering, Round: 5, Node: 3, Seat: 2},
	)

	return events
}
