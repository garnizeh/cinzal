package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// Project is the fog boundary (RFC §3, §9; D01): the only function in the
// codebase that turns a MatchState into something internal/render or
// internal/web may hold. Every field of the returned game.PlayerView is a
// fresh allocation — nothing aliases s, so a later mutation of either
// cannot retroactively change the other (RFC §9.1's own purity
// requirement).
//
// Project's signature is fixed by D01 and matches the RFC's own citation of
// it verbatim: no game.Config parameter. That leaves You.StepAllowance and
// You.RoundsToNextOffer unset (their zero value) — decided in D27
// (https://github.com/garnizeh/cinzal/issues/143), not a gap: Config feeds
// formulas, not visibility, so those two fields are internal/match's
// responsibility to fill from rules.Steps(view, cfg) and
// rules.RoundsToNextOffer(s, seat, cfg) once Project returns, the same way
// it already supplies Config to Legal for order validation.
func Project(s MatchState, seat game.SeatID) game.PlayerView {
	p := s.Players[seat]

	v := game.PlayerView{
		Round:   s.Round,
		You:     projectSelf(s, p),
		Nodes:   projectNodes(s, p),
		Others:  projectOthers(s, seat),
		Archive: cloneArchive(p.Archive),
	}
	v.NodeStats = deriveNodeStats(v.Archive)
	v.Trail = projectTrail(v.Archive, s.Round)
	writeAnchors(&v, s)
	v.Headline = projectHeadline(s)
	v.Deck = projectDeck(s)

	return v
}

// projectSelf builds SelfState: exact and complete, nothing fog-filtered
// (GDD §5). Unlike legalView (validate.go), which reads EntrySnapshot mid-
// Resolve for the round being validated, this reads live Player state —
// Project only ever runs on a fully-resolved MatchState, between rounds,
// so "live" here already means "as the next order phase opens" (GDD
// §11.1's own phrase for exactly this timing).
func projectSelf(s MatchState, p Player) game.SelfState {
	var cargo *game.CarriedCargo
	if p.Cargo != nil {
		c := *p.Cargo
		cargo = &c
	}

	// The next round's global event card, peeked the same way Phase 1's
	// Headline does (eventCardThisRound, trail.go) — a free re-read, the
	// deck order fixed at initial(seed, cfg) (RFC §6.4). Reused below for
	// both Headline.Category and the Curfew step modifier: both describe
	// the same upcoming card.
	nextCard, nextLive := eventCardThisRound(s.Round+1, s.Graph.EventDeck)
	curfew := nextLive && nextCard == EventCurfew

	scaffolding := s.NextRound.Scaffolding != nil && *s.NextRound.Scaffolding == s.Graph.Nodes[p.Position].Sector

	return game.SelfState{
		Balance:      p.Balance,
		Infamy:       p.Infamy,
		RP:           p.RP,
		Position:     p.Position,
		Cargo:        cargo,
		Contracts:    contractsInHand(p.Contracts),
		PendingOffer: offersInHand(p.PendingOffer),
		Items:        slices.Clone(p.Items),
		Posts:        postsHeld(s, p.Posts),
		Ledger:       slices.Clone(p.Ledger),
		StepModifiers: game.StepModifiers{
			Flagged:            p.Flagged,
			EvasiveStepPenalty: p.EvasiveStepPenalty,
			Curfew:             curfew,
			Scaffolding:        scaffolding,
			Retainer:           s.NextRound.Retainer && p.Cargo == nil,
			DistractedGuard:    p.DistractedGuard,
			StreetsBlocked:     p.StreetsBlocked,
		},
		DockersStrike: s.NextRound.DockersStrike,
	}
}

// projectNodes fog-filters s.Graph.Nodes against p.Fog (GDD §7.1): a Hidden
// node is an absent map key, never a zero-valued entry (RFC §9.1 — the
// "NodeView{Type: \"\"}" leak this representation makes unspellable), a
// Rumoured node carries no Edges (RFC §9.1: "the client physically cannot
// plot into it"), and Post/Market only populate from Known/InSight per
// GDD §7.1's own table. Also legalView's (validate.go) sole source for
// v.Nodes — the one fog-filtering walk of p.Fog in the package, not two
// (issue #161).
func projectNodes(s MatchState, p Player) map[game.NodeID]game.NodeView {
	var nodes map[game.NodeID]game.NodeView
	for i, fog := range p.Fog {
		if fog < game.FogRumoured {
			continue
		}
		if nodes == nil {
			nodes = make(map[game.NodeID]game.NodeView, len(p.Fog))
		}

		n := s.Graph.Nodes[i]
		view := game.NodeView{Fog: fog, Name: n.Name, Type: n.Type, Sector: n.Sector, X: n.X, Y: n.Y}

		if fog >= game.FogKnown {
			view.Edges = slices.Clone(n.Edges)
			if n.Post != nil {
				view.Post = &game.NodePost{Owner: n.Post.Owner, RoundsRemaining: n.Post.RoundsRemaining}
			}
		}

		if fog >= game.FogInSight && n.Type == game.NodeBlackMarket {
			view.Market = slices.Clone(n.Market)
		}

		nodes[game.NodeID(i)] = view
	}
	return nodes
}

// projectOthers builds one OpponentView per other seat: credit band, exact
// Infamy and RP, and posts (GDD §5, §5.1). OpponentView has no Position
// field at all (game/view.go) — that is the actual mechanism behind "never
// carries a position", not a check this function performs: a revealed
// position can only ever reach a viewer through Trail or Anchors.
func projectOthers(s MatchState, seat game.SeatID) []game.OpponentView {
	if len(s.Players) <= 1 {
		return nil
	}
	others := make([]game.OpponentView, 0, len(s.Players)-1)
	for _, p := range s.Players {
		if p.Seat == seat {
			continue
		}
		others = append(others, game.OpponentView{
			Seat:   p.Seat,
			Band:   creditBand(p.Balance),
			Infamy: p.Infamy,
			RP:     p.RP,
			Posts:  postsHeld(s, p.Posts),
		})
	}
	return others
}

// creditBand maps an exact balance to its GDD §5.1 disclosure band.
func creditBand(balance int) game.CreditBand {
	switch {
	case balance <= 5:
		return game.BandBroke
	case balance <= 15:
		return game.BandGettingBy
	case balance <= 30:
		return game.BandFlush
	default:
		return game.BandLoaded
	}
}

// projectTrail is RFC §9.1's four sight-gated rows (1 cargo taken, 7
// confrontation, 8 item purchased, 12 Decoy — indistinguishable from row 1
// by design, D12): archive is already this seat's own, sight-filtered
// distribution (writeTrail, trail.go, issue #71) — this only has to select
// this round's slice of it and drop the archive-only Round stamp, never
// re-derive sight itself.
func projectTrail(archive game.SeatArchive, round game.RoundNumber) []game.TrailEntry {
	var trail []game.TrailEntry
	for _, e := range archive.Trail {
		if e.Round != round {
			continue
		}
		trail = append(trail, e.TrailEntry)
	}
	return trail
}

// writeAnchors is RFC §9.1's twelve unconditional, global rows (2-6, 9-11,
// 13-16) — "The single place any of this may be written." Unlike the
// RFC's own pseudocode, this does not also cover the four sight-gated rows
// (1, 7, 8, 12): those are Trail's job (projectTrail above), matching
// PlayerView's own field split (Trail vs Anchors) despite the RFC naming
// the whole table's writer function "writeAnchors". Content here is
// identical for every seat's view — these rows "reach every player
// unconditionally" (RFC §9.1) — so, unlike projectTrail, this needs no
// viewing seat at all.
func writeAnchors(v *game.PlayerView, s MatchState) {
	var anchors []game.Anchor

	// Rows 2 (Delivery), 3 (Post staked), 4 (Lease expired), 11
	// (Informants), 13 (Dead Runner), 14 (Fence's Windfall), 15 (Informant
	// Ring, D26/#142) and 16 (Spilled Load, D26) — built once, at the end
	// of Resolve, by buildRoundAnchors (anchors.go). See
	// MatchState.RoundAnchors's own doc comment for why they cannot be
	// read any other way here.
	for _, a := range s.RoundAnchors {
		anchors = append(anchors, cloneAnchor(a))
	}

	// Rows 5, 6, 9 and 10 need no round-scoped storage: they are
	// recomputed directly from live Player state every time Project runs,
	// for every seat, in ascending SeatID order (RFC §6.5's default
	// ordering — MatchState.Players is already indexed that way).
	for i := range s.Players {
		p := &s.Players[i]

		// Row 5 (GDD §9.1): Loitering escalates silent -> a sight-gated
		// trace at 2 consecutive rounds (writeTrail, trail.go) -> this
		// global, node-only announcement at 3+. Never named (RFC §9.1).
		if p.LoiteringStreak >= 3 {
			anchors = append(anchors, game.Anchor{Kind: game.EventLoitering, Round: s.Round, Node: p.Position})
		}

		// Row 6 (GDD §8.4): a loose (unbound) crate held 2+ consecutive
		// rounds announces the holder's node, never the holder.
		if p.Cargo != nil && !p.Cargo.Bound && p.LooseCrateHeldRounds >= 2 {
			anchors = append(anchors, game.Anchor{Kind: game.EventLooseCrateHeld, Round: s.Round, Node: p.Position})
		}

		// Row 9 (GDD §11, §11.1): Feared and up, position revealed to
		// all at end of round — evaluated against the live, post-
		// resolution Infamy this function already has, exactly as GDD
		// §11.1 requires.
		if EndOfRoundPositionRevealFires(p.Infamy) {
			actor := p.Seat
			anchors = append(anchors, game.Anchor{Kind: game.EventTierFeared, Round: s.Round, Node: p.Position, Actor: &actor})
		}

		// Row 10 (GDD §11, §11.1): Legend, position public through the
		// whole order phase about to open — "as the order phase opens"
		// is exactly the live Infamy Project reads, since nothing
		// mutates state between Resolve returning and that phase
		// starting.
		if OrderPhasePositionBroadcastFires(p.Infamy) {
			actor := p.Seat
			anchors = append(anchors, game.Anchor{Kind: game.EventTierLegend, Round: s.Round, Node: p.Position, Actor: &actor})
		}
	}

	v.Anchors = anchors
}

// cloneAnchor returns a deep copy of a — only Actor needs reallocating, the
// same shape cloneArchive (state.go) already uses for TrailEntry.
func cloneAnchor(a game.Anchor) game.Anchor {
	if a.Actor != nil {
		actor := *a.Actor
		a.Actor = &actor
	}
	return a
}

// deriveNodeStats is the Heat Map's per-node rate (RFC §9.2, GDD §7.5),
// built from the seat's own already-cloned archive: ObservedRounds and
// ObscuredRounds are each a RoundSet.Count(); TrafficRounds intersects a
// node's observed rounds against the rounds its own Trail actually carries
// an entry for that node — RoundSet's bitwise AND does the intersection,
// no per-round loop needed. Built from the union of Sight and Obscured
// keys: in practice Obscured only ever applies to a round the seat also
// had Sight of (D13's suppression fires against a real observation), but
// this does not assume it.
func deriveNodeStats(archive game.SeatArchive) map[game.NodeID]game.NodeStats {
	if archive.Sight == nil && archive.Obscured == nil {
		return nil
	}

	traffic := make(map[game.NodeID]game.RoundSet, len(archive.Trail))
	for _, e := range archive.Trail {
		traffic[e.Node] = traffic[e.Node].With(e.Round)
	}

	stats := make(map[game.NodeID]game.NodeStats, len(archive.Sight))
	for node, sight := range archive.Sight {
		stats[node] = game.NodeStats{
			ObservedRounds: sight.Count(),
			TrafficRounds:  (sight & traffic[node]).Count(),
			ObscuredRounds: archive.Obscured[node].Count(),
		}
	}
	for node, obscured := range archive.Obscured {
		if _, ok := stats[node]; ok {
			continue
		}
		stats[node] = game.NodeStats{ObscuredRounds: obscured.Count()}
	}
	return stats
}

// projectHeadline is GDD §14.1's pre-order broadcast: the next round's
// global event category (nil before round 4, matching Category's own "nil
// in rounds 1-3" contract — eventCardThisRound already returns live=false
// there), the round's Unstable Sector, which UnstableSector's own doc
// comment already states is "the sector flagged for Round+1" — exactly the
// value this needs, unmodified — and, at 2 players only, the upcoming
// round's active Border set (GDD §6.3, activeBordersForRound, borders.go),
// announced "alongside the unstable sector" per that section's own text.
func projectHeadline(s MatchState) game.Headline {
	var h game.Headline

	if card, live := eventCardThisRound(s.Round+1, s.Graph.EventDeck); live {
		category := allEvents[card-1].Category
		h.Category = &category
	}

	if s.UnstableSector != nil {
		sector := *s.UnstableSector
		h.Sector = &sector
	}

	h.ActiveBorders = activeBordersForRound(s.Graph, s.Round+1, len(s.Players))

	return h
}

// projectDeck is the sector incident deck's remaining composition (GDD
// §14.3), always displayed. IncidentDeck's order is fixed at setup and
// indexed by round (incidentCardThisRound, incidents.go: deck[r-3]) —
// round s.Round's own incident already resolved inside the Resolve call
// that produced s, so "remaining" is every deck entry from s.Round+1
// onward.
func projectDeck(s MatchState) game.DeckCounts {
	deck := s.Graph.IncidentDeck
	start := max(0, int(s.Round)+1-3)

	var counts game.DeckCounts
	for i := start; i < len(deck); i++ {
		if allIncidents[deck[i]-1].Hazard {
			counts.HazardsRemaining++
		} else {
			counts.BoonsRemaining++
		}
	}
	return counts
}
