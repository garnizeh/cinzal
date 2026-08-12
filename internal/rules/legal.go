package rules

import (
	"fmt"
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// handLimit is GDD §12's item-hand cap. No named constant existed for it
// before this file — every other reference to "3" in the GDD is a plain
// prose number, not a config dial, so it is not part of game.Config either.
const handLimit = 3

// Reason identifies why an order is illegal, for machine consumption (RFC
// §11.5: never a rendered sentence — the render layer owns the copy). One
// value per row of GDD §15.0's order-legality table, plus D14/#52's
// Deal-at-hand-limit row.
type Reason uint8

const (
	_ Reason = iota

	// ReasonNonAdjacentStep is a route step between nodes with no edge
	// between them in the player's own view (GDD §15.0).
	ReasonNonAdjacentStep

	// ReasonHiddenNodeNotLast covers three GDD §15.0 rows at once: "more
	// than one Hidden node, or a Hidden node that isn't last", "a step out
	// of a Hidden node without Pushing On declared", and the structural case
	// of Pushing On declared without a Hidden-terminated route. All three
	// reduce to the same check — see Legal's route walk.
	ReasonHiddenNodeNotLast

	// ReasonStepAllowanceExceeded is a route longer than Steps (GDD §9.1,
	// §15.0).
	ReasonStepAllowanceExceeded

	// ReasonIllegalActionAtNode is an action the ending node's type cannot
	// support — Deliver off a Border, Deal off a Black Market (GDD §9.2,
	// §15.0).
	ReasonIllegalActionAtNode

	// ReasonPushingOnWithAction is Pushing On declared alongside a non-
	// Nothing action (GDD §9.1, §15.0).
	ReasonPushingOnWithAction

	// ReasonInsufficientBalance is add-ons or a lease exceeding the balance
	// at submission (GDD §15.0).
	ReasonInsufficientBalance

	// ReasonPostCapExceeded is a stake beyond the post cap (GDD §10.3,
	// §15.0).
	ReasonPostCapExceeded

	// ReasonNodeNotKnown is a route plotted through a node not Known to this
	// player — the one row GDD §15.0 calls out for logging as well as
	// rejection: "a well-formed client cannot produce this."
	ReasonNodeNotKnown

	// ReasonHandLimitExceeded is a Deal that would exceed the hand limit
	// after this order's own Field 4 discards (D14, issue #52).
	ReasonHandLimitExceeded

	// ReasonInvalidDiscard is a declared item discard for an item this seat
	// doesn't hold, or declared more times than the hand holds copies of it
	// (GDD §9.4: "discard any number of items you hold" — not more).
	ReasonInvalidDiscard
)

// String returns the reason's short name, or "Reason(n)" for an invalid
// value.
func (r Reason) String() string {
	switch r {
	case ReasonNonAdjacentStep:
		return "non-adjacent step"
	case ReasonHiddenNodeNotLast:
		return "hidden node not last"
	case ReasonStepAllowanceExceeded:
		return "step allowance exceeded"
	case ReasonIllegalActionAtNode:
		return "action illegal at ending node"
	case ReasonPushingOnWithAction:
		return "pushing on combined with an action"
	case ReasonInsufficientBalance:
		return "insufficient balance"
	case ReasonPostCapExceeded:
		return "post cap exceeded"
	case ReasonNodeNotKnown:
		return "route through a node not known to this player"
	case ReasonHandLimitExceeded:
		return "hand limit exceeded"
	case ReasonInvalidDiscard:
		return "discard for an item not held"
	default:
		return fmt.Sprintf("Reason(%d)", uint8(r))
	}
}

// IllegalOrderError is Legal's rejection: a machine-readable Reason plus a
// human-readable Detail for logs. Callers use errors.As to recover Reason —
// ReasonNodeNotKnown is the row GDD §15.0 says to log specially.
type IllegalOrderError struct {
	Reason Reason
	Detail string
}

func (e *IllegalOrderError) Error() string {
	return fmt.Sprintf("rules: illegal order: %s: %s", e.Reason, e.Detail)
}

func illegal(reason Reason, detail string) error {
	return &IllegalOrderError{Reason: reason, Detail: detail}
}

// Legal answers "would the server accept this order right now?" (RFC §6.1).
// It checks every row of GDD §15.0's order-legality table plus D14/#52's
// hand-limit row. Degradation-category concerns — the world moving out from
// under an order that was legal when submitted, GDD §15.0's "Step 0" — are
// never Legal's job: a target node staked by someone else, an edge destroyed
// mid-round, or (per the same reasoning) whether declared cargo is really on
// the ground for a Pickup, are all resolved at Resolve, not here.
func Legal(v game.PlayerView, o game.Order, cfg game.Config) error {
	if err := legalRoute(v, o); err != nil {
		return err
	}

	if len(o.Route) > Steps(v, cfg) {
		return illegal(ReasonStepAllowanceExceeded,
			fmt.Sprintf("route length %d exceeds step allowance %d", len(o.Route), Steps(v, cfg)))
	}

	if err := legalPushingOn(v, o); err != nil {
		return err
	}

	if err := legalAction(v, o); err != nil {
		return err
	}

	if err := legalBalance(v, o, cfg); err != nil {
		return err
	}

	if err := legalPostCap(v, o, cfg); err != nil {
		return err
	}

	if err := legalDiscards(v, o); err != nil {
		return err
	}

	if err := legalHandLimit(v, o); err != nil {
		return err
	}

	return nil
}

// legalRoute walks the route from the player's current position, checking
// adjacency, that no node visited is merely Rumoured (GDD §7.1: a Rumoured
// node has "no edges, so you cannot plot a route through or into it" — true
// at every position, not just mid-route), and that a genuinely Hidden node
// (absent from the view entirely) appears only as the route's last entry.
func legalRoute(v game.PlayerView, o game.Order) error {
	current := v.You.Position

	for i, to := range o.Route {
		fromView, fromKnown := v.Nodes[current]
		if !fromKnown || fromView.Fog < game.FogKnown {
			return illegal(ReasonNodeNotKnown,
				fmt.Sprintf("route step %d leaves node %d, which is not Known to this player", i, current))
		}

		if !slices.Contains(fromView.Edges, to) {
			return illegal(ReasonNonAdjacentStep,
				fmt.Sprintf("route step %d: node %d has no edge to node %d", i, current, to))
		}

		toView, toKnown := v.Nodes[to]
		switch {
		case toKnown && toView.Fog < game.FogKnown:
			return illegal(ReasonNodeNotKnown,
				fmt.Sprintf("route step %d enters node %d, which is only Rumoured to this player", i, to))
		case !toKnown && i != len(o.Route)-1:
			return illegal(ReasonHiddenNodeNotLast,
				fmt.Sprintf("route step %d enters Hidden node %d, which is not the route's last entry", i, to))
		}

		current = to
	}

	return nil
}

// pushingOnDeclared reports whether o declares a Pushing On continuation at
// all — the zero value (Steps: 0, Bias: nil) means none was.
func pushingOnDeclared(o game.Order) bool {
	return o.PushingOn.Steps != 0 || o.PushingOn.Bias != nil
}

// routeEndsHidden reports whether o's route terminates on a node absent from
// v — the only kind of node Pushing On may extend from (GDD §9.1).
func routeEndsHidden(v game.PlayerView, o game.Order) bool {
	if len(o.Route) == 0 {
		_, known := v.Nodes[v.You.Position]
		return !known
	}
	_, known := v.Nodes[o.Route[len(o.Route)-1]]
	return !known
}

// legalPushingOn checks GDD §9.1/§15.0's two Pushing On rules: it may only
// be declared on a route that ends on a Hidden node, and it may never be
// combined with an action ("you have no idea where you'll be").
func legalPushingOn(v game.PlayerView, o game.Order) error {
	if !pushingOnDeclared(o) {
		return nil
	}

	if !routeEndsHidden(v, o) {
		return illegal(ReasonHiddenNodeNotLast,
			"pushing on declared but the route does not end on a Hidden node")
	}

	if o.Action.Kind != game.ActionNothing {
		return illegal(ReasonPushingOnWithAction,
			fmt.Sprintf("pushing on declared alongside action %s", o.Action.Kind))
	}

	return nil
}

// endingNode returns the order's ending node and whether that node's type
// could be determined from the view — false for a route that ends on a
// Hidden node, since its type is not disclosed to the client at submission
// time and so cannot be validated here (deferred to Resolve).
func endingNode(v game.PlayerView, o game.Order) (game.NodeID, game.NodeType, bool) {
	node := v.You.Position
	if len(o.Route) > 0 {
		node = o.Route[len(o.Route)-1]
	}
	view, known := v.Nodes[node]
	if !known {
		return node, 0, false
	}
	return node, view.Type, true
}

// legalAction checks GDD §15.0's "action illegal at the ending node's type"
// row for the two actions with a static node-type restriction. Pickup is
// deliberately not checked here: GDD §9.2 allows it at "any node with
// dropped cargo you have the contract for", which has no type restriction,
// and PlayerView carries no ground-cargo field to verify cargo presence
// against — its real legality is a Resolve-time/degradation concern.
// Stake Post has no node-type restriction either (any node); ownership
// conflicts are explicitly a degradation case (GDD §15.0: "the target node
// got staked by someone else"), checked at Resolve, not here.
func legalAction(v game.PlayerView, o game.Order) error {
	node, nodeType, known := endingNode(v, o)
	if !known {
		return nil
	}

	switch o.Action.Kind {
	case game.ActionDeliver:
		if nodeType != game.NodeBorder {
			return illegal(ReasonIllegalActionAtNode,
				fmt.Sprintf("deliver at node %d, which is not a Border", node))
		}
		if !deliverableCargo(v, node) {
			return illegal(ReasonIllegalActionAtNode,
				fmt.Sprintf("deliver at node %d with no deliverable cargo held", node))
		}
	case game.ActionDeal:
		if nodeType != game.NodeBlackMarket {
			return illegal(ReasonIllegalActionAtNode,
				fmt.Sprintf("deal at node %d, which is not a Black Market", node))
		}
	}

	return nil
}

// deliverableCargo reports whether v.You is carrying cargo that may be
// delivered at node: unbound loose cargo delivers to any Border (GDD §8.4,
// the Spilled Load boon), while bound cargo needs a held contract whose
// Destination is node (GDD §8.1, §8.4).
func deliverableCargo(v game.PlayerView, node game.NodeID) bool {
	cargo := v.You.Cargo
	if cargo == nil {
		return false
	}
	if !cargo.Bound {
		return true
	}
	for _, c := range v.You.Contracts {
		if c.ID == cargo.Contract && c.Destination == node {
			return true
		}
	}
	return false
}

// legalBalance checks GDD §15.0's "Add-ons or a lease exceeding your balance
// at submission" row. Aggressive stance's stake is deliberately excluded:
// neither the GDD row nor RFC §10.2 name it here — RFC §10.2 row 6 treats
// stake affordability as an affordance clamp, not a submission-time reject.
func legalBalance(v game.PlayerView, o game.Order, cfg game.Config) error {
	cost := 0
	if o.AddOns.BuyLedger {
		cost += cfg.LedgerCost
	}
	if o.AddOns.RenewBlocks > 0 {
		cost += o.AddOns.RenewBlocks * cfg.LeaseCostPerBlock
	}

	if cost > v.You.Balance {
		return illegal(ReasonInsufficientBalance,
			fmt.Sprintf("add-ons cost %d, exceeding balance %d", cost, v.You.Balance))
	}

	return nil
}

// legalPostCap checks GDD §10.3/§15.0's post cap row.
func legalPostCap(v game.PlayerView, o game.Order, cfg game.Config) error {
	if o.Action.Kind != game.ActionStakePost {
		return nil
	}

	players := len(v.Others) + 1
	if _, ok := cfg.PostCapByPlayers[players]; !ok {
		return illegal(ReasonPostCapExceeded,
			fmt.Sprintf("no post cap configured for %d players", players))
	}

	if postCap, reached := PostCapReached(len(v.You.Posts), players, cfg); reached {
		return illegal(ReasonPostCapExceeded,
			fmt.Sprintf("already holding %d posts, at the cap of %d", len(v.You.Posts), postCap))
	}

	return nil
}

// legalDiscards checks that every declared item discard (GDD §9.4) names an
// item this seat actually holds, and that no item is discarded more times
// than the hand holds copies of it. This must run before legalHandLimit,
// which otherwise trusts len(o.Items) at face value — a discard for an item
// not held, or the same item declared twice over, would let a fabricated
// order free hand slots that were never really occupied.
func legalDiscards(v game.PlayerView, o game.Order) error {
	held := make(map[game.ItemID]int, len(v.You.Items))
	for _, item := range v.You.Items {
		held[item]++
	}

	for _, d := range o.Items {
		held[d.Item]--
		if held[d.Item] < 0 {
			return illegal(ReasonInvalidDiscard,
				fmt.Sprintf("discard declares item %s beyond what this seat holds", d.Item))
		}
	}

	return nil
}

// legalHandLimit checks D14/#52's rule: a Deal is illegal if, after this
// order's own Field 4 discards free up slots, the resulting hand would
// exceed handLimit. legalDiscards must run first — it is what makes
// len(o.Items) trustworthy here.
func legalHandLimit(v game.PlayerView, o game.Order) error {
	if o.Action.Kind != game.ActionDeal {
		return nil
	}

	handAfterDiscards := len(v.You.Items) - len(o.Items) + 1
	if handAfterDiscards > handLimit {
		return illegal(ReasonHandLimitExceeded,
			fmt.Sprintf("dealing would leave a hand of %d, exceeding the limit of %d", handAfterDiscards, handLimit))
	}

	return nil
}
