package game

// This file is D44's Q4/D47: the wire vocabulary for every iota-based enum
// reachable from Order (ActionKind, Stance, ItemID, Sector) — a frozen
// table declared here rather than derived from String() (enums.go).
// String() stays free to change for display or debug text; the tables
// below are what a stored orders.payload row is allowed to mean, frozen
// the moment the first real row uses them.
//
// # Why these functions, and not a MarshalJSON/UnmarshalJSON pair on the
// # enum types themselves
//
// An earlier version of this file put MarshalJSON/UnmarshalJSON directly on
// ActionKind, Stance, ItemID and Sector. That is wrong: a method on a type
// applies to *every* use of that type, and all four are also fields of
// game.Event (event.go) and game.PlayerView/NodeView (view.go) — neither of
// which has anything to do with orders.payload. internal/rules' own
// pre-existing determinism/golden-fixture suite calls json.Marshal directly
// on MatchState, []game.Event and PlayerView (determinism_test.go,
// bots_determinism_external_test.go, fog_test.go) as its committed hashing
// mechanism; giving these enum types their own MarshalJSON silently changed
// what every one of those calls produces — including erroring outright on
// the ordinary case, since D47's reserved-invalid zero is common in Event
// too. That is the exact silent-reinterpretation failure D44 exists to
// prevent, just relocated onto test infrastructure this decision was never
// meant to touch.
//
// The fix: these four types keep their plain, un-adorned Go representation
// (a bare integer) everywhere else in the codebase, exactly as before this
// issue. The wire *string* representation D44/D47 specify is instead
// implemented once, per Order-owned wrapper struct (ActionOrder,
// StanceOrder, ItemDiscard, AddOns, PushingOn — order.go), each of which
// exists only inside game.Order and is never reused by Event or
// PlayerView. Those wrapper structs' own MarshalJSON/UnmarshalJSON call the
// lookup functions below directly; nothing here is a method, so nothing
// here can leak into an unrelated type's default JSON encoding.
//
// No generics: scripts/check-game-types.go enforces D01's "no type
// parameter" rule inside internal/game unconditionally (constrained or
// not), so each type's table and lookup pair is written out rather than
// shared through one generic helper.

// actionKindWire is ActionKind's frozen wire table (GDD §9.2). Deliberately
// snake_case and lowercase, unrelated to String()'s Title Case display text
// ("Stake Post") — proof, not just prose, that the two are different code
// paths (see order_codec_test.go).
var actionKindWire = [...]struct {
	value ActionKind
	wire  string
}{
	{ActionPickup, "pickup"},
	{ActionDeliver, "deliver"},
	{ActionStakePost, "stake_post"},
	{ActionDeal, "deal"},
	{ActionVanish, "vanish"},
	{ActionSurveil, "surveil"},
	{ActionNothing, "nothing"},
}

func actionKindToWire(v ActionKind) (string, bool) {
	for _, e := range actionKindWire {
		if e.value == v {
			return e.wire, true
		}
	}
	return "", false
}

func actionKindFromWire(s string) (ActionKind, bool) {
	for _, e := range actionKindWire {
		if e.wire == s {
			return e.value, true
		}
	}
	return 0, false
}

// stanceWire is Stance's frozen wire table (GDD §9.3).
var stanceWire = [...]struct {
	value Stance
	wire  string
}{
	{StanceAggressive, "aggressive"},
	{StanceNeutral, "neutral"},
	{StanceEvasive, "evasive"},
}

func stanceToWire(v Stance) (string, bool) {
	for _, e := range stanceWire {
		if e.value == v {
			return e.wire, true
		}
	}
	return "", false
}

func stanceFromWire(s string) (Stance, bool) {
	for _, e := range stanceWire {
		if e.wire == s {
			return e.value, true
		}
	}
	return 0, false
}

// itemIDWire is ItemID's frozen wire table (GDD §12).
var itemIDWire = [...]struct {
	value ItemID
	wire  string
}{
	{ItemShiv, "shiv"},
	{ItemMuscle, "muscle"},
	{ItemPoliceBand, "police_band"},
	{ItemCirculationPermit, "circulation_permit"},
	{ItemTornMap, "torn_map"},
	{ItemDecoy, "decoy"},
	{ItemBoltHole, "bolt_hole"},
	{ItemGuardContact, "guard_contact"},
}

// itemIDToWire backs all three Order-reachable ItemID sites D47 corrected
// D44's audit to name: ActionOrder.Item, AddOns.OpenDoorsItem,
// ItemDiscard.Item.
func itemIDToWire(v ItemID) (string, bool) {
	for _, e := range itemIDWire {
		if e.value == v {
			return e.wire, true
		}
	}
	return "", false
}

func itemIDFromWire(s string) (ItemID, bool) {
	for _, e := range itemIDWire {
		if e.wire == s {
			return e.value, true
		}
	}
	return 0, false
}

// sectorWire is Sector's frozen wire table (GDD §3).
var sectorWire = [...]struct {
	value Sector
	wire  string
}{
	{SectorOldDocks, "old_docks"},
	{SectorIronLow, "iron_low"},
	{SectorMistHeights, "mist_heights"},
	{SectorNorthVale, "north_vale"},
}

// sectorToWire backs PushingOn.Bias's only Order-reachable site. D47 §5:
// Sector's zero has no wire representation at all (the table below has no
// zero entry), which the caller (PushingOn's own MarshalJSON, order.go)
// turns into a hard encode-time error if it is ever asked to encode one —
// unreachable through the wire itself (Bias is *Sector, nil already means
// unset), but not excluded by Go's type system from direct construction.
func sectorToWire(v Sector) (string, bool) {
	for _, e := range sectorWire {
		if e.value == v {
			return e.wire, true
		}
	}
	return "", false
}

func sectorFromWire(s string) (Sector, bool) {
	for _, e := range sectorWire {
		if e.wire == s {
			return e.value, true
		}
	}
	return 0, false
}
