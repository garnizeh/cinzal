package game_test

import (
	"encoding/json"
	mathrand "math/rand/v2"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is D44/D47's round-trip property, for the game-package half of
// the codec (the struct tags and the four enum MarshalJSON/UnmarshalJSON
// pairs). It asserts decode(encode(x)) == x directly against
// encoding/json, with no database involved — internal/store/orderlog's own
// integration tests (issue #317) cover the same property end to end through
// AppendOrder/Load against real Postgres, plus DisallowUnknownFields, which
// is a store-level decode option rather than anything Order itself owns.

// randomOrder builds a structurally-arbitrary Order: it does not need to be
// Legal (this file tests the codec, not internal/rules' validation), only
// to exercise every field, including the corners the codec cares about —
// nil vs. non-nil-empty slices, a nil PushingOn.Bias vs. a real one, every
// enum's reserved-invalid zero vs. a real constant, and ContractChoice both
// nil and set.
func randomOrder(gen *mathrand.Rand) game.Order {
	o := game.Order{Round: game.RoundNumber(1 + gen.IntN(15))}

	switch gen.IntN(3) {
	case 0:
		o.Route = nil
	case 1:
		o.Route = []game.NodeID{}
	default:
		n := 1 + gen.IntN(5)
		o.Route = make([]game.NodeID, n)
		for i := range o.Route {
			o.Route[i] = game.NodeID(gen.IntN(200))
		}
	}

	o.PushingOn = game.PushingOn{Steps: gen.IntN(3)}
	if gen.IntN(2) == 0 {
		s := []game.Sector{game.SectorOldDocks, game.SectorIronLow, game.SectorMistHeights, game.SectorNorthVale}[gen.IntN(4)]
		o.PushingOn.Bias = &s
	}

	actionKinds := []game.ActionKind{
		0, // the reserved-invalid zero, omitted on the wire (D47)
		game.ActionPickup, game.ActionDeliver, game.ActionStakePost,
		game.ActionDeal, game.ActionVanish, game.ActionSurveil, game.ActionNothing,
	}
	o.Action.Kind = actionKinds[gen.IntN(len(actionKinds))]
	if o.Action.Kind == game.ActionDeal && gen.IntN(4) != 0 {
		o.Action.Item = randomItemID(gen)
	}

	stances := []game.Stance{0, game.StanceAggressive, game.StanceNeutral, game.StanceEvasive}
	o.Stance.Stance = stances[gen.IntN(len(stances))]
	if o.Stance.Stance == game.StanceAggressive {
		o.Stance.Stake = gen.IntN(7)
	}

	switch gen.IntN(3) {
	case 0:
		o.Items = nil
	case 1:
		o.Items = []game.ItemDiscard{}
	default:
		n := 1 + gen.IntN(3)
		o.Items = make([]game.ItemDiscard, n)
		for i := range o.Items {
			o.Items[i] = game.ItemDiscard{Item: randomItemID(gen), Target: game.NodeID(gen.IntN(200))}
		}
	}

	o.AddOns.BuyLedger = gen.IntN(2) == 0
	o.AddOns.RenewPost = game.NodeID(gen.IntN(200))
	o.AddOns.RenewBlocks = gen.IntN(5)
	if gen.IntN(2) == 0 {
		n := game.NodeID(gen.IntN(200))
		o.AddOns.OpenDoorsMarket = &n
		o.AddOns.OpenDoorsItem = randomItemID(gen)
	}

	o.AbandonCargo = gen.IntN(2) == 0

	if gen.IntN(2) == 0 {
		c := gen.IntN(4)
		o.ContractChoice = &c
	}

	return o
}

func randomItemID(gen *mathrand.Rand) game.ItemID {
	items := []game.ItemID{
		game.ItemShiv, game.ItemMuscle, game.ItemPoliceBand, game.ItemCirculationPermit,
		game.ItemTornMap, game.ItemDecoy, game.ItemBoltHole, game.ItemGuardContact,
	}
	return items[gen.IntN(len(items))]
}

// TestOrderRoundTripArbitrary is D44 §3's required property: for arbitrary
// generated values, decode(encode(x)) must equal x, across every field.
// The "fails closed" acceptance criterion (issue #317): the generator must
// be proven, not assumed, to have produced at least one order with a
// non-empty Route, a non-zero Action.Kind and a non-empty Items — fifteen
// zero-valued orders round-tripping perfectly would prove nothing.
func TestOrderRoundTripArbitrary(t *testing.T) {
	gen := mathrand.New(mathrand.NewPCG(1, 2))

	var sawNonEmptyRoute, sawNonZeroAction, sawNonEmptyItems bool

	const n = 5000
	for i := 0; i < n; i++ {
		want := randomOrder(gen)

		b, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("iteration %d: Marshal(%+v): %v", i, want, err)
		}

		var got game.Order
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("iteration %d: Unmarshal(%s): %v", i, b, err)
		}

		if !want.Equal(got) {
			t.Fatalf("iteration %d: round trip not equal.\n want: %+v\n got:  %+v\n wire: %s", i, want, got, b)
		}

		if len(want.Route) > 0 {
			sawNonEmptyRoute = true
		}
		if want.Action.Kind != 0 {
			sawNonZeroAction = true
		}
		if len(want.Items) > 0 {
			sawNonEmptyItems = true
		}
	}

	if !sawNonEmptyRoute {
		t.Fatal("fails closed: the generator never produced a non-empty Route across all iterations")
	}
	if !sawNonZeroAction {
		t.Fatal("fails closed: the generator never produced a non-zero Action.Kind across all iterations")
	}
	if !sawNonEmptyItems {
		t.Fatal("fails closed: the generator never produced non-empty Items across all iterations")
	}
}

// TestOrderRoundTripNilVsEmptySlicesAndPointer asserts the nil-vs-empty
// distinction explicitly, per issue #317's acceptance criteria: a nil slice
// (or nil *Sector) comes back nil, and an explicitly-empty-but-non-nil
// slice comes back empty-but-non-nil — both directions checked, since
// Route/Items carry no omitempty tag specifically to keep this distinction
// crossing the wire (order.go).
func TestOrderRoundTripNilVsEmptySlicesAndPointer(t *testing.T) {
	tests := []struct {
		name string
		o    game.Order
	}{
		{"nil Route", game.Order{Route: nil, Items: []game.ItemDiscard{}}},
		{"empty non-nil Route", game.Order{Route: []game.NodeID{}, Items: []game.ItemDiscard{}}},
		{"nil Items", game.Order{Route: []game.NodeID{}, Items: nil}},
		{"empty non-nil Items", game.Order{Route: []game.NodeID{}, Items: []game.ItemDiscard{}}},
		{"nil PushingOn.Bias", game.Order{Route: []game.NodeID{}, Items: []game.ItemDiscard{}, PushingOn: game.PushingOn{Bias: nil}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.o)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got game.Order
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", b, err)
			}

			if (got.Route == nil) != (tt.o.Route == nil) {
				t.Errorf("Route nil-ness not preserved: want nil=%v, got nil=%v (wire: %s)", tt.o.Route == nil, got.Route == nil, b)
			}
			if (got.Items == nil) != (tt.o.Items == nil) {
				t.Errorf("Items nil-ness not preserved: want nil=%v, got nil=%v (wire: %s)", tt.o.Items == nil, got.Items == nil, b)
			}
			if (got.PushingOn.Bias == nil) != (tt.o.PushingOn.Bias == nil) {
				t.Errorf("PushingOn.Bias nil-ness not preserved: want nil=%v, got nil=%v (wire: %s)", tt.o.PushingOn.Bias == nil, got.PushingOn.Bias == nil, b)
			}
		})
	}
}

// TestOrderRoundTripOrdinaryCaseZeroEnums is D47's required fixture: the
// *ordinary* order shape, not a corner case — a real Action.Kind
// (ActionNothing) with no declared Item, no Open Doors declaration, and no
// Pushing On bias. Most real orders look exactly like this, and D47 exists
// specifically because this shape is what most of D44's own audit missed.
func TestOrderRoundTripOrdinaryCaseZeroEnums(t *testing.T) {
	want := game.Order{
		Round:  3,
		Route:  []game.NodeID{},
		Action: game.ActionOrder{Kind: game.ActionNothing}, // Item left zero/unset
		Stance: game.StanceOrder{Stance: game.StanceEvasive},
		Items:  []game.ItemDiscard{},
		AddOns: game.AddOns{}, // OpenDoorsItem left zero/unset, OpenDoorsMarket nil
	}

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got game.Order
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal(%s): %v", b, err)
	}

	if !want.Equal(got) {
		t.Fatalf("ordinary-case round trip not equal.\n want: %+v\n got:  %+v\n wire: %s", want, got, b)
	}
}

// TestEnumWireDivergesFromString proves — not merely declares in prose —
// that the frozen wire table backing MarshalJSON/UnmarshalJSON is a
// different table from String(), per D44's Reasoning: a future cosmetic
// rename of String()'s display text must never change what a stored order
// means. If MarshalJSON ever started delegating to String(), this test
// catches it immediately rather than waiting for a display-text change to
// silently reinterpret history.
func TestEnumWireDivergesFromString(t *testing.T) {
	// game.ActionStakePost: display text has a space and Title Case
	// ("Stake Post"); the wire literal is snake_case ("stake_post"). If
	// these two ever printed the same string, that would not itself prove
	// independence — this assertion is why the two are chosen to differ.
	// Exercised through ActionOrder (order_wire.go), not ActionKind
	// directly: ActionKind itself has no MarshalJSON (see enums_wire.go's
	// own doc comment for why giving the enum type one would leak into
	// game.Event/PlayerView's unrelated JSON encoding).
	b, err := json.Marshal(game.ActionOrder{Kind: game.ActionStakePost})
	if err != nil {
		t.Fatalf("Marshal(ActionOrder{Kind: ActionStakePost}): %v", err)
	}
	wire := string(b)
	display := game.ActionStakePost.String()
	if strings.Contains(wire, display) {
		t.Fatalf("wire encoding (%s) contains String() output (%s) — the two tables have not diverged", wire, display)
	}
	if wire != `{"kind":"stake_post"}` {
		t.Fatalf("ActionOrder{Kind: ActionStakePost} wire encoding = %s, want {\"kind\":\"stake_post\"}", wire)
	}

	// And decode still works purely off the frozen table, independent of
	// whatever String() says today.
	var got game.ActionOrder
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal(%s): %v", b, err)
	}
	if got.Kind != game.ActionStakePost {
		t.Fatalf("decoded %v, want ActionStakePost", got.Kind)
	}
}

// TestOrderFrozenFixtureHandWrittenText decodes a literal, hand-written JSON
// payload (not generated from the live struct — D44 §3) against the
// expected Go value, exercising the actual wire strings a reader would see
// in a real orders.payload row.
func TestOrderFrozenFixtureHandWrittenText(t *testing.T) {
	const wire = `{
		"round": 7,
		"route": [3, 8, 12],
		"pushing_on": {"steps": 2, "bias": "mist_heights"},
		"action": {"kind": "deal", "item": "torn_map"},
		"stance": {"stance": "aggressive", "stake": 4},
		"items": [{"item": "shiv", "target": 0}],
		"add_ons": {"buy_ledger": true, "renew_post": 5, "renew_blocks": 2, "open_doors_market": 9, "open_doors_item": "muscle"},
		"abandon_cargo": true,
		"contract_choice": 1
	}`

	var got game.Order
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("Unmarshal frozen fixture: %v", err)
	}

	bias := game.SectorMistHeights
	market := game.NodeID(9)
	choice := 1
	want := game.Order{
		Round:     7,
		Route:     []game.NodeID{3, 8, 12},
		PushingOn: game.PushingOn{Steps: 2, Bias: &bias},
		Action:    game.ActionOrder{Kind: game.ActionDeal, Item: game.ItemTornMap},
		Stance:    game.StanceOrder{Stance: game.StanceAggressive, Stake: 4},
		Items:     []game.ItemDiscard{{Item: game.ItemShiv, Target: 0}},
		AddOns: game.AddOns{
			BuyLedger: true, RenewPost: 5, RenewBlocks: 2,
			OpenDoorsMarket: &market, OpenDoorsItem: game.ItemMuscle,
		},
		AbandonCargo:   true,
		ContractChoice: &choice,
	}

	if !want.Equal(got) {
		t.Fatalf("frozen fixture decoded to\n%+v\nwant\n%+v", got, want)
	}
}

// TestEnumUnmarshalRejectsUnrecognizedString is D44's core strictness rule:
// a present, non-null string that matches no table entry is a decode error,
// never a silent fall-through to the reserved-invalid zero. Exercised
// through each field's owning wrapper struct (order_wire.go) — the enum
// types themselves have no UnmarshalJSON (see enums_wire.go's own doc
// comment for why).
func TestEnumUnmarshalRejectsUnrecognizedString(t *testing.T) {
	t.Run("ActionOrder.Kind", func(t *testing.T) {
		var v game.ActionOrder
		if err := json.Unmarshal([]byte(`{"kind":"teleport"}`), &v); err == nil {
			t.Fatal("want a decode error for an unrecognized ActionKind string, got nil")
		}
	})
	t.Run("StanceOrder.Stance", func(t *testing.T) {
		var v game.StanceOrder
		if err := json.Unmarshal([]byte(`{"stance":"berserk","stake":0}`), &v); err == nil {
			t.Fatal("want a decode error for an unrecognized Stance string, got nil")
		}
	})
	t.Run("ItemDiscard.Item", func(t *testing.T) {
		var v game.ItemDiscard
		if err := json.Unmarshal([]byte(`{"item":"rocket_launcher","target":0}`), &v); err == nil {
			t.Fatal("want a decode error for an unrecognized ItemID string, got nil")
		}
	})
	t.Run("ActionOrder.Item empty string is rejected, not treated as unset", func(t *testing.T) {
		// D47 §3's "absence or null means unset" does not extend to an
		// explicit empty string — that is a third, malformed case, and
		// must still fail strict lookup.
		var v game.ActionOrder
		if err := json.Unmarshal([]byte(`{"kind":"deal","item":""}`), &v); err == nil {
			t.Fatal("want a decode error for item:\"\" (present but empty), got nil")
		}
	})
	t.Run("PushingOn.Bias unrecognized string", func(t *testing.T) {
		var v game.PushingOn
		if err := json.Unmarshal([]byte(`{"steps":0,"bias":"downtown"}`), &v); err == nil {
			t.Fatal("want a decode error for an unrecognized Sector string, got nil")
		}
	})
	t.Run("PushingOn.Bias rejects a bare number", func(t *testing.T) {
		// D47 §5: Sector is string-typed on the wire; a bare number must
		// fail as a type mismatch, not decode as the reserved-invalid
		// zero.
		var v game.PushingOn
		if err := json.Unmarshal([]byte(`{"steps":0,"bias":0}`), &v); err == nil {
			t.Fatal("want a decode error for a bare-number bias, got nil")
		}
	})
	t.Run("unknown key inside a wrapper struct is rejected", func(t *testing.T) {
		// D44: DisallowUnknownFields is restated inside every wrapper
		// struct's own UnmarshalJSON, not just at the outer Order decode.
		var v game.ActionOrder
		if err := json.Unmarshal([]byte(`{"kind":"deal","item":"shiv","surprise":true}`), &v); err == nil {
			t.Fatal("want a decode error for an unknown key inside action, got nil")
		}
	})
}

// TestWrapperStructsOmitReservedZeroKey is D47's actual encode-side rule:
// ActionKind/Stance/ItemID at their reserved-invalid zero omit the wire key
// entirely (never a named literal, never an error) — the ordinary case for
// most real orders.
func TestWrapperStructsOmitReservedZeroKey(t *testing.T) {
	b, err := json.Marshal(game.ActionOrder{}) // Kind and Item both zero
	if err != nil {
		t.Fatalf("Marshal(ActionOrder{}): %v", err)
	}
	if string(b) != `{}` {
		t.Fatalf("Marshal(ActionOrder{}) = %s, want {} (both keys omitted)", b)
	}

	b, err = json.Marshal(game.StanceOrder{}) // Stance zero, Stake zero
	if err != nil {
		t.Fatalf("Marshal(StanceOrder{}): %v", err)
	}
	if string(b) != `{"stake":0}` {
		t.Fatalf(`Marshal(StanceOrder{}) = %s, want {"stake":0} (stance key omitted)`, b)
	}

	b, err = json.Marshal(game.AddOns{}) // OpenDoorsItem zero, OpenDoorsMarket nil
	if err != nil {
		t.Fatalf("Marshal(AddOns{}): %v", err)
	}
	if string(b) != `{"buy_ledger":false,"renew_post":0,"renew_blocks":0,"open_doors_market":null}` {
		t.Fatalf("Marshal(AddOns{}) = %s, want open_doors_item omitted and open_doors_market explicit null", b)
	}
}

// TestPushingOnBiasRejectsUnrepresentableValue is D47 §5's actual encode
// error case: Sector's reserved-invalid zero (and any value outside its
// four named constants) has no wire representation at all — unreachable
// through the wire itself (Bias is *Sector; nil already means unset), but
// constructible directly in Go, e.g. by a bug in code that builds an
// Order. Encoding such a value is a hard error rather than a silently
// written undefined value.
func TestPushingOnBiasRejectsUnrepresentableValue(t *testing.T) {
	zero := game.Sector(0)
	if _, err := json.Marshal(game.PushingOn{Bias: &zero}); err == nil {
		t.Fatal("want an encode error for PushingOn.Bias pointing at the reserved-invalid Sector zero, got nil")
	}

	outOfRange := game.Sector(99)
	if _, err := json.Marshal(game.PushingOn{Bias: &outOfRange}); err == nil {
		t.Fatal("want an encode error for PushingOn.Bias pointing at an out-of-range Sector, got nil")
	}
}
