package main

import (
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// Breakdown is one match's contribution to the three decompositions issue
// #205's exit demonstration requires and GDD §22's per-match metric set
// deliberately does not carry. Every field here sits *beside* a
// telemetry.MatchSummary row, never in place of one: the verdicts are still
// read off rows 1, 6 and 4, and this exists because a verdict read off a
// single mean can be met by a distribution nobody would ship.
//
//   - R1 (row 1) asks whether the game "reads as a lottery" (GDD §20).
//     Fifteen percent of routes cancelled spread evenly across fifteen rounds
//     and fifteen percent concentrated in three rounds are different games,
//     and the mean cannot tell them apart — hence the per-round vectors.
//   - R6 (row 6) has a band with two failing ends, and a mean of 45% can be
//     the average of a deck where four cards always hit and nine never do.
//     R6's own third dial is "cut the deck to the six least punishing" (GDD
//     §20), which needs the per-card table to name six.
//   - R7 (row 4) is a question about player judgement, not a headcount: GDD
//     §20 says to watch "Tier IV contracts being accepted and then
//     abandoned" and whether "anybody voluntarily crosses Infamy 8". A bot
//     that stumbles into Infamy 9 through confrontations while never once
//     choosing the climb satisfies row 4 and refutes R7.
//
// This lives in cmd/simulate rather than internal/telemetry on purpose.
// internal/telemetry's surface is GDD §22's twenty rows and nothing else
// (RFC §17's "one computation, three sinks" — the analytics table and the
// debug panel are the other two sinks, and neither wants these). These are
// one demonstration's own decompositions, with one sink; adding them to
// MatchSummary would make the §22 metric set stop meaning "§22".
type Breakdown struct {
	// RoutesSubmittedByRound and RoutesHaltedByRound are row 1's own
	// denominator and numerator, split by round instead of summed — index
	// r-1 holds round r, both sized cfg.Rounds. The denominator is
	// order-log-shaped and counts submitted *non-empty* routes, exactly as
	// telemetry.MatchSummary.RoutesCancelledMidRoute defines it; summing
	// RoutesSubmittedByRound reproduces that field's own N. RoutesHaltedByRound
	// stays all-zero — no EventRouteHalted signals a genuine cancellation any
	// more (#267) — so summing it reproduces that field's own numerator (0)
	// too; kept as its own per-round vector rather than dropped so a future
	// rule change that reintroduces a real cancellation path has somewhere to
	// land without another field added here.
	RoutesSubmittedByRound []int
	RoutesHaltedByRound    []int

	// IncidentLive and IncidentHit split row 6 by card identity. A card
	// appears at most once in a match's 13-card deck (GDD §14.3 draws 13 of
	// 16 without replacement), so both are sets, not counts: IncidentLive
	// holds every card this match's deck actually put in a round within
	// 1..cfg.Rounds, and IncidentHit the subset of those whose round
	// produced at least one game.EventIncidentHit. Summing over cards
	// reproduces telemetry's own row-6 numerator and N — asserted directly
	// in breakdown_test.go, because two definitions of one §22 row silently
	// drifting apart is the failure this split invites.
	IncidentLive map[rules.IncidentCardID]bool
	IncidentHit  map[rules.IncidentCardID]bool

	// Tier4Offered counts Tier IV offers this match put in front of a seat.
	// It carries one known undercount, stated rather than papered over:
	// round 1's offer is staged by initial() before the first Resolve call
	// and cleared by that same call, so no end-of-round observation can
	// see it. Tier IV requires Infamy 9 (Config.Contracts[3].InfamyRequired)
	// and every seat starts at 0, so round 1's bootstrap offer can never be
	// a Tier IV one — the undercount is structurally zero for this field.
	Tier4Offered int

	// Tier4Accepted, Tier4Delivered and Tier4Abandoned are R7's "accepted
	// and then abandoned" watch, counted per contract instance across the
	// whole match. A held Tier IV contract leaves a seat's hand by exactly
	// two routes — resolveOneDelivery (deliveries.go) and expireContract
	// (upkeep.go) — so a Tier IV instance that vanished in a round with no
	// matching game.EventDelivered missed its deadline, which is precisely
	// what §20 means by abandoned.
	//
	// Tier4Unresolved is the rest, and it is not a rounding error: a Tier
	// IV contract's deadline is 6 rounds (Config.Contracts[3].Deadline),
	// so anything accepted from round 10 onward reaches final scoring
	// having neither been delivered nor missed its deadline. Reporting an
	// abandonment rate without it would divide by a population most of
	// whose members were never given the chance to fail. The four fields
	// satisfy Accepted == Delivered + Abandoned + Unresolved by
	// construction; breakdown_test.go asserts it rather than assuming it.
	Tier4Accepted   int
	Tier4Delivered  int
	Tier4Abandoned  int
	Tier4Unresolved int

	// AnyPlayerEverReachedInfamy9 is row 4's "reached" read literally, as
	// against telemetry.MatchSummary.AnyPlayerReachedInfamy9's final-state
	// read (D33 row 4 established that reading, and it is the one the
	// verdict is taken from). Infamy falls — a lost confrontation, Debt,
	// upkeep decay — so a match can visit Legend and end below it, and the
	// gap between these two numbers is itself a fact about how the top of
	// the ladder behaves.
	AnyPlayerEverReachedInfamy9 bool

	// AnyPlayerFinallyReachedInfamy9 is the same final-state read
	// telemetry.MatchSummary.AnyPlayerReachedInfamy9 already performs,
	// recomputed here so the breakdown CSV carries both halves of the
	// ever-versus-final comparison in one file instead of asking a reader
	// to join two. breakdown_test.go asserts the two agree on every match
	// it runs — a second copy of a §22 row's definition is only safe while
	// something checks it has not drifted.
	AnyPlayerFinallyReachedInfamy9 bool

	// Crossings classifies every 8-to-9 transition this match produced.
	Crossings crossingCounts
}

// crossingCounts classifies each crossing into the Legend band (a seat whose
// Infamy was below 9 at the previous round's end and 9 or above at this
// one's) by which of that seat's own deliberate acts happened in the same
// round. GDD §20 asks whether "anybody voluntarily crosses Infamy 8", and no
// headless read can recover intent — so this reports coincidence and says
// so, the same loose reading D33 already settled on for §22 row 14 ("a
// confrontation occurred in a round a [C] card was live" — not causation).
//
// The three acts are the Infamy-granting ones a seat chooses outright:
// delivering a contract (deliveries.go), staking a first post in a sector
// (actions.go, InfamyGainFirstPost), and winning a confrontation
// (confront.go, InfamyGainConfrontationWin). The first two are the climb R7
// is about; the third is the one R7 warns can counterfeit it. The counts are
// non-exclusive by design — a round can hold several — with the two
// exclusive readings the verdict actually needs broken out separately.
type crossingCounts struct {
	Total int

	WithDelivery         int
	WithStake            int
	WithConfrontationWin int

	// ConfrontationWinOnly is the counterfeit R7 names: a crossing whose
	// round held a confrontation win and neither a delivery nor a stake.
	ConfrontationWinOnly int

	// NoDeliberateAct is a crossing whose round held none of the three —
	// an Infamy gain from a global event card (events.go) or, since this
	// tracks a threshold rather than a delta, a seat whose Infamy was
	// already at 9 or above and dipped below it and back.
	NoDeliberateAct int
}

// contractKey identifies one contract instance across rounds. game.ContractID
// is a slot index scoped to a seat (game/ids.go), reused the moment a slot
// frees up, so it cannot identify an instance on its own. Origin, Destination
// and Tier pin the instance; ExpiresRound is deliberately absent, because
// GDD §8.4's Deadline Pause moves it mid-life and a key that moved with it
// would read one contract as two.
type contractKey struct {
	id                  game.ContractID
	tier                int
	origin, destination game.NodeID
}

// tier4Index is Config.Contracts' index for Tier IV — GDD §8.3's fourth and
// last band, the one R7 is entirely about.
const tier4Index = 3

// legendInfamy is GDD §11's Legend band floor, and GDD §22 row 4's own
// "reached Infamy 9". Deliberately a constant here and not read from
// Config.Contracts[tier4Index].InfamyRequired, which also happens to be 9:
// those are two different facts that coincide. One is where the Infamy
// ladder's top tier starts, which is a rule; the other is what Tier IV
// contracts ask of you, which is a dial a scenario Config may move (D11).
// Reading the ladder off the contract table would silently redefine row 4
// the first time a scenario changed the contract. internal/telemetry's own
// row-4 read spells the same 9 out for the same reason.
const legendInfamy = 9

// breakdownTracker accumulates the facts that are only visible between
// rounds. R1's and R6's splits are recoverable at the end from the order
// log, the event stream and the final MatchState (finish, below); R7's are
// not — no event announces a contract offer, an acceptance, or an expiry,
// and Infamy's path through the match is gone by final scoring. This walks
// the match instead, one call per resolved round.
type breakdownTracker struct {
	players int

	prevInfamy    []int
	prevContracts []map[contractKey]bool

	b Breakdown
}

func newBreakdownTracker(players int) *breakdownTracker {
	t := &breakdownTracker{
		players:       players,
		prevInfamy:    make([]int, players),
		prevContracts: make([]map[contractKey]bool, players),
	}
	for seat := range t.prevContracts {
		t.prevContracts[seat] = map[contractKey]bool{}
	}
	return t
}

// observe folds one resolved round into the tracker. s is the state that
// round produced (s.Round is the round itself, Resolve having already
// advanced it) and roundEvents is that round's own slice of the stream.
func (t *breakdownTracker) observe(s rules.MatchState, roundEvents []game.Event) {
	delivered := deliveredTier4Contracts(roundEvents)
	acts := deliberateActs(roundEvents, t.players)

	for seat := range t.players {
		p := s.Players[seat]

		for _, o := range p.PendingOffer {
			if o.Tier == tier4Index {
				t.b.Tier4Offered++
			}
		}

		held := make(map[contractKey]bool, len(p.Contracts))
		for _, c := range p.Contracts {
			held[contractKey{id: c.ID, tier: c.Tier, origin: c.Origin, destination: c.Destination}] = true
		}
		for k := range held {
			if k.tier == tier4Index && !t.prevContracts[seat][k] {
				t.b.Tier4Accepted++
			}
		}
		for k := range t.prevContracts[seat] {
			if k.tier != tier4Index || held[k] {
				continue
			}
			if delivered[seatContract{seat: game.SeatID(seat), id: k.id}] {
				t.b.Tier4Delivered++
			} else {
				t.b.Tier4Abandoned++
			}
		}
		t.prevContracts[seat] = held

		if p.Infamy >= legendInfamy {
			t.b.AnyPlayerEverReachedInfamy9 = true
			if t.prevInfamy[seat] < legendInfamy {
				t.b.Crossings.record(acts[seat])
			}
		}
		t.prevInfamy[seat] = p.Infamy
	}
}

// seatContract names one seat's contract slot, the pair game.Event carries
// for a delivery (Seat plus Contract) and the only identity a delivery event
// and a held Contract share.
type seatContract struct {
	seat game.SeatID
	id   game.ContractID
}

// deliveredTier4Contracts is the set of Tier IV contracts delivered in one
// round's events. Tier comes off the event itself — game.Event.Tier is
// populated for EventDelivered precisely because "the delivery announcement
// always names tier alongside actor and location" (event.go).
func deliveredTier4Contracts(roundEvents []game.Event) map[seatContract]bool {
	set := map[seatContract]bool{}
	for _, e := range roundEvents {
		if e.Kind == game.EventDelivered && e.Tier == tier4Index {
			set[seatContract{seat: e.Seat, id: e.Contract}] = true
		}
	}
	return set
}

// seatActs is one seat's deliberate, Infamy-granting acts in one round —
// see crossingCounts for why these three and not others.
type seatActs struct {
	delivered, staked, wonConfrontation bool
}

// deliberateActs indexes seatActs by seat for one round's events. A decisive
// EventConfrontation names the winner in Seat and the loser in Target
// (decisiveEvents, confront.go), and a tie's identically-shaped event leaves
// Decisive false — so the Decisive guard is what keeps a tie participant,
// who gains no Infamy, out of the win column.
func deliberateActs(roundEvents []game.Event, players int) []seatActs {
	acts := make([]seatActs, players)
	for _, e := range roundEvents {
		if int(e.Seat) < 0 || int(e.Seat) >= players {
			continue
		}
		switch {
		case e.Kind == game.EventDelivered:
			acts[e.Seat].delivered = true
		case e.Kind == game.EventPostStaked:
			acts[e.Seat].staked = true
		case e.Kind == game.EventConfrontation && e.Decisive:
			acts[e.Seat].wonConfrontation = true
		}
	}
	return acts
}

func (c *crossingCounts) record(a seatActs) {
	c.Total++
	if a.delivered {
		c.WithDelivery++
	}
	if a.staked {
		c.WithStake++
	}
	if a.wonConfrontation {
		c.WithConfrontationWin++
	}
	switch {
	case a.delivered || a.staked:
		// A crossing the climb can account for; not counted as either
		// exclusive reading below.
	case a.wonConfrontation:
		c.ConfrontationWinOnly++
	default:
		c.NoDeliberateAct++
	}
}

// finish completes the Breakdown with the two splits that need no per-round
// observation: R1's per-round vectors, off the order log and the event
// stream, and R6's per-card sets, off the final MatchState's own incident
// deck. Both mirror internal/telemetry's definitions of the rows they split
// — see the fields' own comments.
//
// It rejects an OrderLog entry outside 1..cfg.Rounds the same way
// telemetry.Match does (match.go), rather than silently excluding it from
// RoutesSubmittedByRound the way this function used to: two readers of the
// same row 1 population disagreeing on a malformed log — one counting it
// in, one dropping it — is exactly the gap #263 found. In RunMatch's own
// flow this is unreachable, since telemetry.Match already rejects the same
// log first; it matters for a synthetic log built directly, the way
// breakdown_test.go's own fails-closed test does.
func (t *breakdownTracker) finish(s rules.MatchState, log rules.OrderLog, events []game.Event, cfg game.Config) (Breakdown, error) {
	for round := range log {
		if round < 1 || int(round) > cfg.Rounds {
			return Breakdown{}, fmt.Errorf("cmd/simulate: finish: OrderLog has round %d, outside 1-%d", round, cfg.Rounds)
		}
	}

	b := t.b

	for _, p := range s.Players {
		if p.Infamy >= legendInfamy {
			b.AnyPlayerFinallyReachedInfamy9 = true
		}
		for _, c := range p.Contracts {
			if c.Tier == tier4Index {
				b.Tier4Unresolved++
			}
		}
	}

	b.RoutesSubmittedByRound = make([]int, cfg.Rounds)
	b.RoutesHaltedByRound = make([]int, cfg.Rounds)

	for round, orders := range log {
		idx := int(round) - 1
		for _, o := range orders {
			if len(o.Route) > 0 {
				b.RoutesSubmittedByRound[idx]++
			}
		}
	}

	// RoutesHaltedByRound stays all-zero: no EventRouteHalted this match
	// produces still signals a genuine mid-route cancellation, the same
	// reason telemetry.MatchSummary.RoutesCancelledMidRoute's own
	// numerator is always 0 (match.go's routesCancelledMidRoute, #267) —
	// haltOrConvertMovement (internal/rules/confront.go, D39) converts
	// every halt's unspent budget into further blind Pushing On steps
	// instead of losing it. b.RoutesHaltedByRound is left at its
	// zero-valued make([]int, cfg.Rounds) above so summing it still
	// reproduces that field's own N-shaped numerator exactly (0), the
	// same invariant this type's own doc comment states for every row it
	// splits.
	hitRounds := map[game.RoundNumber]bool{}
	for _, e := range events {
		if e.Kind == game.EventIncidentHit {
			hitRounds[e.Round] = true
		}
	}

	// Which card was live in which round comes from rules.IncidentCardForRound
	// — the one exported form of the peek Resolve itself uses, which
	// internal/telemetry's row-6 liveness also calls. Three readers of GDD
	// §14.3's "from round 3 onward" rule, one implementation: this file's
	// whole R6 claim is that summing per card reproduces telemetry's own
	// row 6, and a second copy of the round-to-card rule is the one way
	// that could quietly stop being true. The deck is peeked and never
	// popped, so the final state still holds it whole.
	b.IncidentLive = map[rules.IncidentCardID]bool{}
	b.IncidentHit = map[rules.IncidentCardID]bool{}
	for round := 1; round <= cfg.Rounds; round++ {
		card, live := rules.IncidentCardForRound(game.RoundNumber(round), s.Graph.IncidentDeck)
		if !live {
			continue
		}
		b.IncidentLive[card] = true
		if hitRounds[game.RoundNumber(round)] {
			b.IncidentHit[card] = true
		}
	}

	return b, nil
}
