package game

import "testing"

// Stringer is satisfied by every enum in this package. Kept as an explicit
// compile-time assertion because a String() method with the wrong receiver
// or a typo'd signature fails silently — fmt falls back to the numeric
// default instead of a build error.
type stringer interface{ String() string }

var (
	_ stringer = NodeType(0)
	_ stringer = Sector(0)
	_ stringer = FogState(0)
	_ stringer = Stance(0)
	_ stringer = ActionKind(0)
	_ stringer = InfamyTier(0)
	_ stringer = ItemID(0)
	_ stringer = EventKind(0)
	_ stringer = CreditBand(0)
	_ stringer = EventCategory(0)
	_ stringer = HaltCause(0)
)

// TestZeroValueIsInvalid holds every enum in this package to the convention
// documented in enums.go: the zero value is reserved and must not print as
// one of the named constants, so a field that was never explicitly set is
// distinguishable from one that was.
func TestZeroValueIsInvalid(t *testing.T) {
	cases := []struct {
		name string
		zero stringer
		want string
	}{
		{"NodeType", NodeType(0), "NodeType(0)"},
		{"Sector", Sector(0), "Sector(0)"},
		{"FogState", FogState(0), "FogState(0)"},
		{"Stance", Stance(0), "Stance(0)"},
		{"ActionKind", ActionKind(0), "ActionKind(0)"},
		{"InfamyTier", InfamyTier(0), "InfamyTier(0)"},
		{"ItemID", ItemID(0), "ItemID(0)"},
		{"EventKind", EventKind(0), "EventKind(0)"},
		{"CreditBand", CreditBand(0), "CreditBand(0)"},
		{"EventCategory", EventCategory(0), "EventCategory(0)"},
		{"HaltCause", HaltCause(0), "HaltCause(0)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.zero.String(); got != c.want {
				t.Fatalf("zero value of %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestFogStateOrdersForwardOnly(t *testing.T) {
	if FogHidden >= FogRumoured || FogRumoured >= FogKnown || FogKnown >= FogInSight {
		t.Fatalf("FogState constants are not in ascending Hidden < Rumoured < Known < InSight order")
	}
}

func TestInfamyTierOrdersAscending(t *testing.T) {
	if TierNobody >= TierKnown || TierKnown >= TierFeared || TierFeared >= TierLegend {
		t.Fatalf("InfamyTier constants are not in ascending order")
	}
}

func TestItemIDNamesMatchGDDSection12(t *testing.T) {
	want := map[ItemID]string{
		ItemShiv:              "Shiv",
		ItemMuscle:            "Muscle",
		ItemPoliceBand:        "Police Band",
		ItemCirculationPermit: "Circulation Permit",
		ItemTornMap:           "Torn Map",
		ItemDecoy:             "Decoy",
		ItemBoltHole:          "Bolt Hole",
		ItemGuardContact:      "Guard Contact",
	}

	if len(want) != 8 {
		t.Fatalf("test table itself is wrong: GDD §12 lists 8 items, table has %d", len(want))
	}

	for id, name := range want {
		if got := id.String(); got != name {
			t.Errorf("%v.String() = %q, want %q", id, got, name)
		}
	}
}

func TestCreditBandNamesMatchGDDSection5Point1(t *testing.T) {
	want := map[CreditBand]string{
		BandBroke:     "Broke",
		BandGettingBy: "Getting by",
		BandFlush:     "Flush",
		BandLoaded:    "Loaded",
	}

	if len(want) != 4 {
		t.Fatalf("test table itself is wrong: GDD §5.1 lists 4 bands, table has %d", len(want))
	}

	for band, name := range want {
		if got := band.String(); got != name {
			t.Errorf("%v.String() = %q, want %q", band, got, name)
		}
	}
}

// TestHaltCauseNamesAreStable pins the String() output of every HaltCause
// constant — D39's split of EventRouteHalted's three call sites, the value
// RFC §11.3's narrated resolution list (M5) and §15.1's debug panel read
// (D43, docs/decisions/D43-row-1-unmeasurable-post-d39.md).
func TestHaltCauseNamesAreStable(t *testing.T) {
	want := map[HaltCause]string{
		HaltCauseTie:             "Tie",
		HaltCauseDecisiveLoser:   "DecisiveLoser",
		HaltCauseCorrectedWinner: "CorrectedWinner",
	}

	if len(want) != 3 {
		t.Fatalf("test table itself is wrong: EventRouteHalted has 3 call sites, table has %d", len(want))
	}

	for cause, name := range want {
		if got := cause.String(); got != name {
			t.Errorf("%v.String() = %q, want %q", cause, got, name)
		}
	}
}

func TestEventCategoryNamesMatchGDDSection14Point2(t *testing.T) {
	want := map[EventCategory]string{
		CategoryPolice:     "Police",
		CategoryEconomy:    "Economy",
		CategoryUnderworld: "Underworld",
		CategoryCity:       "City",
	}

	if len(want) != 4 {
		t.Fatalf("test table itself is wrong: GDD §14.2 lists 4 categories, table has %d", len(want))
	}

	for category, name := range want {
		if got := category.String(); got != name {
			t.Errorf("%v.String() = %q, want %q", category, got, name)
		}
	}
}
