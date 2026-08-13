package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// The two Infamy thresholds GDD §7.3's trail table gates a name on. Decoy
// (D12) reuses infamyNameCargoTaken exactly — "the same question recurs:
// whose Infamy gates the fake" — rather than defining its own.
const (
	infamyNameCargoTaken    = 3 // "Cargo taken... Only if Infamy >= 3."
	infamyNameItemPurchased = 6 // "Item purchased... Only if Infamy >= 6."
)

// writeTrail is GDD §15's Step N+4 (RFC §6.7): Loitering evaluation — after
// actions, per RFC §6.6, so the Vanish exemption reads this round's real
// Infamy delta rather than a stale one — loose crate heat, the round's
// per-node trail logs, and distribution by sight into every seat's
// game.SeatArchive (RFC §9.2).
//
// validated, seats and walks are the same movement-loop values Resolve
// already threads through resolveConfrontations; vanished is resolveActions'
// second return value (actions.go); roundEvents is every event the pipeline
// has produced so far this round (confrontations, actions, deliveries,
// addons) — writeTrail reads it for the sight-gated kinds it does not itself
// produce (EventCargoTaken, EventConfrontation, EventItemPurchased) and to
// tell whether a contended action this seat declared actually succeeded.
// Only the events writeTrail itself originates (EventFreshTracks,
// EventLoitering, EventLooseCrateHeld) are returned — reappending
// roundEvents would duplicate what Resolve already accumulated.
//
// No RNG draw happens anywhere in this file (D13: "No RNG consumption
// change" — Rain's suppression is a re-read of the event deck's fixed,
// already-shuffled order, never a new draw).
func writeTrail(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, walks map[game.SeatID]*seatWalk, vanished map[game.SeatID]bool, roundEvents []game.Event) []game.Event {
	var out []game.Event

	entries := map[game.NodeID][]game.TrailEntry{}

	addCargoTakenAndDecoy(*s, roundEvents, validated, seats, entries)

	for _, n := range addFreshTracks(*s, validated, seats, walks, entries) {
		out = append(out, game.Event{Kind: game.EventFreshTracks, Round: s.Round, Node: n})
	}

	addConfrontation(roundEvents, entries)

	for _, seat := range seats {
		event, trailEntry := evaluateLoitering(s, validated, roundEvents, vanished, seat)
		if event != nil {
			out = append(out, *event)
		}
		if trailEntry != nil {
			entries[trailEntry.Node] = append(entries[trailEntry.Node], *trailEntry)
		}
		if e := evaluateLooseCrateHeat(s, seat); e != nil {
			out = append(out, *e)
		}
	}

	addItemPurchased(*s, roundEvents, entries)

	distributeTrail(s, validated, seats, entries, rainActive(*s))

	return out
}

// addCargoTakenAndDecoy adds GDD §7.3 row 1's "Cargo left here" entry for
// every EventCargoTaken this round, and — per D12 — one structurally
// identical entry for every Decoy discard, at the planter's declared Known
// node instead of a real pickup's node. Both are named iff the actor's live
// Infamy is >= 3 at this point in resolution: after confrontations, staking,
// Vanish and deliveries, strictly before globalEvent/incident/pressure/
// upkeep (D12) — which for a Pickup or a Decoy is exactly "at the moment it
// resolves" (event.go's EventCargoTaken doc), since neither action a seat
// takes this round can itself change that same seat's own Infamy again
// between here and Step N+1 having already run.
func addCargoTakenAndDecoy(s MatchState, roundEvents []game.Event, validated map[game.SeatID]game.Order, seats []game.SeatID, entries map[game.NodeID][]game.TrailEntry) {
	for _, e := range roundEvents {
		if e.Kind != game.EventCargoTaken {
			continue
		}
		entries[e.Node] = append(entries[e.Node], cargoTakenEntry(s, e.Node, e.Seat))
	}

	for _, seat := range seats {
		for _, d := range validated[seat].Items {
			if d.Item == game.ItemDecoy {
				entries[d.Target] = append(entries[d.Target], cargoTakenEntry(s, d.Target, seat))
			}
		}
	}
}

// cargoTakenEntry builds one GDD §7.3 row-1-shaped TrailEntry for seat at
// node, named iff seat's live Infamy is >= 3 — shared by a genuine pickup
// and by Decoy (D12), which must be byte-identical to a real one.
func cargoTakenEntry(s MatchState, node game.NodeID, seat game.SeatID) game.TrailEntry {
	te := game.TrailEntry{Kind: game.EventCargoTaken, Node: node}
	if s.Players[seat].Infamy >= infamyNameCargoTaken {
		actor := seat
		te.Actor = &actor
	}
	return te
}

// addFreshTracks adds GDD §7.3's never-named "Fresh tracks" entry for every
// node any seat entered and then left this round — walks[seat].Path minus
// its own final entry, which is this round's ending position, not a
// passed-through node (GDD §7.2: "Passing through is not looking around").
// Deduplicated per node: two different seats crossing the same node the
// same round leave one indistinguishable, unnamed fact, not two (#71 owns
// this construction choice — D4's own text leaves it open). A seat whose
// Action this round was Vanish contributes nothing at all (GDD §9.2: "You
// leave no 'fresh tracks' trace this round") — an honest absence, not
// something for Rain/Blackout's Obscured bookkeeping to ever see (D13).
//
// Returns the ascending-NodeID list of nodes that gained an entry, for the
// caller to mint one EventFreshTracks per node.
func addFreshTracks(s MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, walks map[game.SeatID]*seatWalk, entries map[game.NodeID][]game.TrailEntry) []game.NodeID {
	mask := make([]bool, len(s.Graph.Nodes))
	for _, seat := range seats {
		if validated[seat].Action.Kind == game.ActionVanish {
			continue
		}
		path := walks[seat].Path
		for _, node := range path[:len(path)-1] {
			mask[node] = true
		}
	}

	var nodes []game.NodeID
	for _, n := range s.Graph.Nodes {
		if mask[n.ID] {
			nodes = append(nodes, n.ID)
			entries[n.ID] = append(entries[n.ID], game.TrailEntry{Kind: game.EventFreshTracks, Node: n.ID})
		}
	}
	return nodes
}

// addConfrontation adds GDD §7.3 row 7's "Blood and broken glass" entry,
// always naming both parties, for every EventConfrontation this round —
// decisive and tied outcomes alike (confront.go emits the same Kind for
// both).
func addConfrontation(roundEvents []game.Event, entries map[game.NodeID][]game.TrailEntry) {
	for _, e := range roundEvents {
		if e.Kind != game.EventConfrontation {
			continue
		}
		actor, target := e.Seat, e.Target
		entries[e.Node] = append(entries[e.Node], game.TrailEntry{
			Kind: game.EventConfrontation, Node: e.Node, Actor: &actor, Target: &target,
		})
	}
}

// addItemPurchased adds GDD §7.3 row 8's "Someone worked the counter"
// entry for every EventItemPurchased this round, named iff the buyer's live
// Infamy is >= 6 — the same "at the moment it resolves" reasoning as
// cargoTakenEntry.
func addItemPurchased(s MatchState, roundEvents []game.Event, entries map[game.NodeID][]game.TrailEntry) {
	for _, e := range roundEvents {
		if e.Kind != game.EventItemPurchased {
			continue
		}
		item := e.Item
		te := game.TrailEntry{Kind: game.EventItemPurchased, Node: e.Node, Item: &item}
		if s.Players[e.Seat].Infamy >= infamyNameItemPurchased {
			actor := e.Seat
			te.Actor = &actor
		}
		entries[e.Node] = append(entries[e.Node], te)
	}
}

// performedQualifyingAction reports whether seat's declared Action this
// round is one of Loitering's exemptions (GDD §9.1: "Pickup, Deliver, Stake
// Post, Deal, or qualifying Vanish") and, for the first four, that it
// genuinely took effect — the corresponding event actually fired for this
// seat this round, not merely that the action was declared. A contended
// Pickup or Stake Post a rival's earlier fairness-order turn already
// claimed, or a Deal on stock someone else already bought, is not "no
// qualifying action" turned into an exemption by mere intent; a seat that
// tried and lost the race is exactly as loitering-eligible as one that
// never tried. Vanish is decided by vanished, resolveActions' own in-flight
// record of whether it actually reduced Infamy (GDD §9.1, RFC §6.6).
func performedQualifyingAction(seat game.SeatID, validated map[game.SeatID]game.Order, roundEvents []game.Event, vanished map[game.SeatID]bool) bool {
	var want game.EventKind
	switch validated[seat].Action.Kind {
	case game.ActionPickup:
		want = game.EventCargoTaken
	case game.ActionDeliver:
		want = game.EventDelivered
	case game.ActionStakePost:
		want = game.EventPostStaked
	case game.ActionDeal:
		want = game.EventItemPurchased
	case game.ActionVanish:
		return vanished[seat]
	default:
		return false
	}

	for _, e := range roundEvents {
		if e.Kind == want && e.Seat == seat {
			return true
		}
	}
	return false
}

// evaluateLoitering applies GDD §9.1's Loitering rule for seat this round,
// mutating its two cross-round counters (RFC §6.6: LastEndNode,
// LoiteringStreak) exactly once, and reports what — if anything — this
// round's streak produces: an EventLoitering (both escalation tiers carry
// the true seat, per Event's own "nothing here is fog-filtered" contract)
// and, only at the streak's 2nd consecutive round, the sight-gated
// TrailEntry GDD §7.3 describes ("readable by anyone with sight of it").
// The 3rd-round-and-up global escalation is Anchor territory (RFC §9.1 row
// 5) — #75/Project's job, not SeatArchive's, so it produces an Event but no
// TrailEntry here.
//
// Both of the rule's two conditions are evaluated against this round's
// final, post-confrontation state: withinOneStep compares this round's
// ending position against LastEndNode (same node, or adjacent — "1 step");
// Warehouses and Borders are exempt as ending nodes outright, unconditionally
// breaking the streak like any other non-loitering round.
func evaluateLoitering(s *MatchState, validated map[game.SeatID]game.Order, roundEvents []game.Event, vanished map[game.SeatID]bool, seat game.SeatID) (event *game.Event, trailEntry *game.TrailEntry) {
	p := &s.Players[seat]
	lastEnd := p.LastEndNode
	thisEnd := p.Position

	withinOneStep := thisEnd == lastEnd || slices.Contains(s.Graph.Nodes[thisEnd].Edges, lastEnd)
	exemptEndNode := s.Graph.Nodes[thisEnd].Type == game.NodeWarehouse || s.Graph.Nodes[thisEnd].Type == game.NodeBorder
	loitering := withinOneStep && !exemptEndNode && !performedQualifyingAction(seat, validated, roundEvents, vanished)

	streak := 0
	if loitering {
		streak = p.LoiteringStreak + 1
	}
	p.LoiteringStreak = streak
	p.LastEndNode = thisEnd

	if streak < 2 {
		return nil, nil
	}

	e := game.Event{Kind: game.EventLoitering, Round: s.Round, Node: thisEnd, Seat: seat}
	if streak == 2 {
		te := game.TrailEntry{Kind: game.EventLoitering, Node: thisEnd}
		return &e, &te
	}
	return &e, nil
}

// evaluateLooseCrateHeat applies GDD §8.4's loose-crate heat escalation,
// mutating seat's LooseCrateHeldRounds (RFC §6.6) and reporting an
// EventLooseCrateHeld — GDD §7.3 row 6, always global, never named (RFC
// §9.1) — for every round from the second consecutive one seat ends holding
// an unbound crate onward, not merely on the transition to 2, matching
// Loitering's own "3+" phrasing (GDD §9.1) rather than a one-shot fire.
// Bound cargo (a held contract's own pickup) never counts — only a loose
// crate has no deadline to punish camping with, which is this rule's entire
// point (GDD §8.4).
func evaluateLooseCrateHeat(s *MatchState, seat game.SeatID) *game.Event {
	p := &s.Players[seat]

	held := 0
	if p.Cargo != nil && !p.Cargo.Bound {
		held = p.LooseCrateHeldRounds + 1
	}
	p.LooseCrateHeldRounds = held

	if held < 2 {
		return nil
	}
	return &game.Event{Kind: game.EventLooseCrateHeld, Round: s.Round, Node: p.Position, Seat: seat}
}

// eventCardThisRound returns the global event deck's card for round, and
// whether one is live at all — GDD §14.2: "Global events run in rounds 4
// through 15... Rounds 1-3 have no global event." buildEventDeck (cards.go)
// builds exactly those 12 rounds' cards, in order, so round r's card is
// deck[r-4] — a peek, never a pop: RFC §6.4 fixes the whole deck order at
// initial(seed, cfg), so this is the identical peek Phase 1's Headline
// already performs, re-read rather than re-drawn (D13).
func eventCardThisRound(round game.RoundNumber, deck []EventCardID) (EventCardID, bool) {
	idx := int(round) - 4
	if idx < 0 || idx >= len(deck) {
		return 0, false
	}
	return deck[idx], true
}

// rainActive reports whether Rain (GDD §14.2) is this round's global event
// card — D13's Obscured mechanism for it: "No 'fresh tracks' entries are
// recorded anywhere this round." Blackout (GDD §14.2) is D13's other
// suppression source, but has no live trigger yet: no code anywhere sets a
// next-round modifier flag for it (that is #72's scope, RFC §6.7's upkeep
// step 4 list), so there is no state here to read — the same "nothing this
// issue's acceptance criteria asks it to do" deferral movement.go's own
// Gas Leak comment already documents for an identical reason. #72 is the
// natural place to extend suppressFreshTracks's caller once that state
// exists.
func rainActive(s MatchState) bool {
	card, ok := eventCardThisRound(s.Round, s.Graph.EventDeck)
	return ok && card == EventRain
}

// seatSight returns seat's sight set this round, ascending NodeID,
// deduplicated — GDD §7.2's four sources: the ending position and
// everything adjacent to it, each held post's own node only (PostSight,
// posts.go), everything within 2 graph steps when this round's Action is
// Surveil, and any node declared for a Police Band discard. Every source
// grants sight directly, regardless of the node's current Fog state — the
// same rule D23's opening-sight seeding already establishes at Setup
// (initial.go: a start node's Hidden neighbour is seeded straight to
// FogInSight, never held at an intermediate tier first).
func seatSight(s MatchState, validated map[game.SeatID]game.Order, seat game.SeatID) []game.NodeID {
	mask := make([]bool, len(s.Graph.Nodes))

	pos := s.Players[seat].Position
	mask[pos] = true
	for _, n := range s.Graph.Nodes[pos].Edges {
		mask[n] = true
	}

	for _, post := range s.Players[seat].Posts {
		mask[post] = true // PostSight(post, post): each post's own node only.
	}

	if validated[seat].Action.Kind == game.ActionSurveil {
		dist := s.Graph.distances(pos)
		for _, n := range s.Graph.Nodes {
			if d := dist[n.ID]; d >= 0 && d <= 2 {
				mask[n.ID] = true
			}
		}
	}

	for _, d := range validated[seat].Items {
		if d.Item == game.ItemPoliceBand {
			mask[d.Target] = true
		}
	}

	var nodes []game.NodeID
	for _, n := range s.Graph.Nodes {
		if mask[n.ID] {
			nodes = append(nodes, n.ID)
		}
	}
	return nodes
}

// updateSeatFog applies this round's sight set to p.Fog: every node in
// sight becomes FogInSight regardless of its prior state (seatSight's own
// comment explains why sight can skip tiers), and every node that was
// FogInSight last round but is not in sight this round downgrades to
// FogKnown — "sight is temporary; knowledge is not" (GDD §7.2). Downgrading
// before upgrading means a node in sight both rounds never flickers through
// FogKnown in between.
func updateSeatFog(p *Player, sight []game.NodeID) {
	mask := make([]bool, len(p.Fog))
	for _, n := range sight {
		mask[n] = true
	}

	for i, fog := range p.Fog {
		if fog == game.FogInSight && !mask[i] {
			p.Fog[i] = game.FogKnown
		}
	}
	for _, n := range sight {
		p.Fog[n] = game.FogInSight
	}
}

// distributeTrail applies GDD §7.3's "you read the logs of nodes you had
// sight of" and RFC §9.2's archive bookkeeping, for every seat: update Fog
// to this round's sight set, then for every sighted node either append its
// (post-suppression) entries to Archive.Trail and mark the round in
// Archive.Sight, or — per D13 — mark it in Archive.Obscured instead when a
// real entry genuinely occurred there and Rain's suppression erased the
// only thing that would have been recorded. A node with no real activity at
// all is an honest Sight zero either way, never Obscured.
func distributeTrail(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, entries map[game.NodeID][]game.TrailEntry, suppressFreshTracks bool) {
	type nodeResult struct {
		entries  []game.TrailEntry
		obscured bool
	}

	results := make(map[game.NodeID]nodeResult, len(entries))
	for _, n := range s.Graph.Nodes {
		before, ok := entries[n.ID]
		if !ok {
			continue
		}
		after := before
		if suppressFreshTracks {
			after = withoutFreshTracks(before)
		}
		results[n.ID] = nodeResult{entries: after, obscured: len(after) == 0}
	}

	for _, seat := range seats {
		p := &s.Players[seat]
		sight := seatSight(*s, validated, seat)
		updateSeatFog(p, sight)

		for _, node := range sight {
			res, had := results[node]
			if had && res.obscured {
				if p.Archive.Obscured == nil {
					p.Archive.Obscured = map[game.NodeID]game.RoundSet{}
				}
				p.Archive.Obscured[node] = p.Archive.Obscured[node].With(s.Round)
				continue
			}

			if p.Archive.Sight == nil {
				p.Archive.Sight = map[game.NodeID]game.RoundSet{}
			}
			p.Archive.Sight[node] = p.Archive.Sight[node].With(s.Round)
			for _, te := range res.entries {
				p.Archive.Trail = append(p.Archive.Trail, game.StampedTrailEntry{TrailEntry: te, Round: s.Round})
			}
		}
	}
}

// withoutFreshTracks returns entries with every EventFreshTracks entry
// removed — Rain's exact, narrower-than-the-round suppression (D13: "No
// 'fresh tracks' entries are recorded anywhere this round" touches that one
// kind, never cargo taken, confrontation, item purchased, or Loitering).
func withoutFreshTracks(entries []game.TrailEntry) []game.TrailEntry {
	var out []game.TrailEntry
	for _, e := range entries {
		if e.Kind != game.EventFreshTracks {
			out = append(out, e)
		}
	}
	return out
}
