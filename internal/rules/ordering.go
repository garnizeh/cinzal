package rules

import (
	"sort"

	"github.com/garnizeh/cinzal/internal/game"
)

// The only three orderings in this package (RFC §6.5). Every "the one with
// the least/most X" selection or ranking routes through one of them —
// bySeat when position confers no advantage, byFairness when it does,
// byFinalStanding for GDD §16's end-of-match placement — never through an
// ad-hoc sort of its own. TestOnlyOrderingFileUsesSortSlice guards that: a
// fourth, independently invented ordering elsewhere in this package is
// exactly the mistake this file exists to make impossible.

// bySeat returns every seat in s, ascending seat index. This is the
// ordering for RNG batches — pressure rolls, Snatch Job relocations, crate
// placement, confrontation dice — where each draw is independent and
// position in the batch confers no advantage (RFC §6.5). Seat index is
// fixed at match setup, so it cannot be reordered by anything a round does
// to Infamy, Balance, or RP — unlike the fairness key, which must not be
// used here for exactly that reason: it would couple RNG index assignment
// to mutable state that can change earlier in the same phase.
//
// s.Players is already SeatID-indexed ascending (see MatchState's own
// comment), so this never needs to sort.
func bySeat(s MatchState) []game.SeatID {
	seats := make([]game.SeatID, len(s.Players))
	for i, p := range s.Players {
		seats[i] = p.Seat
	}
	return seats
}

// fairnessCompare reports how a and b order under the GDD §15 fairness
// key's first three levels — ascending Infamy, then ascending Balance, then
// ascending RP — returning -1, 0, or 1. It never touches the fourth level
// (the seeded coin): that level needs an RNG draw, so callers handle it
// themselves, lazily, only for the seats that actually reach it.
func fairnessCompare(s MatchState, a, b game.SeatID) int {
	pa, pb := s.Players[a], s.Players[b]

	if pa.Infamy != pb.Infamy {
		if pa.Infamy < pb.Infamy {
			return -1
		}
		return 1
	}
	if pa.Balance != pb.Balance {
		if pa.Balance < pb.Balance {
			return -1
		}
		return 1
	}
	if pa.RP != pb.RP {
		if pa.RP < pb.RP {
			return -1
		}
		return 1
	}
	return 0
}

// byFairness returns every seat in s, ordered by the GDD §15 fairness key:
// ascending Infamy, then ascending Balance, then ascending RP, then — only
// among seats still tied after all three — a per-round seeded coin. This is
// the ordering for contended actions: Step N+1's action resolution order is
// this key applied directly (RFC §6.7); New Boss's and Bounty's target
// selection reuse the identical chain to break a tie on their own primary
// key (RFC §6.5, D14 §5).
//
// The coin is one draw per tied group, not one per pairwise comparison:
// sort.SliceStable's comparator may be invoked many times for the same
// pair, and a draw on every call would both violate "exactly one index" and
// make the result depend on the sort algorithm's internals rather than on
// the tie itself. Instead the seats are first ordered on the three
// RNG-free levels, then walked once to find each maximal run still tied
// through all three — a single draw per such run selects a rotation
// offset, which turns one number into a full, unbiased order for a tied
// group of any size instead of requiring a separate draw per pair.
func byFairness(s MatchState, rng *RNG) []game.SeatID {
	seats := bySeat(s)
	sort.SliceStable(seats, func(i, j int) bool {
		return fairnessCompare(s, seats[i], seats[j]) < 0
	})

	for i := 0; i < len(seats); {
		j := i + 1
		for j < len(seats) && fairnessCompare(s, seats[i], seats[j]) == 0 {
			j++
		}
		if j-i > 1 {
			k := rng.Next(PurposeConfrontTiebreak, j-i)
			rotate(seats[i:j], k)
		}
		i = j
	}

	return seats
}

// rotate left-rotates group in place by k positions.
func rotate(group []game.SeatID, k int) {
	if k == 0 {
		return
	}
	rotated := make([]game.SeatID, len(group))
	copy(rotated, group[k:])
	copy(rotated[len(group)-k:], group[:k])
	copy(group, rotated)
}

// resolveFairnessTie returns the one seat among candidates that sorts first
// under the fairness key (RFC §6.5) — the argmin, not a full order, since
// every selection that reuses this chain (New Boss, Bounty) needs a single
// named target, not a ranking of the whole candidate set. Candidates must
// already be in seat order (e.g. built by filtering bySeat's result), which
// is what makes the coin's rotation-offset draw, when one is needed, land
// on a stable baseline.
//
// candidates must be non-empty.
func resolveFairnessTie(s MatchState, candidates []game.SeatID, rng *RNG) game.SeatID {
	best := candidates[0]
	tied := []game.SeatID{best}
	for _, c := range candidates[1:] {
		switch fairnessCompare(s, c, best) {
		case -1:
			best = c
			tied = []game.SeatID{c}
		case 0:
			tied = append(tied, c)
		}
	}

	if len(tied) == 1 {
		return tied[0]
	}
	k := rng.Next(PurposeConfrontTiebreak, len(tied))
	return tied[k]
}

// finalStandingCompare reports how a and b order under GDD §16's
// final-scoring tiebreak — descending contracts delivered, then descending
// Infamy, then descending balance — returning -1 if a ranks ahead of b, 1
// if b ranks ahead, 0 if every level ties. Unlike fairnessCompare, this key
// has no fourth, RNG-broken level: GDD §16 names exactly three, and a
// remaining tie is left in seat order (sort.SliceStable) rather than
// inventing a coin flip the spec doesn't call for.
func finalStandingCompare(s MatchState, a, b game.SeatID) int {
	pa, pb := s.Players[a], s.Players[b]

	if pa.ContractsDelivered != pb.ContractsDelivered {
		if pa.ContractsDelivered > pb.ContractsDelivered {
			return -1
		}
		return 1
	}
	if pa.Infamy != pb.Infamy {
		if pa.Infamy > pb.Infamy {
			return -1
		}
		return 1
	}
	if pa.Balance != pb.Balance {
		if pa.Balance > pb.Balance {
			return -1
		}
		return 1
	}
	return 0
}

// byFinalStanding returns every seat in s, ordered by GDD §16's final
// tiebreak key alone (finalStandingCompare). rankFinalScores uses this as
// the secondary sort beneath total RP, the primary key a match actually
// places on. Never used during a round and never for RNG batching (RFC
// §6.5) — this key exists only once, at the end of round 15.
func byFinalStanding(s MatchState) []game.SeatID {
	seats := bySeat(s)
	sort.SliceStable(seats, func(i, j int) bool {
		return finalStandingCompare(s, seats[i], seats[j]) < 0
	})
	return seats
}

// rankFinalScores sorts breakdowns into GDD §16 match placement — Total RP
// descending, ties broken by byFinalStanding's key — in place, rank 1
// first. FinalScore (scoring.go) computes Total per seat but never sorts
// it directly: every sort in this package lives here
// (TestOnlyOrderingFileUsesSortSlice), so the actual ranking step is this
// function, not a second sort.Slice call living in scoring.go.
func rankFinalScores(s MatchState, breakdowns []FinalScoreBreakdown) {
	tiebreakRank := make(map[game.SeatID]int, len(breakdowns))
	for i, seat := range byFinalStanding(s) {
		tiebreakRank[seat] = i
	}

	sort.SliceStable(breakdowns, func(i, j int) bool {
		if breakdowns[i].Total != breakdowns[j].Total {
			return breakdowns[i].Total > breakdowns[j].Total
		}
		return tiebreakRank[breakdowns[i].Seat] < tiebreakRank[breakdowns[j].Seat]
	})
}

// filterBySeat returns the seats in subset, in seat order — the shape every
// RNG-batch selection over a subset of seats needs (RFC §6.5: a 3+-way
// melee's losers, drawing pushback.hop, must be seat-ordered rather than
// however a collision detector happened to append them).
func filterBySeat(s MatchState, subset []game.SeatID) []game.SeatID {
	include := make(map[game.SeatID]bool, len(subset))
	for _, seat := range subset {
		include[seat] = true
	}

	ordered := make([]game.SeatID, 0, len(subset))
	for _, seat := range bySeat(s) {
		if include[seat] {
			ordered = append(ordered, seat)
		}
	}
	return ordered
}
