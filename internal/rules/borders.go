package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// borderNodeIDs returns every Border node's ID, ascending — the shared
// candidate pool both Dragnet's seal (events.go's drawDragnetBorders) and
// two-player rotation (below) draw from. Graph.Nodes is already
// NodeID-ascending (state.go), so filtering preserves that order for free.
func borderNodeIDs(g Graph) []game.NodeID {
	var borders []game.NodeID
	for _, n := range g.Nodes {
		if n.Type == game.NodeBorder {
			borders = append(borders, n.ID)
		}
	}
	return borders
}

// activeBordersForRound returns round's active Border set under GDD §6.3's
// two-player-only rotation (mechanism fixed by D03): sort every Border by
// NodeID, alternate labels A, B, A, B... down the list; the active set is
// A on odd rounds, B on even — round 1 opens on A. Nil at any player count
// other than 2, where rotation never applies and every Border always
// accepts deliveries (issue #76's own acceptance criterion).
//
// round and players are taken explicitly rather than read off a
// MatchState, because the two callers need different rounds against the
// same state: Resolve wants the round it is currently folding (next.Round,
// already advanced), while projectHeadline (fog.go) wants the round it is
// announcing one ahead of the state it projects from — the same s.Round+1
// convention projectHeadline's Category and projectDeck already use.
func activeBordersForRound(g Graph, round game.RoundNumber, players int) []game.NodeID {
	if players != 2 {
		return nil
	}

	borders := borderNodeIDs(g)
	oddRound := round%2 == 1

	var active []game.NodeID
	for i, id := range borders {
		labelA := i%2 == 0
		if labelA == oddRound {
			active = append(active, id)
		}
	}
	return active
}

// inactiveBordersForRound is activeBordersForRound's complement: every
// Border rotation leaves closed this round. Nil at 3+ players, same as
// activeBordersForRound — rotation contributes nothing to blocking a
// delivery outside the two-player match it exists for.
func inactiveBordersForRound(g Graph, round game.RoundNumber, players int) []game.NodeID {
	if players != 2 {
		return nil
	}

	active := activeBordersForRound(g, round, players)
	var inactive []game.NodeID
	for _, id := range borderNodeIDs(g) {
		if !slices.Contains(active, id) {
			inactive = append(inactive, id)
		}
	}
	return inactive
}

// blockedBordersThisRound is every Border not accepting deliveries this
// round: Dragnet's own seal (nil unless it fired, drawDragnetBorders in
// events.go) unioned with rotation's inactive set (nil at 3+ players),
// with D28's safety valve applied on top — if that union would cover
// every Border on the map, the lowest-NodeID Border among them reopens,
// so a Deliver order is never left with zero Borders it could possibly
// resolve against. Provably a no-op at 3+ players: rotation contributes
// nothing there, and Dragnet seals at most 2 of the ≥5 Borders every
// 3+-player map has (GDD §6.2's allocation table), so the union can never
// reach "every Border" outside the two-player, 3-Border case D28 exists
// for.
func blockedBordersThisRound(g Graph, round game.RoundNumber, players int, dragnetSealed []game.NodeID) []game.NodeID {
	all := borderNodeIDs(g)

	blocked := slices.Clone(dragnetSealed)
	for _, id := range inactiveBordersForRound(g, round, players) {
		if !slices.Contains(blocked, id) {
			blocked = append(blocked, id)
		}
	}
	slices.Sort(blocked)

	if len(all) > 0 && len(blocked) == len(all) {
		reopen := all[0]
		blocked = slices.DeleteFunc(blocked, func(id game.NodeID) bool { return id == reopen })
	}

	return blocked
}
