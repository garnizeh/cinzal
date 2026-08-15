package game

import "slices"

// Order is the whole of one player's turn: the five fields of GDD §9, plus
// the abandon-cargo flag and the RFC §11.1a staleness field. It is the
// server's only input — everything Resolve does starts from a map of these.
type Order struct {
	// Round is the round this form was rendered for (RFC §11.1a), checked
	// against the round it resolves in so a stale resubmission after a tick
	// is rejected rather than silently acting on outdated state.
	Round RoundNumber

	// Route is a sequence of steps along graph edges, starting from the
	// player's current position. An empty route — staying put — is legal
	// (GDD §9.1).
	Route []NodeID

	// PushingOn is the optional blind continuation appended to a route that
	// ends on a Hidden node (GDD §9.1). Zero value (Steps: 0, Bias: nil)
	// means no Pushing On was declared.
	PushingOn PushingOn

	// Action is the action taken at the route's final node (GDD §9.2).
	Action ActionOrder

	// Stance is the one stance declared for any confrontation along the
	// route (GDD §9.3).
	Stance StanceOrder

	// Items lists discards declared this round, up to the hand limit of 3
	// (GDD §9.4).
	Items []ItemDiscard

	// AddOns are the two order add-ons (GDD §9.5).
	AddOns AddOns

	// AbandonCargo drops any carried cargo at the ending node, free and
	// costing no action (GDD §9.3).
	AbandonCargo bool

	// ContractChoice answers this round's Phase 2 offer, if one is pending
	// (GDD §8.1-8.2; D29): nil declines, and is ignored entirely when no
	// offer is pending this round; a non-nil value in 0-len(offer)-1
	// accepts the correspondingly-indexed contract. It is not one of GDD
	// §9's five order fields — the underlying rule is already fully
	// specified there — this only carries that same-round decision
	// through the one input surface a round has, mirroring the *NodeID
	// "unset" shape AddOns.OpenDoorsMarket already uses below. An absent
	// or out-of-range value against a pending offer is a GDD §15.0
	// illegal payload, degrading to decline rather than a partial accept.
	ContractChoice *int
}

// PushingOn is GDD §9.1's blind continuation: a step count of 0-2 past a
// route's terminal Hidden node, plus a sector bias.
type PushingOn struct {
	// Steps is the blind step count, legally 0-2.
	Steps int

	// Bias is the declared sector to steer toward, evaluated by the priority
	// ladder in GDD §9.1. Nil means "none" — a plain random walk with no
	// backtracking, per GDD §9.1's own equivalence of the two.
	Bias *Sector
}

// Equal reports whether p and other declare the same Pushing On, including a
// deep comparison of Bias (a *Sector cannot be compared with ==).
func (p PushingOn) Equal(other PushingOn) bool {
	if p.Steps != other.Steps {
		return false
	}
	if (p.Bias == nil) != (other.Bias == nil) {
		return false
	}
	return p.Bias == nil || *p.Bias == *other.Bias
}

// ActionOrder is GDD §9.2's action field. Item is meaningful only when
// Kind is ActionDeal, selecting which of the Black Market's three rolled
// items to buy; every other ActionKind takes no target, since GDD §9.2 always
// resolves them against the route's final node rather than a declared one.
type ActionOrder struct {
	Kind ActionKind
	Item ItemID
}

// StanceOrder is GDD §9.3's stance field. Stake is meaningful only when
// Stance is StanceAggressive — Neutral and Evasive fix it at 0 by rule, but
// this type does not enforce that; it is a validation concern for
// internal/rules, not a representation concern here.
type StanceOrder struct {
	Stance Stance
	Stake  int // Cr$ 0-6
}

// ItemDiscard declares one item discarded this round (GDD §9.4). Target is
// populated for Police Band (a node), Decoy (a Known node), and Bolt Hole (a
// destination 2 steps away); it is the zero NodeID for every other item,
// which takes none.
type ItemDiscard struct {
	Item   ItemID
	Target NodeID
}

// AddOns are GDD §9.5's two checkboxes, plus Open Doors' pre-declaration
// (GDD §14.3, D14 §4). RenewBlocks is 0 when no lease renewal is requested —
// legal purchases are 1-4 blocks (GDD §10.4), so 0 is never a real value and
// needs no separate flag or pointer.
type AddOns struct {
	BuyLedger   bool
	RenewPost   NodeID
	RenewBlocks int

	// OpenDoorsMarket and OpenDoorsItem are Open Doors' own pre-
	// declaration (D14 §4, Option C): "there is no moment at which to
	// choose" once the incident is known, so — exactly like Bolt Hole's
	// own item-declaration shape — the player names, up front, a Black
	// Market node they currently have sight of and an item from its
	// current stock. OpenDoorsMarket nil means "not declared"; NodeID's
	// own zero value is a real node, so unlike OpenDoorsItem (whose zero
	// value is already this package's reserved-invalid convention,
	// enums.go) this one needs a pointer to carry "unset" at all. Legal
	// accepts the declaration unconditionally (D14: every failure mode —
	// no sight, stock moved, hand full — is a resolution-time fact, not
	// one knowable at submission).
	OpenDoorsMarket *NodeID
	OpenDoorsItem   ItemID
}

// Equal reports whether o and other are the same order. Order is not
// comparable with == because Route and Items are slices; this is the
// documented equality helper the fold and the degradation path both need
// (#67).
func (o Order) Equal(other Order) bool {
	if (o.ContractChoice == nil) != (other.ContractChoice == nil) {
		return false
	}
	if o.ContractChoice != nil && *o.ContractChoice != *other.ContractChoice {
		return false
	}
	return o.Round == other.Round &&
		slices.Equal(o.Route, other.Route) &&
		o.PushingOn.Equal(other.PushingOn) &&
		o.Action == other.Action &&
		o.Stance == other.Stance &&
		slices.Equal(o.Items, other.Items) &&
		o.AddOns.Equal(other.AddOns) &&
		o.AbandonCargo == other.AbandonCargo
}

// Equal reports whether a and other declare the same add-ons, including a
// deep comparison of OpenDoorsMarket (a *NodeID cannot be compared with ==:
// two orders declaring the identical node would otherwise compare unequal
// merely for holding different pointers to it).
func (a AddOns) Equal(other AddOns) bool {
	if a.BuyLedger != other.BuyLedger || a.RenewPost != other.RenewPost || a.RenewBlocks != other.RenewBlocks {
		return false
	}
	if a.OpenDoorsItem != other.OpenDoorsItem {
		return false
	}
	if (a.OpenDoorsMarket == nil) != (other.OpenDoorsMarket == nil) {
		return false
	}
	return a.OpenDoorsMarket == nil || *a.OpenDoorsMarket == *other.OpenDoorsMarket
}
