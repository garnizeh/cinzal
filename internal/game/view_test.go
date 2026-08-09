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
func TestOpponentViewCarriesNoPositionOrBalance(t *testing.T) {
	typ := reflect.TypeFor[OpponentView]()
	nodeIDType := reflect.TypeFor[NodeID]()

	for field := range typ.Fields() {
		if field.Type == nodeIDType {
			t.Errorf("OpponentView.%s is a NodeID — opponent position must only ever arrive via Trail or Anchors (RFC §9.1)", field.Name)
		}
		if field.Name == "Balance" {
			t.Errorf("OpponentView.%s: opponents must see only a credit band, never an exact balance (GDD §5.1)", field.Name)
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
