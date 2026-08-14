package rules

import "github.com/garnizeh/cinzal/internal/game"

// buildRoundAnchors extracts round's global, unconditional position-writer
// facts (RFC §9.1 rows 2-4, 11, 13-14) from the round's own event stream,
// for MatchState.RoundAnchors — see that field's own doc comment for why
// this exists at all. Called once, at the end of Resolve, on the full
// events slice it is about to return to its caller.
//
// Rows 1, 7, 8 and 12 are sight-gated, not global, and never appear here —
// they reach Project (#75) through each seat's own Archive.Trail instead
// (writeTrail, trail.go). Rows 5, 6, 9 and 10 are global but never routed
// through events either: Project derives them directly from live Player
// state (LoiteringStreak, LooseCrateHeldRounds, Infamy) each time it runs,
// so routing them through here would just be a second, redundant path to
// the same fact.
//
// game.EventInformantRing and game.EventSpilledLoadCrate (#73's sector
// incidents) are not literal rows of RFC §9.1's table — D26 (#142) is open
// on whether they should be. Until it resolves, they are folded into rows
// 11 and 13 respectively, on the strength of their own doc comments
// (event.go): "the same position-reveal shape as EventInformants" and
// "Mirrors EventDeadRunnerCrate". Kept as their own game.EventKind on the
// resulting Anchor, not renumbered to their partner row's Kind, so a
// recap/telemetry consumer can still tell them apart, exactly as those doc
// comments intend.
func buildRoundAnchors(events []game.Event, round game.RoundNumber) []game.Anchor {
	var anchors []game.Anchor
	for _, ev := range events {
		switch ev.Kind {
		case game.EventDelivered, game.EventPostStaked, game.EventLeaseExpired, game.EventInformants, game.EventInformantRing:
			seat := ev.Seat
			anchors = append(anchors, game.Anchor{Kind: ev.Kind, Round: round, Node: ev.Node, Actor: &seat, Tier: ev.Tier})
		case game.EventDeadRunnerCrate, game.EventFenceWindfallAnnounced, game.EventSpilledLoadCrate:
			anchors = append(anchors, game.Anchor{Kind: ev.Kind, Round: round, Node: ev.Node})
		}
	}
	return anchors
}
