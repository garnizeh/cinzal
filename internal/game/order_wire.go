package game

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// This file is D44's Q4/D47, implemented at the level that actually
// contains the change: the five struct types order.go declares that exist
// only inside game.Order (PushingOn, ActionOrder, StanceOrder, ItemDiscard,
// AddOns) each get their own MarshalJSON/UnmarshalJSON here, built on the
// frozen wire tables and lookup functions in enums_wire.go. See that file's
// own doc comment for why the codec lives here — on Order's own wrapper
// structs — rather than as methods on ActionKind/Stance/ItemID/Sector
// themselves: those four types are also used by game.Event and
// game.PlayerView, and a method on the type would apply there too, silently
// changing internal/rules' pre-existing determinism/golden-fixture JSON
// hashing along with it.
//
// Each type below decodes through its own private "wire" mirror struct with
// string-typed enum fields, then converts through enums_wire.go's lookup
// functions — never through the affected type's own encoding/json
// reflection, since these types have no MarshalJSON of their own. A pointer
// (*string), not a plain string, is used for every wire-table field: this
// is what lets UnmarshalJSON tell "the key was absent, or present as JSON
// null" (nil pointer, D47 §3's legal "unset") apart from "the key was
// present with the literal empty string" (non-nil pointer to ""), which is
// neither absence nor null and must still fail strict lookup rather than
// silently reading as unset.
//
// Every wire mirror struct's own UnmarshalJSON runs through a fresh
// json.Decoder with DisallowUnknownFields set: once a type owns its own
// UnmarshalJSON, the outer decoder's DisallowUnknownFields setting
// (internal/store/orderlog's decode of the whole Order) no longer reaches
// inside it automatically — D44's corruption guard has to be restated at
// this level explicitly, or a stray key inside "action": {...} (for
// example) would decode silently instead of erroring.

// PushingOn

type pushingOnWire struct {
	Steps int     `json:"steps"`
	Bias  *string `json:"bias"`
}

// MarshalJSON encodes p. Bias nil -> JSON null; Bias pointing at a value
// with no sectorWire entry (D47 §5's unreachable-through-the-wire case) is
// a hard encode error rather than a silently written undefined value.
func (p PushingOn) MarshalJSON() ([]byte, error) {
	w := pushingOnWire{Steps: p.Steps}
	if p.Bias != nil {
		s, ok := sectorToWire(*p.Bias)
		if !ok {
			return nil, fmt.Errorf("game: PushingOn.Bias has no wire representation for value %d", uint8(*p.Bias))
		}
		w.Bias = &s
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes strictly: an absent or null bias decodes to nil; a
// present string not in sectorWire is a decode error.
func (p *PushingOn) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w pushingOnWire
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("game: decode PushingOn: %w", err)
	}

	p.Steps = w.Steps
	p.Bias = nil
	if w.Bias != nil {
		v, ok := sectorFromWire(*w.Bias)
		if !ok {
			return fmt.Errorf("game: PushingOn.Bias: unrecognized wire value %q", *w.Bias)
		}
		p.Bias = &v
	}
	return nil
}

// ActionOrder

type actionOrderWire struct {
	Kind *string `json:"kind,omitempty"`
	Item *string `json:"item,omitempty"`
}

// MarshalJSON encodes a. Kind/Item at their reserved-invalid zero omit the
// key entirely (D47) — the ordinary case for every order whose Kind is not
// ActionDeal.
func (a ActionOrder) MarshalJSON() ([]byte, error) {
	var w actionOrderWire
	if a.Kind != 0 {
		s, ok := actionKindToWire(a.Kind)
		if !ok {
			return nil, fmt.Errorf("game: ActionOrder.Kind has no wire representation for value %d", uint8(a.Kind))
		}
		w.Kind = &s
	}
	if a.Item != 0 {
		s, ok := itemIDToWire(a.Item)
		if !ok {
			return nil, fmt.Errorf("game: ActionOrder.Item has no wire representation for value %d", uint8(a.Item))
		}
		w.Item = &s
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes strictly: an absent or null field decodes to the
// reserved-invalid zero (D47 §3); a present string matching no table entry
// — including the empty string, which is never a table entry — is a
// decode error.
func (a *ActionOrder) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w actionOrderWire
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("game: decode ActionOrder: %w", err)
	}

	*a = ActionOrder{}
	if w.Kind != nil {
		v, ok := actionKindFromWire(*w.Kind)
		if !ok {
			return fmt.Errorf("game: ActionOrder.Kind: unrecognized wire value %q", *w.Kind)
		}
		a.Kind = v
	}
	if w.Item != nil {
		v, ok := itemIDFromWire(*w.Item)
		if !ok {
			return fmt.Errorf("game: ActionOrder.Item: unrecognized wire value %q", *w.Item)
		}
		a.Item = v
	}
	return nil
}

// StanceOrder

type stanceOrderWire struct {
	Stance *string `json:"stance,omitempty"`
	Stake  int     `json:"stake"`
}

// MarshalJSON encodes s. Stance at its reserved-invalid zero omits the key
// (D47) — only reachable on a malformed payload (GDD §15.0), but governed
// by the same rule as every other enum field reachable from Order.
func (s StanceOrder) MarshalJSON() ([]byte, error) {
	w := stanceOrderWire{Stake: s.Stake}
	if s.Stance != 0 {
		v, ok := stanceToWire(s.Stance)
		if !ok {
			return nil, fmt.Errorf("game: StanceOrder.Stance has no wire representation for value %d", uint8(s.Stance))
		}
		w.Stance = &v
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes strictly, per ActionOrder's own doc comment above.
func (s *StanceOrder) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w stanceOrderWire
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("game: decode StanceOrder: %w", err)
	}

	s.Stake = w.Stake
	s.Stance = 0
	if w.Stance != nil {
		v, ok := stanceFromWire(*w.Stance)
		if !ok {
			return fmt.Errorf("game: StanceOrder.Stance: unrecognized wire value %q", *w.Stance)
		}
		s.Stance = v
	}
	return nil
}

// ItemDiscard

type itemDiscardWire struct {
	Item   *string `json:"item,omitempty"`
	Target NodeID  `json:"target"`
}

// MarshalJSON encodes i. Item at its reserved-invalid zero omits the key
// (D47) — the third of the three ItemID sites reachable from Order, per
// D47's correction of D44's own audit.
func (i ItemDiscard) MarshalJSON() ([]byte, error) {
	w := itemDiscardWire{Target: i.Target}
	if i.Item != 0 {
		s, ok := itemIDToWire(i.Item)
		if !ok {
			return nil, fmt.Errorf("game: ItemDiscard.Item has no wire representation for value %d", uint8(i.Item))
		}
		w.Item = &s
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes strictly, per ActionOrder's own doc comment above.
func (i *ItemDiscard) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w itemDiscardWire
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("game: decode ItemDiscard: %w", err)
	}

	i.Target = w.Target
	i.Item = 0
	if w.Item != nil {
		v, ok := itemIDFromWire(*w.Item)
		if !ok {
			return fmt.Errorf("game: ItemDiscard.Item: unrecognized wire value %q", *w.Item)
		}
		i.Item = v
	}
	return nil
}

// AddOns

type addOnsWire struct {
	BuyLedger       bool    `json:"buy_ledger"`
	RenewPost       NodeID  `json:"renew_post"`
	RenewBlocks     int     `json:"renew_blocks"`
	OpenDoorsMarket *NodeID `json:"open_doors_market"`
	OpenDoorsItem   *string `json:"open_doors_item,omitempty"`
}

// MarshalJSON encodes a. OpenDoorsMarket nil -> JSON null (D44's structural
// pointer answer); OpenDoorsItem at its reserved-invalid zero omits the key
// (D47).
func (a AddOns) MarshalJSON() ([]byte, error) {
	w := addOnsWire{
		BuyLedger:       a.BuyLedger,
		RenewPost:       a.RenewPost,
		RenewBlocks:     a.RenewBlocks,
		OpenDoorsMarket: a.OpenDoorsMarket,
	}
	if a.OpenDoorsItem != 0 {
		s, ok := itemIDToWire(a.OpenDoorsItem)
		if !ok {
			return nil, fmt.Errorf("game: AddOns.OpenDoorsItem has no wire representation for value %d", uint8(a.OpenDoorsItem))
		}
		w.OpenDoorsItem = &s
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes strictly, per ActionOrder's own doc comment above.
// OpenDoorsMarket decodes through encoding/json's own pointer-null handling
// directly (no lookup table involved: NodeID is a bare structural index).
func (a *AddOns) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w addOnsWire
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("game: decode AddOns: %w", err)
	}

	a.BuyLedger = w.BuyLedger
	a.RenewPost = w.RenewPost
	a.RenewBlocks = w.RenewBlocks
	a.OpenDoorsMarket = w.OpenDoorsMarket
	a.OpenDoorsItem = 0
	if w.OpenDoorsItem != nil {
		v, ok := itemIDFromWire(*w.OpenDoorsItem)
		if !ok {
			return fmt.Errorf("game: AddOns.OpenDoorsItem: unrecognized wire value %q", *w.OpenDoorsItem)
		}
		a.OpenDoorsItem = v
	}
	return nil
}
