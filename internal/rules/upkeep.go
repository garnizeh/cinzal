package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// upkeep runs Phase 8 (GDD "Upkeep", RFC §6.7, D5) once per round, round 15
// included — the phase diagram draws no truncated final round, so final
// scoring (GDD §16) reads state after this step, not before it.
//
// Four steps, in the fixed, dependency-ordered sequence D5 settles:
//
//  1. Contract deadlines — penalty, discard, dropped cargo, Debt cascade.
//  2. Lease decrement — expire at zero, the public lease-expired anchor.
//  3. Sinkhole decrement — passable again at zero, no announcement.
//  4. Next-round modifier clear — Scaffolding, Retainer, Blackout,
//     Dockers' Strike.
//
// Steps 1 and 2 must run in this order: step 1's Debt cascade selects
// "the lease with fewest rounds remaining" (GDD §13), and step 2's own
// decrement must not have touched that value first, or a healthier lease
// than necessary gets surrendered (D5's worked example).
//
// entryNextRound is s.NextRound as it stood entering this round — captured
// by Resolve before this round's own globalEvent() can set a fresh value —
// so step 4 clears only a modifier this round actually consumed, never one
// just written for the round about to start (state.go's own NextRound
// comment works through why an unconditional clear is wrong).
//
// Flagged and EvasiveStepPenalty are deliberately not among these steps —
// see resetRoundFlags (resolve.go) and D5's own Reasoning section for why
// an Upkeep-phase clear can never be correct for either field. The Contact
// Cooldown and LooseCrateHeldRounds are absent for the same reason: neither
// has anything for Upkeep to do (contracts.go, trail.go).
func upkeep(s *MatchState, cfg game.Config, entryNextRound NextRoundModifiers) []game.Event {
	var events []game.Event
	events = append(events, upkeepContracts(s, cfg)...)
	events = append(events, upkeepLeases(s)...)
	upkeepSinkholes(s)
	upkeepNextRoundModifiers(s, entryNextRound)
	return events
}

// upkeepContracts is step 1: every seat's held contracts, seat-index order,
// each seat's own up-to-2 slots in ascending ContractID order (RFC §6.5's
// default — no positional advantage exists here, this just has to be
// single-valued). A contract whose ExpiresRound has been reached expires
// independently of its slot-mate: two contracts may miss their deadline in
// the same round, and each runs the full penalty/discard/Debt sequence on
// its own.
func upkeepContracts(s *MatchState, cfg game.Config) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		for _, id := range [2]game.ContractID{0, 1} {
			idx := slices.IndexFunc(s.Players[seat].Contracts, func(c Contract) bool { return c.ID == id })
			if idx < 0 {
				continue
			}
			c := s.Players[seat].Contracts[idx]
			if s.Round < c.ExpiresRound {
				continue
			}
			events = append(events, expireContract(s, seat, c, cfg)...)
		}
	}
	return events
}

// expireContract applies GDD §8.4's "miss the deadline" consequence for
// seat's contract c: pay the penalty (Penalize, cargo.go), discard the
// contract, drop any cargo held for it — "gone", not dropped on the ground,
// the way a confrontation loss works — and run Debt (applyDebt, debt.go)
// for whatever the balance alone can't cover. The deadline-miss Infamy
// loss (Tier IV only) applies unconditionally, on top of whatever Debt's
// own −1 adds if the penalty triggers it.
//
// Order between the cargo/contract bookkeeping and the Cr$ obligation
// doesn't matter — they touch disjoint fields — but the contract is removed
// and the cargo dropped before applyDebt runs so a contract already gone by
// the time Debt's own lease-surrender path reads Player.Contracts can never
// be seen half-updated.
func expireContract(s *MatchState, seat game.SeatID, c Contract, cfg game.Config) []game.Event {
	p := &s.Players[seat]

	if p.Cargo != nil && p.Cargo.Bound && p.Cargo.Contract == c.ID {
		p.Cargo = nil
	}
	p.Contracts = removeContract(p.Contracts, c.ID)

	cost, infamyLoss := Penalize(c, cfg)
	if infamyLoss != 0 {
		p.Infamy = ApplyInfamyDelta(p.Infamy, -infamyLoss)
	}

	_, debtEvent := applyDebt(s, seat, cost, s.Round)
	if debtEvent == nil {
		return nil
	}
	return []game.Event{*debtEvent}
}

// removeContract removes the contract with the given id from contracts,
// preserving the order of what remains — mirrors removePost (debt.go).
func removeContract(contracts []Contract, id game.ContractID) []Contract {
	for i, c := range contracts {
		if c.ID == id {
			return slices.Delete(contracts, i, i+1)
		}
	}
	return contracts
}

// upkeepLeases is step 2: every lease still held, including any that
// survived step 1's Debt cascade (a surrendered lease is already gone from
// Graph.Nodes by the time this runs, so it is simply absent here, not
// double-decremented). Iterates Graph.Nodes in NodeID-ascending order — the
// same map-scoped key RFC §6.5 already assigns to lease selection — rather
// than ranging a map.
func upkeepLeases(s *MatchState) []game.Event {
	var events []game.Event
	for i := range s.Graph.Nodes {
		post := s.Graph.Nodes[i].Post
		if post == nil {
			continue
		}

		rounds, expires := DecrementLease(post.RoundsRemaining)
		post.RoundsRemaining = rounds
		if !expires {
			continue
		}

		node := game.NodeID(i)
		seat := post.Owner
		s.Graph.Nodes[i].Post = nil
		s.Players[seat].Posts = removePost(s.Players[seat].Posts, node)
		events = append(events, game.Event{Kind: game.EventLeaseExpired, Round: s.Round, Node: node, Seat: seat})
	}
	return events
}

// upkeepSinkholes is step 3: every active Sinkhole (Node.SinkholeRounds),
// iterated in NodeID-ascending order. No announcement at zero — "it's read
// off the map like any other passable node" (GDD "Upkeep") — matching
// resolveSinkhole's own creation (incidents.go), which is equally silent.
func upkeepSinkholes(s *MatchState) {
	for i := range s.Graph.Nodes {
		if s.Graph.Nodes[i].SinkholeRounds > 0 {
			s.Graph.Nodes[i].SinkholeRounds--
		}
	}
}

// upkeepNextRoundModifiers is step 4: clear whichever of Scaffolding,
// Retainer, Blackout, or Dockers' Strike was already active entering this
// round (entryNextRound) — the modifier this round's own resolution steps
// (legalView, Legal, writeTrail) actually consumed. A field entryNextRound
// does not carry is left untouched, however it looks by the time this
// runs: it can only be a value this round's own globalEvent() just wrote
// for the round about to start, and clearing it here would delete a flag
// before round N+1 ever gets to read it (state.go's NextRound comment).
func upkeepNextRoundModifiers(s *MatchState, entryNextRound NextRoundModifiers) {
	if entryNextRound.Scaffolding != nil {
		s.NextRound.Scaffolding = nil
	}
	if entryNextRound.Retainer {
		s.NextRound.Retainer = false
	}
	if entryNextRound.Blackout {
		s.NextRound.Blackout = false
	}
	if entryNextRound.DockersStrike {
		s.NextRound.DockersStrike = false
	}
}
