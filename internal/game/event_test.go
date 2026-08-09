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

		// Not from GDD §7.3 — RFC §9.1's Anchor writer table, rows 6, 9,
		// 10 and 11, sourced from GDD §8.4, §11, and §14.2 respectively.
		EventLooseCrateHeld: "LooseCrateHeld",
		EventTierFeared:     "TierFeared",
		EventTierLegend:     "TierLegend",
		EventInformants:     "Informants",
	}

	if len(want) != 12 {
		t.Fatalf("test table itself is wrong: GDD §7.3's 8 trail archetypes plus RFC §9.1's 4 additional writer rows is 12, table has %d", len(want))
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
