package rules

import "github.com/garnizeh/cinzal/internal/game"

// CanCollect reports whether a seat holding contracts may pick up cargo (GDD
// §8.4). A loose crate (Dead Runner, Spilled Load — cargo.Bound false) takes
// no contract at all: "anyone, no contract required." Bound cargo — fallen
// from a lost confrontation — needs a held contract with the identical
// origin/destination pair; contracts is per-seat, so this answers the
// question for whichever seat's own Contracts slice is passed in.
func CanCollect(contracts []Contract, cargo Cargo) bool {
	if !cargo.Bound {
		return true
	}
	for _, c := range contracts {
		if c.Origin == cargo.Origin && c.Destination == cargo.Destination {
			return true
		}
	}
	return false
}

// WithDeadlinePause returns a copy of c with GDD §8.4's Deadline Pause
// applied, once: losing a confrontation while carrying c's cargo extends
// c's deadline by one round, the first time only. DeadlinePauseUsed lives on
// the contract instance, never the seat or the cargo, so two seats racing
// for the same dropped crate (§8.4's worked case) never share or inherit
// one another's pause — c is already scoped to exactly one seat's one
// contract slot.
//
// Calling this again on a contract that already used its pause is a no-op:
// the returned copy is unchanged.
func (c Contract) WithDeadlinePause() Contract {
	if c.DeadlinePauseUsed {
		return c
	}
	c.ExpiresRound++
	c.DeadlinePauseUsed = true
	return c
}

// NextLooseCrateHeldRounds computes a seat's updated LooseCrateHeldRounds
// (RFC §6.6) for a round just ended: 0 if the seat is not carrying an
// unbound loose crate at the end of this round, otherwise one more than
// current. Carrying bound cargo, or nothing, both reset the streak — only
// an unbroken run of rounds spent holding the same loose crate counts (GDD
// §8.4).
func NextLooseCrateHeldRounds(carryingLoose bool, current int) int {
	if !carryingLoose {
		return 0
	}
	return current + 1
}

// LooseCrateHeatFires reports whether heldRounds — a seat's
// LooseCrateHeldRounds after NextLooseCrateHeldRounds has updated it for the
// round just ended — has reached GDD §8.4's "run hot" threshold: a global
// announcement naming the node (never the seat, GDD §8.4: "not you, but the
// node is enough") fires "from the second consecutive round you end holding
// one." Producing and distributing that announcement (game.EventKind
// EventLooseCrateHeld) is writeTrail's job (#71); this is the threshold rule
// it calls.
func LooseCrateHeatFires(heldRounds int) bool {
	return heldRounds >= 2
}

// warehousePickupContract reports the first of seat's held contracts whose
// Origin is node, when node is a Warehouse (GDD §9.2's first Pickup source:
// "a Warehouse matching a held contract") — the source with no supply limit
// (GDD §6.2, cut in v1.1), so nothing is consumed from Graph.Cargo here.
func warehousePickupContract(s MatchState, seat game.SeatID, node game.NodeID) (Contract, bool) {
	if s.Graph.Nodes[node].Type != game.NodeWarehouse {
		return Contract{}, false
	}
	for _, c := range s.Players[seat].Contracts {
		if c.Origin == node {
			return c, true
		}
	}
	return Contract{}, false
}

// groundCargoIndex returns the index into s.Graph.Cargo of the first entry at
// node seat may collect (GDD §9.2's second Pickup source: "any node with
// dropped cargo you have the contract for"), via CanCollect — a bound crate
// needing a matching contract, an unbound loose crate collectible by anyone.
func groundCargoIndex(s MatchState, seat game.SeatID, node game.NodeID) (int, bool) {
	for i, c := range s.Graph.Cargo {
		if c.Node == node && CanCollect(s.Players[seat].Contracts, c) {
			return i, true
		}
	}
	return 0, false
}

// pickupAvailable reports whether a Pickup at node would find something for
// seat to take, checking both GDD §9.2 sources. This is the predicate
// validate.go's Step 0 degradation check must use — checking only
// groundCargoIndex, as an earlier version of that check did, would wrongly
// degrade every legitimate Warehouse Pickup, since a Warehouse's own supply
// is never represented as a Graph.Cargo entry at all.
func pickupAvailable(s MatchState, seat game.SeatID, node game.NodeID) bool {
	if _, ok := groundCargoIndex(s, seat, node); ok {
		return true
	}
	_, ok := warehousePickupContract(s, seat, node)
	return ok
}

// Deliver reports GDD §8.3's payout for completing c: Cr$ payment and RP
// from c's tier row, plus the Infamy gained on delivery — 1 for Tiers I and
// II, 2 for Tiers III and IV ("Delivering any contract grants +1 Infamy;
// tiers III and IV grant +2").
func Deliver(c Contract, cfg game.Config) (payment, rp, infamyGain int) {
	tier := cfg.Contracts[c.Tier]
	infamyGain = 1
	if c.Tier >= 2 {
		infamyGain = 2
	}
	return tier.Payment, tier.RP, infamyGain
}

// Penalize reports GDD §8.3's cost for missing c's deadline: the tier's Cr$
// penalty, plus its additional Infamy loss (0 for every tier except IV,
// which also costs −2 Infamy).
func Penalize(c Contract, cfg game.Config) (cost, infamyLoss int) {
	tier := cfg.Contracts[c.Tier]
	return tier.Penalty, tier.PenaltyInfamy
}
