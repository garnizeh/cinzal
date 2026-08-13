package rules

import (
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
)

// ItemTiming is which of GDD §9.4's discard-timing classes an item falls
// into. Immediate and Armed are the two the GDD calls out explicitly — "the
// distinction is a rule": Immediate discards resolve before movement, Armed
// discards are spent whether or not they fire. Decoy is neither of those
// two: D12/#50 fixed its own resolution point at end-of-round, with the
// trail, so it gets a third value rather than being forced into either
// existing one. Muscle is never a discard at all (GDD §9.4: "not a discard
// ... permanent while held").
type ItemTiming uint8

const (
	_ ItemTiming = iota

	// TimingImmediate resolves before movement (GDD §9.4): Torn Map, Guard
	// Contact, Police Band.
	TimingImmediate

	// TimingArmed is spent whether or not it fires (GDD §9.4): Shiv,
	// Circulation Permit, Bolt Hole.
	TimingArmed

	// TimingEndOfRound resolves with the trail, at the end of the round
	// (GDD §9.4, D12/#50): Decoy only.
	TimingEndOfRound

	// TimingPermanent is never a discard (GDD §9.4): Muscle only.
	TimingPermanent
)

// itemMeta is one row of the eight-item catalog (GDD §12, §9.4): identity,
// price, and discard-timing class.
type itemMeta struct {
	Item   game.ItemID
	Price  int
	Timing ItemTiming
}

// allItems is the eight Black Market items' full catalog (GDD §12),
// declared in the GDD's own table order — the same fixed-order convention
// cards.go's allEvents and allIncidents already use, and already ascending
// by ItemID (game/ids.go's own declaration order matches). RollMarketStock
// (market.go) draws from exactly this list, never a copy built by ranging a
// map.
var allItems = []itemMeta{
	{game.ItemShiv, 4, TimingArmed},
	{game.ItemMuscle, 7, TimingPermanent},
	{game.ItemPoliceBand, 3, TimingImmediate},
	{game.ItemCirculationPermit, 5, TimingArmed},
	{game.ItemTornMap, 3, TimingImmediate},
	{game.ItemDecoy, 5, TimingEndOfRound},
	{game.ItemBoltHole, 5, TimingArmed},
	{game.ItemGuardContact, 6, TimingImmediate},
}

// itemPrice returns item's GDD §12 catalog price. Panics on an ItemID
// outside the eight-item catalog — every declared discard and every Deal
// target is already checked against a real market's stock or a held hand
// before reaching here, so an unknown ItemID at this point is a caller
// bug, not a data condition to handle gracefully.
func itemPrice(item game.ItemID) int {
	for _, it := range allItems {
		if it.Item == item {
			return it.Price
		}
	}
	panic(fmt.Sprintf("rules: itemPrice: unknown item %v", item))
}

// The confrontation-modifier constants GDD §12 fixes for the two items that
// grant one. Named here so the confrontation resolver (#69) has no magic
// numbers to invent when it sums Field 4's contribution to the TOTAL
// formula (GDD §15) — applying them is that resolver's job, not this file's.
const (
	ShivBonus   = 3 // GDD §12: "+3 in one confrontation"
	MuscleBonus = 1 // GDD §12: "+1 in all confrontations"
)

// InfamyLossGuardContact is GDD §12's fixed −3 — Guard Contact's whole
// point (GDD §9.4: "dropping three Infamy after the order phase but before
// the fighting is the whole point of the item").
const InfamyLossGuardContact = -3

// ResolveGuardContact applies Guard Contact's −3 through the same clamp
// every other Infamy delta in this package uses (ApplyInfamyDelta,
// infamy.go). GDD §9.4 requires this to land immediately, before movement —
// i.e. before any confrontation this round resolves — so the caller
// (Resolve, #67) must apply it as one of the round's first steps, ahead of
// the confrontation phase, not folded in alongside the ordinary
// InfamyGainConfrontationWin/InfamyLossConfrontationLoss deltas that phase
// produces later in the pipeline.
func ResolveGuardContact(infamy int) int {
	return ApplyInfamyDelta(infamy, InfamyLossGuardContact)
}

// RevealTornMap draws GDD §12's "reveal 4 random Hidden nodes as Known" via
// the RFC §6.4-mandated shape: partial Fisher-Yates over hidden, which the
// caller must already have built sorted by NodeID (PartialFisherYates's own
// contract) and containing only this seat's currently Hidden nodes. It
// consumes exactly min(4, len(hidden)) PurposeItemTornMap draws — zero when
// hidden is empty, since PartialFisherYates's own loop never runs. Torn Map
// is the only item in the game that draws (GDD §12).
func RevealTornMap(rng *RNG, hidden []game.NodeID) []game.NodeID {
	return PartialFisherYates(rng, PurposeItemTornMap, hidden, 4)
}

// MuscleLoss reports whether a Muscle holder loses it after one
// confrontation, per D14/#52: every non-winner of a 3+-way melee loses
// Muscle if held; a tie (GDD §15: "nobody wins... nobody loses cargo") loses
// nobody's. isWinner and isTie describe the holder's own outcome in that one
// confrontation — the caller (the confrontation resolver, #69) computes the
// melee's winner/loser partition; this function only encodes D14's ruling
// over it.
func MuscleLoss(isWinner, isTie bool) bool {
	return !isWinner && !isTie
}
