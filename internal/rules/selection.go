package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// The remaining rows of RFC §6.5's selection table, each built on bySeat or
// byFairness rather than inventing its own comparator.

// SelectLeaseByFewestRounds returns the node whose lease has the fewest
// RoundsRemaining, breaking ties by the lowest NodeID (RFC §6.5) — the key
// both Debt's lease surrender (GDD §13) and Autopilot's renewal choice use.
// NodeID rather than acquisition order is deliberate: acquisition order is
// one more piece of cross-round state to carry and keep correct (§6.6), and
// the choice is arbitrary either way, so it only has to be stable.
//
// posts is exported as []game.Post — the same shape PlayerView.SelfState
// already carries — so a bot's Autopilot decision (which sees only its own
// PlayerView) and Debt's lease surrender (which sees the full MatchState)
// can share this exact function rather than two implementations of the same
// rule.
//
// The second return is false when posts is empty — a seat with no posts has
// nothing to surrender or renew.
func SelectLeaseByFewestRounds(posts []game.Post) (game.NodeID, bool) {
	if len(posts) == 0 {
		return 0, false
	}

	best := posts[0]
	for _, p := range posts[1:] {
		if p.RoundsRemaining < best.RoundsRemaining ||
			(p.RoundsRemaining == best.RoundsRemaining && p.Node < best.Node) {
			best = p
		}
	}
	return best.Node, true
}

// leasePosts builds the []game.Post shape SelectLeaseByFewestRounds takes,
// from seat's posts in MatchState — the leases s.Graph.Nodes carries, not
// PlayerView's fog-filtered copy.
func leasePosts(s MatchState, seat game.SeatID) []game.Post {
	nodes := s.Players[seat].Posts
	posts := make([]game.Post, len(nodes))
	for i, n := range nodes {
		posts[i] = game.Post{Node: n, RoundsRemaining: s.Graph.Nodes[n].Post.RoundsRemaining}
	}
	return posts
}

// selectNewBossTarget returns New Boss's target: the lowest-RP seat, ties
// broken by the fairness key (GDD §14.2; RFC §6.5).
func selectNewBossTarget(s MatchState, rng *RNG) game.SeatID {
	return resolveFairnessTie(s, extremeRP(s, false), rng)
}

// selectBountyTarget returns Bounty's target: the highest-RP seat, ties
// broken by the fairness key — the identical chain New Boss uses one row up
// in RFC §6.5's table, for the identical reason: one stat, one named target
// (D14 §5). Because every tied candidate already shares the same RP by
// construction, the chain's own RP level is a guaranteed no-op here and
// falls through to Infamy, exactly as it does whenever New Boss's own tie
// is broken.
func selectBountyTarget(s MatchState, rng *RNG) game.SeatID {
	return resolveFairnessTie(s, extremeRP(s, true), rng)
}

// extremeRP returns every seat sharing the match's lowest (highest = false)
// or highest (highest = true) RP, in seat order.
func extremeRP(s MatchState, highest bool) []game.SeatID {
	seats := bySeat(s)

	best := s.Players[seats[0]].RP
	for _, seat := range seats[1:] {
		if rp := s.Players[seat].RP; (highest && rp > best) || (!highest && rp < best) {
			best = rp
		}
	}

	candidates := make([]game.SeatID, 0, len(seats))
	for _, seat := range seats {
		if s.Players[seat].RP == best {
			candidates = append(candidates, seat)
		}
	}
	return candidates
}

// selectRaidTargets returns every seat at the match's highest Infamy — GDD
// §14.2's Raid: "Ties: all tied players." No further tie-break: RFC §6.5
// names this row explicitly as the one that looks like it needs one and
// does not — adding one would be a rule change wearing the costume of a
// determinism fix.
func selectRaidTargets(s MatchState) []game.SeatID {
	seats := bySeat(s)

	highest := s.Players[seats[0]].Infamy
	for _, seat := range seats[1:] {
		if inf := s.Players[seat].Infamy; inf > highest {
			highest = inf
		}
	}

	var targets []game.SeatID
	for _, seat := range seats {
		if s.Players[seat].Infamy == highest {
			targets = append(targets, seat)
		}
	}
	return targets
}

// selectDefaultMeleeCargoTarget returns the lowest-seat loser carrying
// cargo — RFC §6.5's default for a 3+-way melee winner who declared no
// choice of their own (bot behavior, or a human order that named none). The
// second return is false when no loser carries cargo.
func selectDefaultMeleeCargoTarget(s MatchState, losers []game.SeatID) (game.SeatID, bool) {
	for _, seat := range filterBySeat(s, losers) {
		if s.Players[seat].Cargo != nil {
			return seat, true
		}
	}
	return 0, false
}

// orderLosersForPushback returns losers in seat order, so a 3+-way melee's
// pushback.hop draws happen in a stable sequence rather than whatever order
// the collision detector happened to append them in (RFC §6.5) — before any
// of those draws are consumed.
func orderLosersForPushback(s MatchState, losers []game.SeatID) []game.SeatID {
	return filterBySeat(s, losers)
}

// orderConfrontationsByNode returns nodes ascending by NodeID — RFC §6.5:
// when a movement step produces confrontations at more than one node, they
// resolve one node at a time, lowest NodeID first, rather than in whatever
// order the step loop happened to detect them.
func orderConfrontationsByNode(nodes []game.NodeID) []game.NodeID {
	ordered := slices.Clone(nodes)
	slices.Sort(ordered)
	return ordered
}
