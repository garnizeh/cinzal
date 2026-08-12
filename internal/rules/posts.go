package rules

import "github.com/garnizeh/cinzal/internal/game"

// maxLeaseBlocks is GDD §10.4's fixed per-post ceiling: "Maximum 4 blocks
// held at once on a single post." Not a Config field — the sensitive dial
// GDD §10.4 calls out is the *rate* (Config.LeaseCostPerBlock,
// Config.LeaseBlockRounds), not this block count, the same line legal.go's
// handLimit already draws for GDD §12's hand cap.
const maxLeaseBlocks = 4

// PostCapReached reports whether numPosts already meets or exceeds players'
// post cap (GDD §10.3). cap is 0 and reached is true when players has no
// configured cap at all — fail closed, so a missing PostCapByPlayers entry
// can never read as "no cap" (CLAUDE.md: "a gate that passes when it can't
// run is worse than no gate").
//
// Legal (legalPostCap) enforces this at submission. Resolve's actions step
// (#70) must check it again immediately before creating a post: a stake
// legal when submitted must still degrade to Nothing, never a silent extra
// post, if the world has moved by the time it resolves. Both call sites
// share this one predicate so the two checks can never quietly drift apart
// the way GDD §9.2 and §10.3 once did (#40, D15).
func PostCapReached(numPosts, players int, cfg game.Config) (cap int, reached bool) {
	cap, ok := cfg.PostCapByPlayers[players]
	if !ok {
		return 0, true
	}
	return cap, numPosts >= cap
}

// FirstPostInSector reports whether staking a post at node would be seat's
// first live post in node's own sector, evaluated against the posts seat
// already holds — GDD §11's Infamy table: "Stake your first post in a
// sector: +1."
func FirstPostInSector(s MatchState, seat game.SeatID, node game.NodeID) bool {
	sector := s.Graph.Nodes[node].Sector
	for _, owned := range s.Players[seat].Posts {
		if s.Graph.Nodes[owned].Sector == sector {
			return false
		}
	}
	return true
}

// RenewalBlockCap returns the most lease blocks that may still be added to a
// post currently at currentRoundsRemaining without exceeding GDD §10.4's
// ceiling of maxLeaseBlocks blocks — "Renewals may top a post back up to 12
// remaining rounds," never past it, however many blocks were declared. A
// fresh, not-yet-staked post (currentRoundsRemaining 0) returns the full
// maxLeaseBlocks, so this same function serves a brand-new stake's own
// up-front block count with no special case.
func RenewalBlockCap(currentRoundsRemaining int, cfg game.Config) int {
	if cfg.LeaseBlockRounds <= 0 {
		return 0
	}
	headroom := maxLeaseBlocks*cfg.LeaseBlockRounds - currentRoundsRemaining
	if headroom <= 0 {
		return 0
	}
	return headroom / cfg.LeaseBlockRounds
}

// RenewedRoundsRemaining returns a post's new RoundsRemaining after adding
// blocks lease blocks to a lease currently at current, clamped at GDD
// §10.4's ceiling regardless of how many blocks were declared — the same
// "top back up to 12, never past it" rule RenewalBlockCap exists to keep the
// affordance layer from ever offering a wasteful count of in the first
// place; this is resolution's own defensive floor under that same ceiling.
func RenewedRoundsRemaining(current, blocks int, cfg game.Config) int {
	if cfg.LeaseBlockRounds <= 0 {
		return current
	}
	ceiling := maxLeaseBlocks * cfg.LeaseBlockRounds
	return min(ceiling, current+blocks*cfg.LeaseBlockRounds)
}

// DecrementLease applies Upkeep step 2's ordinary per-round lease decrement
// (RFC §6.7, D5) and reports whether it just crossed to zero — the instant
// "The corner went quiet" fires publicly (game.EventLeaseExpired), whether
// the lease expired on its own or was surrendered to Debt (GDD §13, §15
// Upkeep: "the same trace whether the lease expired on its own or was
// surrendered for debt").
func DecrementLease(roundsRemaining int) (next int, expires bool) {
	next = roundsRemaining - 1
	return next, next == 0
}

// PostSight reports whether a live post at postNode grants sight of target
// this round — GDD §7.2: "Each post you hold: the node itself only." v1.0
// let a post see its node *and everything adjacent*, and GDD §7.2's own
// saturation table measured that rule at 76-98% permanent map coverage,
// which is why v1.1 cut it down to this. Project (#75) must call this
// rather than re-derive the comparison, so the restriction can never
// regress to the v1.0 adjacency rule by an edit that "helpfully" reaches
// for postNode's Edges.
func PostSight(postNode, target game.NodeID) bool {
	return postNode == target
}

// SectorMajority returns, for every sector with at least one live post, the
// seat holding the most of them there — GDD §16's +3 RP sector-majority
// score, meant to be evaluated once, at final scoring, over exactly the
// leases still live at that moment. A sector with no live post, or whose
// top seat count is tied, is absent from the result: "ties score nobody"
// (GDD §16).
func SectorMajority(s MatchState) map[game.Sector]game.SeatID {
	result := map[game.Sector]game.SeatID{}
	for sector := game.SectorOldDocks; sector <= game.SectorNorthVale; sector++ {
		if seat, ok := sectorMajorityWinner(s, sector); ok {
			result[sector] = seat
		}
	}
	return result
}

// sectorMajorityWinner scans sector's live posts once per seat, in seat
// order (bySeat — position confers no advantage in a plain count, RFC
// §6.5), rather than ranging a map: this package avoids map iteration in
// resolution-adjacent code on principle (RFC §6.3), even where, as here,
// aggregation and tie detection are provably order-independent.
func sectorMajorityWinner(s MatchState, sector game.Sector) (game.SeatID, bool) {
	var winner game.SeatID
	best, tied, any := 0, false, false
	for _, seat := range bySeat(s) {
		count := 0
		for _, n := range s.Graph.Nodes {
			if n.Sector == sector && n.Post != nil && n.Post.Owner == seat {
				count++
			}
		}
		if count == 0 {
			continue
		}
		any = true
		switch {
		case count > best:
			winner, best, tied = seat, count, false
		case count == best:
			tied = true
		}
	}
	if !any || tied {
		return 0, false
	}
	return winner, true
}
