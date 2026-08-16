package rules

import (
	"fmt"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is RFC §16.1's "Anchor parity" row: a table-driven test over
// both GDD §7.3's trail table and RFC §9.1's sixteen-writer table, so a
// change to either — a row added, removed, or reclassified in either
// document — is visible here, either as a build failure (a renamed or
// removed game.EventKind) or a test failure (a row whose named/distribution
// semantics or citation drifts from what the doc says).
//
// RFC §9.1's own words on the mapping, which is why this is not a simple
// row-count check: "Rows 1-5, 7 and 8 map one-to-one onto seven of §7.3's
// eight event types (the eighth, Fresh tracks, is never named and so has no
// row here by design)... Rows 6, 9, 10 and 11 are sourced from other GDD
// sections... row 12 is sourced from D12... rows 13 and 14... from GDD
// §14.2 directly... rows 15 and 16 (D26) from GDD §14.3 directly." A
// literal row-count or row-for-row equality check between the two tables
// was never correct (RFC §9.1) — every §7.3 row with a name attached must
// map to exactly one §9.1 row, and every §9.1 row without a §7.3 mapping
// must cite its real source instead.

// gdd73Row is one row of GDD §7.3's eight-entry trail table.
type gdd73Row struct {
	name      string         // GDD §7.3's own row name
	kind      game.EventKind // the writer this row's trail entry actually uses
	named     bool           // the "Name attached?" column, collapsed to a bool — Cargo taken and Item purchased are conditionally named, still "yes" in GDD's own table
	mapsToRow int            // the RFC §9.1 row this maps to, or 0 for Fresh tracks (no row, by design)
}

var gdd73Rows = []gdd73Row{
	{"Cargo taken", game.EventCargoTaken, true, 1},
	{"Fresh tracks", game.EventFreshTracks, false, 0},
	{"Confrontation", game.EventConfrontation, true, 7},
	{"Post staked", game.EventPostStaked, true, 3},
	{"Loitering, 2+ rounds", game.EventLoitering, false, 5},
	{"Lease expired", game.EventLeaseExpired, true, 4},
	{"Delivery", game.EventDelivered, true, 2},
	{"Item purchased", game.EventItemPurchased, true, 8},
}

// rfc91Row is one row of RFC §9.1's sixteen-writer table.
type rfc91Row struct {
	row      int
	kind     game.EventKind // 0 for row 12 (Decoy), which shares row 1's EventCargoTaken kind rather than owning one (D12)
	global   bool           // Distribution: Global (true) or sight-gated trail (false)
	named    bool           // "Names the player?" column
	citation string         // source document/section for a row with no §7.3 counterpart; empty for the seven that have one
}

var rfc91Rows = []rfc91Row{
	{1, game.EventCargoTaken, false, true, ""},
	{2, game.EventDelivered, true, true, ""},
	{3, game.EventPostStaked, true, true, ""},
	{4, game.EventLeaseExpired, true, true, ""},
	{5, game.EventLoitering, true, false, ""},
	{6, game.EventLooseCrateHeld, true, false, "GDD §8.4"},
	{7, game.EventConfrontation, false, true, ""},
	{8, game.EventItemPurchased, false, true, ""},
	{9, game.EventTierFeared, true, true, "GDD §11"},
	{10, game.EventTierLegend, true, true, "GDD §11"},
	{11, game.EventInformants, true, true, "GDD §14.2"},
	{12, 0, false, true, "D12, grounded in GDD §§9.4 and 12"},
	{13, game.EventDeadRunnerCrate, true, false, "GDD §14.2"},
	{14, game.EventFenceWindfallAnnounced, true, false, "GDD §14.2"},
	{15, game.EventInformantRing, true, true, "GDD §14.3 (D26)"},
	{16, game.EventSpilledLoadCrate, true, false, "GDD §14.3 (D26)"},
}

func rfc91RowByNumber(t *testing.T, n int) rfc91Row {
	t.Helper()
	for _, r := range rfc91Rows {
		if r.row == n {
			return r
		}
	}
	t.Fatalf("rfc91Rows has no row %d", n)
	return rfc91Row{}
}

// TestAnchorParityTableShapeIsSixteenRowsInOrder guards the two tables
// above against a fat-fingered edit to this file itself: sixteen §9.1 rows,
// numbered 1-16 with no gap or duplicate, split exactly 12 global / 4
// sight-gated the way RFC §9.1's own prose divides them ("Rows 2-6, 9-11
// and 13-16 reach every player unconditionally. Rows 1, 7, 8 and 12... all
// four reach only players who had sight").
func TestAnchorParityTableShapeIsSixteenRowsInOrder(t *testing.T) {
	if len(rfc91Rows) != 16 {
		t.Fatalf("len(rfc91Rows) = %d, want 16", len(rfc91Rows))
	}
	// Rows 1, 7, 8 and 12 are RFC §9.1's own sight-gated set — every other
	// row must be global. An aggregate 12/4 count alone would still pass if
	// one row swapped Distribution with another, so each row is checked
	// against this list individually, not just the totals.
	sightGatedRows := map[int]bool{1: true, 7: true, 8: true, 12: true}
	var global, sightGated int
	for i, r := range rfc91Rows {
		if r.row != i+1 {
			t.Errorf("rfc91Rows[%d].row = %d, want %d — rows must be in order with no gap", i, r.row, i+1)
		}
		if wantGlobal := !sightGatedRows[r.row]; r.global != wantGlobal {
			t.Errorf("row %d global = %v, want %v", r.row, r.global, wantGlobal)
		}
		if r.global {
			global++
		} else {
			sightGated++
		}
	}
	if global != 12 {
		t.Errorf("global rows = %d, want 12", global)
	}
	if sightGated != 4 {
		t.Errorf("sight-gated rows = %d, want 4", sightGated)
	}
}

// TestAnchorParityGDDRowsMapToRFCRows is the first half of RFC §16.1's
// two-part rule: every §7.3 row with a name attached maps to exactly one
// §9.1 row, with a matching named/unnamed semantic — Fresh tracks (never
// named) correctly maps to none.
func TestAnchorParityGDDRowsMapToRFCRows(t *testing.T) {
	for _, g := range gdd73Rows {
		t.Run(g.name, func(t *testing.T) {
			if g.name == "Fresh tracks" {
				if g.mapsToRow != 0 {
					t.Fatalf("Fresh tracks maps to row %d, want 0 — never named, out of scope by design (RFC §9.1)", g.mapsToRow)
				}
				return
			}
			if g.mapsToRow == 0 {
				t.Fatalf("%s has no RFC §9.1 row, want a mapping — every named §7.3 row must map to one", g.name)
			}
			r := rfc91RowByNumber(t, g.mapsToRow)

			// Loitering is RFC §9.1's own documented exception: GDD §7.3
			// names its 2nd-consecutive-round sight-gated trail entry,
			// while row 5 (this row) is its 3rd-round-and-up global
			// escalation — two distribution tiers of the one never-named
			// rule (event.go's EventLoitering doc, GDD §9.1), not a kind
			// mismatch to flag here.
			if g.name != "Loitering, 2+ rounds" && r.kind != g.kind {
				t.Errorf("row %d kind = %v, want %v — GDD %q's own writer", r.row, r.kind, g.kind, g.name)
			}
			if r.named != g.named {
				t.Errorf("row %d named = %v, want %v to match GDD %q's \"Name attached?\" column", r.row, r.named, g.named, g.name)
			}
		})
	}
}

// TestAnchorParityRFCRowsWithoutGDDCiteTheirSource is the second half:
// every §9.1 row without a §7.3 counterpart — rows 6, 9-16 — cites its real
// source document and section, since no row-for-row equality between an
// eight-row table and a sixteen-row table could ever hold (RFC §9.1).
func TestAnchorParityRFCRowsWithoutGDDCiteTheirSource(t *testing.T) {
	mapped := make(map[int]bool, len(gdd73Rows))
	for _, g := range gdd73Rows {
		mapped[g.mapsToRow] = true
	}

	for _, r := range rfc91Rows {
		t.Run(fmt.Sprintf("row%d", r.row), func(t *testing.T) {
			if mapped[r.row] {
				if r.citation != "" {
					t.Errorf("row %d has both a GDD §7.3 counterpart and a citation %q — only an uncited row needs one", r.row, r.citation)
				}
				return
			}
			if r.citation == "" {
				t.Errorf("row %d has no §7.3 counterpart and no citation — every such row must name its own source document and section (RFC §9.1)", r.row)
			}
		})
	}
}

// TestAnchorParityGlobalWriterRowsMatchBuildRoundAnchors ties rows 2, 3, 4,
// 11, 13-16 — the eight rows sourced from MatchState.RoundAnchors — to the
// function that actually builds them, so an edit to buildRoundAnchors's
// switch that drops or misnames a case fails here, not just in
// anchors_test.go's separate coverage of the same function.
func TestAnchorParityGlobalWriterRowsMatchBuildRoundAnchors(t *testing.T) {
	for _, n := range []int{2, 3, 4, 11, 13, 14, 15, 16} {
		r := rfc91RowByNumber(t, n)
		t.Run(fmt.Sprintf("row%d", n), func(t *testing.T) {
			ev := game.Event{Kind: r.kind, Node: 3, Round: 7, Seat: 1}
			anchors := buildRoundAnchors([]game.Event{ev}, 7)
			if len(anchors) != 1 {
				t.Fatalf("buildRoundAnchors(%v) = %+v, want exactly 1 anchor", r.kind, anchors)
			}
			if got := anchors[0]; got.Kind != r.kind || got.Node != 3 || got.Round != 7 {
				t.Errorf("row %d anchor = %+v, want Kind %v, Node 3, Round 7", n, got, r.kind)
			}
			if named := anchors[0].Actor != nil; named != r.named {
				t.Errorf("row %d anchor named = %v, want %v", n, named, r.named)
			}
		})
	}
}

// TestAnchorParitySightGatedNamingThresholdsMatchTable ties rows 1, 7 and
// 8 — the three sight-gated writers whose naming rule is a live Infamy
// threshold rather than a constant — to the functions and named constants
// that actually decide it, rather than a magic number re-typed here.
func TestAnchorParitySightGatedNamingThresholdsMatchTable(t *testing.T) {
	s := trailTestState(5, 0)

	// Row 1: named iff Infamy >= infamyNameCargoTaken.
	s.Players[0].Infamy = infamyNameCargoTaken
	if te := cargoTakenEntry(s, 0, 0); te.Actor == nil {
		t.Error("row 1 at the naming threshold: Actor = nil, want named")
	}
	s.Players[0].Infamy = infamyNameCargoTaken - 1
	if te := cargoTakenEntry(s, 0, 0); te.Actor != nil {
		t.Error("row 1 below the naming threshold: Actor != nil, want anonymous")
	}

	// Row 7: always named, both parties, no threshold.
	entries := map[game.NodeID][]game.TrailEntry{}
	addConfrontation([]game.Event{{Kind: game.EventConfrontation, Node: 0, Seat: 0, Target: 1}}, entries)
	if len(entries[0]) != 1 || entries[0][0].Actor == nil || entries[0][0].Target == nil {
		t.Errorf("row 7 entries = %+v, want both parties named unconditionally", entries[0])
	}

	// Row 8: named iff Infamy >= infamyNameItemPurchased.
	s.Players[0].Infamy = infamyNameItemPurchased
	entries = map[game.NodeID][]game.TrailEntry{}
	addItemPurchased(s, []game.Event{{Kind: game.EventItemPurchased, Node: 0, Seat: 0}}, entries, globalEventContext{}, incidentContext{}, nil)
	if len(entries[0]) != 1 || entries[0][0].Actor == nil {
		t.Errorf("row 8 at the naming threshold: entries = %+v, want named", entries[0])
	}
	s.Players[0].Infamy = infamyNameItemPurchased - 1
	entries = map[game.NodeID][]game.TrailEntry{}
	addItemPurchased(s, []game.Event{{Kind: game.EventItemPurchased, Node: 0, Seat: 0}}, entries, globalEventContext{}, incidentContext{}, nil)
	if len(entries[0]) != 1 || entries[0][0].Actor != nil {
		t.Errorf("row 8 below the naming threshold: entries = %+v, want anonymous", entries[0])
	}
}
