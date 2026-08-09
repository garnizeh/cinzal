package game

import (
	"reflect"
	"testing"
)

// TestEventKindNamesAreStable pins the String() output of every EventKind
// constant. RFC §6.7 lists the debug trace and telemetry among Event's six
// consumers, both of which read this text — a silent rename here is exactly
// the kind of drift those consumers would not notice until a dashboard went
// blank.
func TestEventKindNamesAreStable(t *testing.T) {
	want := map[EventKind]string{
		EventCargoTaken:    "CargoTaken",
		EventFreshTracks:   "FreshTracks",
		EventConfrontation: "Confrontation",
		EventPostStaked:    "PostStaked",
		EventLoitering:     "Loitering",
		EventLeaseExpired:  "LeaseExpired",
		EventDelivered:     "Delivered",
		EventItemPurchased: "ItemPurchased",
	}

	if len(want) != 8 {
		t.Fatalf("test table itself is wrong: GDD §7.3 lists 8 trail archetypes, table has %d", len(want))
	}

	for kind, name := range want {
		if got := kind.String(); got != name {
			t.Errorf("%v.String() = %q, want %q", kind, got, name)
		}
	}
}

// TestEventCarriesNoStringField guards RFC §11.5's rendering contract: "kind,
// params", structured, never a rendered sentence. A string field is exactly
// how a locale-dependent sentence would sneak back in, so this walks Event's
// fields by reflection and fails if any of them is a string — a check that
// stays meaningful as fields are added, unlike asserting each field's type
// individually (which Go's compiler already guarantees and can never fail).
func TestEventCarriesNoStringField(t *testing.T) {
	typ := reflect.TypeFor[Event]()
	for field := range typ.Fields() {
		if field.Type.Kind() == reflect.String {
			t.Errorf("Event.%s is a string — RFC §11.5 forbids prose in Event", field.Name)
		}
	}
}
