package rules

import (
	"slices"
	"sort"

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
//
// ctx (issue #72) carries this round's already-drawn Festival node — see
// ownTraceSuppressed below — needed here because Festival's "leaves no
// trace" half has no consumer before Phase 6 pops the card, exactly like
// Rain's suppression already reads eventCardThisRound directly rather than
// waiting for globalEvent (events.go).
//
// incCtx (issue #73) carries this round's already-known flagged sector and
// incident card, needed here for the same reason: Distracted Guard's own
// "leaves no trace this round" (GDD §14.3) has no consumer before Phase 7
// pops the card (ownTraceSuppressed, below), and Riot's trail-entry
// permutation (GDD §14.3, D4) has to run against the entries this same
// function builds, before distributeTrail hands them out by sight — both
// strictly before incident()'s own Phase 7 call site. r is threaded through
// for Riot's one PurposeIncidentRiot draw (D4) — the only RNG consumer in
// this file; every other suppression here is a re-read, never a draw (D13).
func writeTrail(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, walks map[game.SeatID]*seatWalk, vanished map[game.SeatID]bool, roundEvents []game.Event, ctx globalEventContext, incCtx incidentContext, r *RNG) []game.Event {
	var out []game.Event

	entries := map[game.NodeID][]game.TrailEntry{}

	// Rain's and Blackout's suppression are both computed once, up front:
	// D13's card text is a world fact, not a viewer-side redaction — "No
	// 'fresh tracks' entries are recorded anywhere this round" (Rain),
	// "nobody generates trail entries" (Blackout, read from the next-round
	// modifier a prior round's card set — state.go, NextRoundModifiers) —
	// so the returned Event stream (the substrate for recap, telemetry and
	// the debug trace, RFC §6.7) must omit them too, not merely have
	// distributeTrail's Archive writes skip them below. The entries
	// themselves are still constructed here, unconditionally:
	// distributeTrail needs the pre-suppression fact to tell an honest
	// zero (nothing happened) from an Obscured one (suppression erased the
	// only thing that would have been recorded).
	suppressFreshTracks := rainActive(*s)
	blackoutActive := s.NextRound.Blackout

	addCargoTakenAndDecoy(*s, roundEvents, validated, seats, entries, ctx, incCtx)

	for _, n := range addFreshTracks(*s, validated, seats, walks, entries) {
		if !suppressFreshTracks && !blackoutActive {
			out = append(out, game.Event{Kind: game.EventFreshTracks, Round: s.Round, Node: n})
		}
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

	addItemPurchased(*s, roundEvents, entries, ctx, incCtx, validated)

	// Riot (issue #73, GDD §14.3, D4) permutes the flagged sector's
	// sight-gated entries in place, strictly before distributeTrail hands
	// them out by sight — see writeTrail's own doc for why this cannot
	// wait for incident()'s normal Phase 7 call site. Its own EventIncidentHit
	// events (D40 row 6) are appended into out here, matching every other
	// roundEvents contributor in this file.
	out = append(out, applyRiotPermutation(s, entries, incCtx, r)...)

	distributeTrail(s, validated, seats, entries, suppressFreshTracks, blackoutActive)

	return out
}

// ownTraceSuppressed reports whether seat's own entries at node should be
// suppressed this round — Festival (GDD §14.2, issue #72: "Anyone ending
// there this round... leaves no trace") or Distracted Guard (GDD §14.3,
// issue #73: "leaves no trace this round"), the two "suppress the acting
// player's own entry, not a world-level event" cases D13 already groups
// together. Both require seat's live Position (unchanged since movement,
// at every call site this is read from) to equal node — Festival's
// already-drawn target (ctx.festivalNode, buildGlobalEventContext) or
// Distracted Guard's flagged sector (incCtx.sector). A seat that declared
// Circulation Permit this round is exempt from Distracted Guard's half
// (GDD §12: immune to the incident means neither the boon nor its
// trace-suppression fires for them) but not from Festival's, which is an
// unrelated global event card.
//
// Scoped to cargo-taken and item-purchased — the two sight-gated entry
// kinds a seat can actually contribute at its own ending node; fresh
// tracks already excludes a seat's own ending node unconditionally
// (addFreshTracks), and confrontation/post-staked are Anchor rows neither
// card's text touches. Never populates Obscured (D13) — same as Vanish.
func ownTraceSuppressed(s MatchState, ctx globalEventContext, incCtx incidentContext, validated map[game.SeatID]game.Order, seat game.SeatID, node game.NodeID) bool {
	if s.Players[seat].Position != node {
		return false
	}
	if ctx.live && ctx.card == EventFestival && node == ctx.festivalNode {
		return true
	}
	if incCtx.live && incCtx.card == IncidentDistractedGuard && s.Graph.Nodes[node].Sector == *incCtx.sector &&
		!hasDiscard(validated[seat], game.ItemCirculationPermit) {
		return true
	}
	return false
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
func addCargoTakenAndDecoy(s MatchState, roundEvents []game.Event, validated map[game.SeatID]game.Order, seats []game.SeatID, entries map[game.NodeID][]game.TrailEntry, ctx globalEventContext, incCtx incidentContext) {
	for _, e := range roundEvents {
		if e.Kind != game.EventCargoTaken || ownTraceSuppressed(s, ctx, incCtx, validated, e.Seat, e.Node) {
			continue
		}
		entries[e.Node] = append(entries[e.Node], cargoTakenEntry(s, e.Node, e.Seat))
	}

	for _, seat := range seats {
		for _, d := range validated[seat].Items {
			if d.Item == game.ItemDecoy && !ownTraceSuppressed(s, ctx, incCtx, validated, seat, d.Target) {
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
// node any seat entered and then left this round — walks[seat].Path with
// its own ending position excluded by node ID, not merely by dropping the
// path's last index: a there-and-back route (e.g. Path == {2, 1, 2}) can
// revisit its own final node earlier in the same round, and that earlier
// visit is not "passing through" either — the seat ends there, so it is a
// sight source (GDD §7.2), never a fresh-tracks one, no matter how many
// times the path touches it before settling. Deduplicated per node: two
// different seats crossing the same node the same round leave one
// indistinguishable, unnamed fact, not two (#71 owns this construction
// choice — D4's own text leaves it open). A seat whose Action this round
// was Vanish contributes nothing at all (GDD §9.2: "You leave no 'fresh
// tracks' trace this round") — an honest absence, not something for
// Rain/Blackout's Obscured bookkeeping to ever see (D13).
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
		ending := path[len(path)-1]
		for _, node := range path[:len(path)-1] {
			if node == ending {
				continue
			}
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
func addItemPurchased(s MatchState, roundEvents []game.Event, entries map[game.NodeID][]game.TrailEntry, ctx globalEventContext, incCtx incidentContext, validated map[game.SeatID]game.Order) {
	for _, e := range roundEvents {
		if e.Kind != game.EventItemPurchased || ownTraceSuppressed(s, ctx, incCtx, validated, e.Seat, e.Node) {
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

// riotEligible reports whether kind is one of D4's four sight-gated
// archetypes eligible for Riot's permutation (GDD §14.3): cargo taken,
// fresh tracks, confrontation, item purchased. The five global-
// announcement kinds (delivery, post staked, lease expired, Loitering,
// loose crate) are never constructed into writeTrail's own entries map at
// all — only Loitering's 2nd-consecutive-round entry is, and it is not
// among these four — so no separate exclusion is needed for the others.
func riotEligible(kind game.EventKind) bool {
	switch kind {
	case game.EventCargoTaken, game.EventFreshTracks, game.EventConfrontation, game.EventItemPurchased:
		return true
	default:
		return false
	}
}

// riotParticipant returns e's true participant seat for D4's sort key: the
// lower of the two parties for a confrontation (addConfrontation always
// names both, unconditionally), the real actor for a cargo-taken or
// item-purchased entry when named, 0 otherwise — an unnamed
// cargo-taken/item-purchased (below its own Infamy gate) or a fresh-tracks
// entry (which never carries a seat identity at all, GDD §7.3). Two
// same-node, same-kind entries that both land on 0 here are genuinely
// anonymous to every reader regardless of which one Riot's shuffle treats
// as "first" — riotSites' own final tiebreak (its build order) is what
// keeps that case fully deterministic without needing to distinguish them
// further.
func riotParticipant(e game.TrailEntry) game.SeatID {
	if e.Kind == game.EventConfrontation {
		return min(*e.Actor, *e.Target)
	}
	if e.Actor != nil {
		return *e.Actor
	}
	return 0
}

// riotSite is one Riot-eligible entry, stripped out of entries: its
// original TrailEntry value (Node still its true origin at this point) and
// the sort key riotParticipant computed for it.
type riotSite struct {
	entry game.TrailEntry
	seat  game.SeatID
}

// riotSitesByKey implements sort.Interface over D4's total key — origin
// NodeID, then entry-type declaration order (this package's own EventKind
// order), then participant SeatID — for applyRiotPermutation's sort.Stable
// call below. A named sort.Interface type rather than an anonymous
// closure-based sort, so this file doesn't trip
// TestOnlyOrderingFileUsesSortSlice (ordering_test.go): RFC §6.5's "two
// orderings" (bySeat, byFairness) are the package's only SeatID-batching
// orderings; this is a different kind of sort (Node/Kind/seat), the same
// "different symbol" carve-out orderConfrontationsByNode already uses via
// the slices package.
type riotSitesByKey []riotSite

func (s riotSitesByKey) Len() int      { return len(s) }
func (s riotSitesByKey) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s riotSitesByKey) Less(i, j int) bool {
	a, b := s[i], s[j]
	if a.entry.Node != b.entry.Node {
		return a.entry.Node < b.entry.Node
	}
	if a.entry.Kind != b.entry.Kind {
		return a.entry.Kind < b.entry.Kind
	}
	return a.seat < b.seat
}

// applyRiotPermutation applies D4's mechanism for GDD §14.3's Riot: strips
// every riotEligible entry generated this round at a node inside the
// flagged sector out of entries, sorts them by D4's total key — origin
// NodeID, then entry-type declaration order (this package's own EventKind
// declaration order, which already matches GDD §7.3's table), then
// participant SeatID, with each riotSite's own position in the collection
// walk below (itself NodeID-ascending, then per-node construction order)
// as a final, fully deterministic tiebreak among entries that tie on all
// three (see riotParticipant's own doc) — and permutes their node
// assignment via Torn Map's exact shuffle shape (RFC §6.4) at k=n,
// PurposeIncidentRiot: a genuine, closed permutation of the real origins,
// never inventing a target node with no real activity. Entry contents —
// event type, whether it's named, and whose name — travel unchanged; only
// .Node moves. No-op, zero draws, when Riot isn't this round's card or no
// eligible entry exists (RFC §6.4's lazy-draw rule) — matches D4's own
// n=0 case exactly.
//
// D40 row 6: every eligible entry that names a seat also gets that seat its
// own EventIncidentHit before the permutation moves it — Node is the real
// origin, never the shuffled target, since row 6 is measuring what the
// incident actually did to a player's own turn, not where their trace
// ended up landing. A confrontation entry names both parties (addConfrontation
// always names both, unconditionally), so both get their own hit. A Fresh
// Tracks entry, or a cargo-taken/item-purchased entry below its own naming
// gate, has no Actor at all — riotParticipant's 0 return for that case is
// documented as ambiguous between "seat 0" and "no seat," so it is never
// read here; only entry.Actor/entry.Target, directly, decide whether a hit
// is minted. Built from the sorted sites — the same deterministic (Node,
// Kind, Seat) order the permutation itself sorts by — so the returned event
// order stays reproducible independent of the map iteration above.
func applyRiotPermutation(s *MatchState, entries map[game.NodeID][]game.TrailEntry, incCtx incidentContext, r *RNG) []game.Event {
	if !incCtx.live || incCtx.card != IncidentRiot {
		return nil
	}
	sector := *incCtx.sector

	var sites []riotSite
	for _, n := range s.Graph.Nodes {
		list, ok := entries[n.ID]
		if !ok || n.Sector != sector {
			continue
		}
		kept := list[:0:0]
		for _, e := range list {
			if riotEligible(e.Kind) {
				sites = append(sites, riotSite{entry: e, seat: riotParticipant(e)})
			} else {
				kept = append(kept, e)
			}
		}
		entries[n.ID] = kept
	}
	if len(sites) == 0 {
		return nil
	}

	sort.Stable(riotSitesByKey(sites))

	var events []game.Event
	for _, site := range sites {
		e := site.entry
		if e.Actor != nil {
			events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: e.Node, Seat: *e.Actor})
		}
		if e.Kind == game.EventConfrontation {
			events = append(events, game.Event{Kind: game.EventIncidentHit, Round: s.Round, Node: e.Node, Seat: *e.Target})
		}
	}

	targets := make([]game.NodeID, len(sites))
	for i, site := range sites {
		targets[i] = site.entry.Node
	}
	targets = PartialFisherYates(r, PurposeIncidentRiot, targets, len(targets))

	for i, site := range sites {
		e := site.entry
		e.Node = targets[i]
		entries[e.Node] = append(entries[e.Node], e)
	}

	return events
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
//
// blackout (GDD §14.2, issue #72) overrides every one of those sources at
// once — "nobody has sight beyond their own node" carves out no exception
// for a held post, a declared Surveil, or a Police Band target, so this
// returns seat's own position alone rather than reading any of the other
// three.
func seatSight(s MatchState, validated map[game.SeatID]game.Order, seat game.SeatID, blackout bool) []game.NodeID {
	pos := s.Players[seat].Position
	if blackout {
		return []game.NodeID{pos}
	}

	mask := make([]bool, len(s.Graph.Nodes))
	mask[pos] = true
	for _, n := range s.Graph.Nodes[pos].Edges {
		mask[n] = true
	}

	for _, post := range s.Players[seat].Posts {
		mask[post] = true // PostSight(post, post): each post's own node only.
	}

	// Local Informant (GDD §14.3, issue #73) grants the identical 2-step
	// distance mask as a declared Surveil — "sight of every node within 2
	// steps of wherever they end their route next round," which this
	// round's live pos already is. Player.LocalInformant is cleared by
	// distributeTrail right after this call, consumed exactly once.
	if validated[seat].Action.Kind == game.ActionSurveil || s.Players[seat].LocalInformant {
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
// to this round's sight set (capped to the seat's own node when blackoutActive
// — seatSight's own doc), then for every sighted node either append its
// (post-suppression) entries to Archive.Trail and mark the round in
// Archive.Sight, or — per D13 — mark it in Archive.Obscured instead when a
// real entry genuinely occurred there and Rain's or Blackout's suppression
// erased the only thing that would have been recorded (blackoutActive erases
// every entry kind at once; suppressFreshTracks only that one kind). A node
// with no real activity at all is an honest Sight zero either way, never
// Obscured.
func distributeTrail(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, entries map[game.NodeID][]game.TrailEntry, suppressFreshTracks, blackoutActive bool) {
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
		var after []game.TrailEntry
		switch {
		case blackoutActive:
			after = nil
		case suppressFreshTracks:
			after = withoutFreshTracks(before)
		default:
			after = before
		}
		results[n.ID] = nodeResult{entries: after, obscured: len(after) == 0}
	}

	for _, seat := range seats {
		p := &s.Players[seat]
		sight := seatSight(*s, validated, seat, blackoutActive)
		p.LocalInformant = false // consumed exactly once (GDD §14.3, issue #73)
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
