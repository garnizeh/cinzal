package rules

import "github.com/garnizeh/cinzal/internal/game"

// buildRoundAnchors extracts round's global, unconditional position-writer
// facts (RFC §9.1 rows 2-4, 11, 13-16) from the round's own event stream,
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
// incidents) are rows 15 and 16, decided by D26 (#142): a row is defined by
// its own named GDD source (here, GDD §14.3's Informant Ring and Spilled
// Load cards), not by matching distribution/named/node-only columns with
// its shape-partner row — rows 5/6, 9/10 and 13/14 already pair
// column-identical rows kept distinct for the same reason. Both share their
// partner row's case below because the case groups by disclosure shape,
// which is a different, and coarser, granularity than the RFC table's
// per-source rows (D26's own reasoning). Kept as their own game.EventKind
// on the resulting Anchor, not renumbered to their partner row's Kind, so a
// recap/telemetry consumer can still tell them apart, exactly as their own
// doc comments (event.go) intend: "the same position-reveal shape as
// EventInformants" and "Mirrors EventDeadRunnerCrate".
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
