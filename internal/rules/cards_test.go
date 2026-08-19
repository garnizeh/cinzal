package rules

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestAllEventsCatalog pins GDD §14.2's catalog shape: 24 cards, 6 per
// category, no duplicate ID or name — the precondition every deck-quota test
// below assumes rather than re-derives.
func TestAllEventsCatalog(t *testing.T) {
	if len(allEvents) != 24 {
		t.Fatalf("len(allEvents) = %d, want 24", len(allEvents))
	}

	perCategory := map[int]int{}
	seenIDs := map[EventCardID]bool{}
	seenNames := map[string]bool{}
	for _, c := range allEvents {
		if seenIDs[c.ID] {
			t.Errorf("EventCardID %d declared more than once in allEvents", c.ID)
		}
		seenIDs[c.ID] = true

		if seenNames[c.Name] {
			t.Errorf("event card name %q declared more than once", c.Name)
		}
		seenNames[c.Name] = true

		if len(c.Tags) == 0 {
			t.Errorf("event card %q has no tags", c.Name)
		}

		perCategory[int(c.Category)]++
	}

	for cat, count := range perCategory {
		if count != 6 {
			t.Errorf("category %d has %d cards, want 6", cat, count)
		}
	}
	if len(perCategory) != 4 {
		t.Errorf("allEvents spans %d categories, want 4", len(perCategory))
	}
}

// TestAllIncidentsCatalog pins GDD §14.3's catalog shape: 16 cards, 11
// hazards and 5 boons, no duplicate ID or name.
func TestAllIncidentsCatalog(t *testing.T) {
	if len(allIncidents) != 16 {
		t.Fatalf("len(allIncidents) = %d, want 16", len(allIncidents))
	}

	var hazards, boons int
	seenIDs := map[IncidentCardID]bool{}
	seenNames := map[string]bool{}
	for _, c := range allIncidents {
		if seenIDs[c.ID] {
			t.Errorf("IncidentCardID %d declared more than once in allIncidents", c.ID)
		}
		seenIDs[c.ID] = true

		if seenNames[c.Name] {
			t.Errorf("incident card name %q declared more than once", c.Name)
		}
		seenNames[c.Name] = true

		if c.Hazard {
			hazards++
		} else {
			boons++
		}
	}

	if hazards != 11 {
		t.Errorf("hazards = %d, want 11", hazards)
	}
	if boons != 5 {
		t.Errorf("boons = %d, want 5", boons)
	}
}

// TestBuildEventDeckExactCounts is #61's first acceptance criterion:
// "Exactly 12 event cards drawn, exactly 3 per category, at every seed."
func TestBuildEventDeckExactCounts(t *testing.T) {
	for seedByte := range 50 {
		rng := NewRNG(testSeed(byte(seedByte)), 0)
		deck := buildEventDeck(rng)

		if len(deck) != 12 {
			t.Fatalf("seed %d: len(deck) = %d, want 12", seedByte, len(deck))
		}

		seen := map[EventCardID]bool{}
		perCategory := map[game.EventCategory]int{}
		for _, id := range deck {
			if seen[id] {
				t.Fatalf("seed %d: card %d drawn twice", seedByte, id)
			}
			seen[id] = true
			perCategory[eventCategoryOf(t, id)]++
		}
		for cat, count := range perCategory {
			if count != 3 {
				t.Errorf("seed %d: category %v has %d cards, want 3", seedByte, cat, count)
			}
		}
		if len(perCategory) != 4 {
			t.Errorf("seed %d: deck spans %d categories, want 4", seedByte, len(perCategory))
		}
	}
}

// TestBuildIncidentDeckExactCounts is #61's second acceptance criterion:
// "Exactly 13 incident cards drawn, exactly 9 hazards and 4 boons, at every
// seed."
func TestBuildIncidentDeckExactCounts(t *testing.T) {
	for seedByte := range 50 {
		rng := NewRNG(testSeed(byte(seedByte)), 0)
		deck := buildIncidentDeck(rng)

		if len(deck) != 13 {
			t.Fatalf("seed %d: len(deck) = %d, want 13", seedByte, len(deck))
		}

		seen := map[IncidentCardID]bool{}
		var hazards, boons int
		for _, id := range deck {
			if seen[id] {
				t.Fatalf("seed %d: card %d drawn twice", seedByte, id)
			}
			seen[id] = true
			if incidentHazardOf(t, id) {
				hazards++
			} else {
				boons++
			}
		}
		if hazards != 9 {
			t.Errorf("seed %d: hazards = %d, want 9", seedByte, hazards)
		}
		if boons != 4 {
			t.Errorf("seed %d: boons = %d, want 4", seedByte, boons)
		}
	}
}

// TestDeckIndexConsumptionMatchesTable is #61's acceptance criterion "Index
// consumption matches #41's row for deck.event and deck.incident exactly" —
// read against D03's granular split of that row into select/order (see
// rng_purpose.go's ConsumptionTable).
func TestDeckIndexConsumptionMatchesTable(t *testing.T) {
	rng := NewRNG(testSeed(200), 0)
	buildEventDeck(rng)
	if got := rng.Consumed(PurposeDeckEventSelect); got != 12 {
		t.Errorf("Consumed(deck.event.select) = %d, want 12 (3 per category x 4 categories)", got)
	}
	if got := rng.Consumed(PurposeDeckEventOrder); got != 11 {
		t.Errorf("Consumed(deck.event.order) = %d, want 11 (n-1 for n=12)", got)
	}

	rng2 := NewRNG(testSeed(201), 0)
	buildIncidentDeck(rng2)
	if got := rng2.Consumed(PurposeDeckIncidentSelect); got != 13 {
		t.Errorf("Consumed(deck.incident.select) = %d, want 13 (9 hazards + 4 boons)", got)
	}
	if got := rng2.Consumed(PurposeDeckIncidentOrder); got != 12 {
		t.Errorf("Consumed(deck.incident.order) = %d, want 12 (n-1 for n=13)", got)
	}
}

// TestDeckPeekConsumesNoIndices is #61's acceptance criterion "Peeking
// consumes zero indices. A test peeks 15 times and asserts seq is
// unchanged." Both decks are plain slices once built — RFC §6.4's "peek"
// is an ordinary index read, with no RNG in the call path at all.
func TestDeckPeekConsumesNoIndices(t *testing.T) {
	rng := NewRNG(testSeed(202), 0)
	eventDeck := buildEventDeck(rng)
	incidentDeck := buildIncidentDeck(rng)

	seqAfterBuild := rng.Seq()
	for range 15 {
		_ = eventDeck[0]
		_ = incidentDeck[0]
	}
	if rng.Seq() != seqAfterBuild {
		t.Fatalf("Seq() changed from %d to %d after 15 non-consuming peeks", seqAfterBuild, rng.Seq())
	}
}

// TestBuildDecksDeterministic is #61's acceptance criterion "The same seed
// produces the same deck order across runs."
func TestBuildDecksDeterministic(t *testing.T) {
	seed := testSeed(203)

	run := func() ([]EventCardID, []IncidentCardID) {
		rng := NewRNG(seed, 0)
		return buildEventDeck(rng), buildIncidentDeck(rng)
	}

	events1, incidents1 := run()
	events2, incidents2 := run()

	if len(events1) != len(events2) {
		t.Fatalf("event deck length mismatch: %d vs %d", len(events1), len(events2))
	}
	for i := range events1 {
		if events1[i] != events2[i] {
			t.Fatalf("event deck diverged at %d: %v vs %v", i, events1, events2)
		}
	}
	if len(incidents1) != len(incidents2) {
		t.Fatalf("incident deck length mismatch: %d vs %d", len(incidents1), len(incidents2))
	}
	for i := range incidents1 {
		if incidents1[i] != incidents2[i] {
			t.Fatalf("incident deck diverged at %d: %v vs %v", i, incidents1, incidents2)
		}
	}
}

func eventCategoryOf(t *testing.T, id EventCardID) game.EventCategory {
	t.Helper()
	for _, c := range allEvents {
		if c.ID == id {
			return c.Category
		}
	}
	t.Fatalf("event card ID %d not found in allEvents", id)
	return 0
}

func incidentHazardOf(t *testing.T, id IncidentCardID) bool {
	t.Helper()
	for _, c := range allIncidents {
		if c.ID == id {
			return c.Hazard
		}
	}
	t.Fatalf("incident card ID %d not found in allIncidents", id)
	return false
}

// TestEventCardTags pins EventCardTags against allEvents directly — the
// accessor internal/telemetry uses for GDD §22 rows 13/14's seed-derivable
// card identity (D33), so a divergence from the catalog here would be a
// silent wrong answer for that package, not just this one.
func TestEventCardTags(t *testing.T) {
	tests := []struct {
		id   EventCardID
		want []EventTag
	}{
		{EventDragnet, []EventTag{TagConvergence}},
		{EventFestival, []EventTag{TagConvergence}},
		{EventShiftChange, []EventTag{TagOpportunity}},
		{EventFencesWindfall, []EventTag{TagConvergence, TagOpportunity}},
	}
	for _, tc := range tests {
		got := EventCardTags(tc.id)
		if len(got) != len(tc.want) {
			t.Fatalf("EventCardTags(%v) = %v, want %v", tc.id, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("EventCardTags(%v) = %v, want %v", tc.id, got, tc.want)
			}
		}
	}
}

// TestEventCardTagsUnknownID reports EventCardTags' documented behavior for
// an ID naming no card in the catalog: nil, not a panic.
func TestEventCardTagsUnknownID(t *testing.T) {
	if got := EventCardTags(EventCardID(255)); got != nil {
		t.Errorf("EventCardTags(255) = %v, want nil", got)
	}
}

// TestEventCardTagsReturnsACopy mutates a returned slice and requires a
// second call to come back unchanged — EventCardTags must never hand out
// allEvents' own backing array, or one caller's mutation would corrupt the
// catalog for every match this process resolves afterward, not just its
// own read. before is snapshotted with its own slices.Clone, independent
// of got: if EventCardTags regressed to returning allEvents' own slice
// directly, before and got would alias the same backing array, and
// mutating got would silently mutate before's view of it too — the
// comparison below would then pass on a real regression instead of
// catching it.
func TestEventCardTagsReturnsACopy(t *testing.T) {
	before := slices.Clone(EventCardTags(EventFencesWindfall))

	got := EventCardTags(EventFencesWindfall)
	got[0] = TagDamage

	after := EventCardTags(EventFencesWindfall)
	if len(after) != len(before) {
		t.Fatalf("EventCardTags(EventFencesWindfall) after mutation = %v, want unchanged from %v", after, before)
	}
	for i := range after {
		if after[i] != before[i] {
			t.Errorf("EventCardTags(EventFencesWindfall) after mutation = %v, want unchanged from %v", after, before)
		}
	}
}
