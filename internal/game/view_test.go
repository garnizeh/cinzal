package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestNodesIsKeyedNotFlagged pins PlayerView.Nodes to a map keyed by NodeID.
// Issue #55: "A map with missing keys cannot express [the leak of a
// zero-valued hidden node]; a slice of all nodes with a visibility flag
// invites it on the first refactor. Choose the representation that makes
// the leak unspellable." This guards against exactly that refactor.
func TestNodesIsKeyedNotFlagged(t *testing.T) {
	field, ok := reflect.TypeFor[PlayerView]().FieldByName("Nodes")
	if !ok {
		t.Fatal("PlayerView has no Nodes field")
	}
	if field.Type.Kind() != reflect.Map {
		t.Fatalf("PlayerView.Nodes is a %s, not a map — absence must be structural, not a visibility flag (RFC §9.1)", field.Type.Kind())
	}
	if field.Type.Key() != reflect.TypeFor[NodeID]() {
		t.Fatalf("PlayerView.Nodes is keyed by %s, not NodeID", field.Type.Key())
	}
}

// TestOpponentViewCarriesNoPositionOrBalance walks OpponentView's fields by
// reflection so the fog boundary stays enforced as fields are added, unlike
// asserting each current field's type individually (which the compiler
// already guarantees). RFC §9.1: "Others never carries a position unless a
// writer below authorises it, and never an exact balance."
//
// It also covers issue #66's own fog acceptance criterion — "Items in hand
// never appear in any other seat's PlayerView" — structurally: no field may
// be an []ItemID, the exact type SelfState.Items uses for the seat's own
// hand (GDD §12: "Items in hand are hidden"). This is the honest scope
// available before #75's Project exists: a structural guarantee that
// OpponentView has nowhere to write a hand to, provable by reflection over
// the type rather than by calling a projection function that isn't built
// yet. The real RFC §16.3-style negative serialisation test against actual
// projected views is #75's to add, over this same guarantee.
func TestOpponentViewCarriesNoPositionOrBalance(t *testing.T) {
	typ := reflect.TypeFor[OpponentView]()
	nodeIDType := reflect.TypeFor[NodeID]()
	itemSliceType := reflect.TypeFor[[]ItemID]()

	for field := range typ.Fields() {
		if field.Type == nodeIDType {
			t.Errorf("OpponentView.%s is a NodeID — opponent position must only ever arrive via Trail or Anchors (RFC §9.1)", field.Name)
		}
		if field.Name == "Balance" {
			t.Errorf("OpponentView.%s: opponents must see only a credit band, never an exact balance (GDD §5.1)", field.Name)
		}
		if field.Type == itemSliceType {
			t.Errorf("OpponentView.%s is an []ItemID — item hands are hidden from every other seat (GDD §12); this must not exist on OpponentView", field.Name)
		}
	}
}

// TestZeroValuePlayerViewSerialisesEmpty is issue #55's acceptance
// criterion: "Serialising a zero-valued PlayerView to JSON produces no node
// IDs and no seat positions."
func TestZeroValuePlayerViewSerialisesEmpty(t *testing.T) {
	b, err := json.Marshal(PlayerView{})
	if err != nil {
		t.Fatalf("json.Marshal(PlayerView{}): %v", err)
	}

	var got struct {
		Nodes     map[NodeID]NodeView
		Others    []OpponentView
		NodeStats map[NodeID]NodeStats
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.Nodes) != 0 {
		t.Errorf("zero-valued PlayerView serialised %d node(s), want 0", len(got.Nodes))
	}
	if len(got.Others) != 0 {
		t.Errorf("zero-valued PlayerView serialised %d opponent(s), want 0", len(got.Others))
	}
	if len(got.NodeStats) != 0 {
		t.Errorf("zero-valued PlayerView serialised %d node stat(s), want 0", len(got.NodeStats))
	}
}

// TestNodeViewEdgesNilForZeroValue guards the Rumoured contract at the type
// level: a NodeView built without explicitly setting Edges — exactly what a
// Rumoured node's projection looks like — must not carry a route the client
// could plot into (GDD §7.1; RFC §9.1).
func TestNodeViewEdgesNilForZeroValue(t *testing.T) {
	var rumoured NodeView
	if rumoured.Edges != nil {
		t.Fatalf("zero-valued NodeView.Edges = %v, want nil", rumoured.Edges)
	}
}
