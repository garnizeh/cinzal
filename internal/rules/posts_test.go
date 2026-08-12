package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestPostCapReached pins GDD §10.3's per-player-count cap table and the
// fail-closed rule for an unconfigured player count.
func TestPostCapReached(t *testing.T) {
	cfg := game.DefaultConfig() // PostCapByPlayers: {2: 4, 3: 4, 4: 4, 5: 3}

	if cap, reached := PostCapReached(3, 2, cfg); reached || cap != 4 {
		t.Fatalf("PostCapReached(3, 2 players) = (%d, %v), want (4, false)", cap, reached)
	}
	if cap, reached := PostCapReached(4, 2, cfg); !reached || cap != 4 {
		t.Fatalf("PostCapReached(4, 2 players) = (%d, %v), want (4, true)", cap, reached)
	}
	if cap, reached := PostCapReached(3, 5, cfg); !reached || cap != 3 {
		t.Fatalf("PostCapReached(3, 5 players) = (%d, %v), want (3, true) — 5-player cap is 3", cap, reached)
	}

	// No PostCapByPlayers entry at all: fail closed, never read as "no cap".
	if cap, reached := PostCapReached(0, 6, cfg); !reached || cap != 0 {
		t.Fatalf("PostCapReached(0, 6 players — unconfigured) = (%d, %v), want (0, true)", cap, reached)
	}
}

// TestFirstPostInSector covers GDD §11's "+1 Infamy for your first post in a
// sector": true with no posts owned yet in the target sector, false once one
// already exists there, regardless of how many posts sit elsewhere.
func TestFirstPostInSector(t *testing.T) {
	s := MatchState{
		Graph: Graph{Nodes: []Node{
			{ID: 0, Sector: game.SectorOldDocks},
			{ID: 1, Sector: game.SectorOldDocks},
			{ID: 2, Sector: game.SectorIronLow},
		}},
		Players: []Player{
			{Seat: 0, Posts: []game.NodeID{2}}, // already holds Iron Low
		},
	}

	if !FirstPostInSector(s, 0, 0) {
		t.Error("FirstPostInSector(node 0, Old Docks) = false, want true — no post held there yet")
	}
	if !FirstPostInSector(s, 0, 1) {
		t.Error("FirstPostInSector(node 1, Old Docks) = false, want true — a different Old Docks node is still unowned")
	}
	if FirstPostInSector(s, 0, 2) {
		// Not staked directly at the already-held node, but same sector
		// membership is what the rule tests, not node identity.
		t.Error("FirstPostInSector(node 2, Iron Low) = true, want false — Iron Low already held")
	}

	sWithBoth := MatchState{
		Graph: s.Graph,
		Players: []Player{
			{Seat: 0, Posts: []game.NodeID{0, 2}},
		},
	}
	if FirstPostInSector(sWithBoth, 0, 1) {
		t.Error("FirstPostInSector(node 1, Old Docks) = true, want false — Old Docks already held via node 0")
	}
}

// TestRenewalBlockCapEnforcesTheTwelveRoundCeiling pins GDD §10.4: "Maximum
// 4 blocks held at once... renewals may top a post back up to 12 remaining
// rounds," never past it — including a lease sitting on a non-multiple-of-3
// remainder, since Upkeep decrements by 1 every round.
func TestRenewalBlockCapEnforcesTheTwelveRoundCeiling(t *testing.T) {
	cfg := game.DefaultConfig() // LeaseBlockRounds: 3

	cases := []struct {
		current int
		want    int
	}{
		{0, 4},  // a fresh, not-yet-staked post: the full ceiling
		{3, 3},  // 1 block spent, 3 remain purchasable (9 more -> 12)
		{9, 1},  // 3 blocks spent, exactly 1 more fits
		{10, 0}, // headroom of 2 rounds: not enough for another whole block
		{12, 0}, // already at the ceiling: cannot be renewed higher
	}
	for _, c := range cases {
		if got := RenewalBlockCap(c.current, cfg); got != c.want {
			t.Errorf("RenewalBlockCap(%d) = %d, want %d", c.current, got, c.want)
		}
	}
}

// TestRenewalBlockCapGuardsZeroLeaseBlockRounds guards the divide against a
// malformed Config rather than panicking.
func TestRenewalBlockCapGuardsZeroLeaseBlockRounds(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.LeaseBlockRounds = 0
	if got := RenewalBlockCap(0, cfg); got != 0 {
		t.Fatalf("RenewalBlockCap with LeaseBlockRounds=0 = %d, want 0", got)
	}
}

// TestRenewedRoundsRemainingClampsAtTwelve: a renewal declaring more blocks
// than the ceiling has room for still lands at exactly 12, never past it —
// resolution's own defensive floor under the same ceiling
// RenewalBlockCap keeps the affordance layer from ever offering in the
// first place.
func TestRenewedRoundsRemainingClampsAtTwelve(t *testing.T) {
	cfg := game.DefaultConfig()

	if got := RenewedRoundsRemaining(0, 1, cfg); got != 3 {
		t.Errorf("RenewedRoundsRemaining(0, 1 block) = %d, want 3", got)
	}
	if got := RenewedRoundsRemaining(9, 4, cfg); got != 12 {
		t.Errorf("RenewedRoundsRemaining(9, 4 blocks) = %d, want 12 (clamped, not 21)", got)
	}
	if got := RenewedRoundsRemaining(12, 1, cfg); got != 12 {
		t.Errorf("RenewedRoundsRemaining(12, 1 block) = %d, want 12 (already at the ceiling)", got)
	}
}

// TestDecrementLeaseExpiresExactlyAtZero: Upkeep step 2's ordinary
// decrement (RFC §6.7, D5) crosses to zero once, on the round it happens,
// never a round early or late.
func TestDecrementLeaseExpiresExactlyAtZero(t *testing.T) {
	next, expires := DecrementLease(3)
	if next != 2 || expires {
		t.Fatalf("DecrementLease(3) = (%d, %v), want (2, false)", next, expires)
	}

	next, expires = DecrementLease(1)
	if next != 0 || !expires {
		t.Fatalf("DecrementLease(1) = (%d, %v), want (0, true)", next, expires)
	}
}

// TestPostSightIsNodeOnly is GDD §7.2's node-only restriction, v1.1's fix
// for the v1.0 rule (post sight + everything adjacent) the same section
// measures at 76-98% permanent map coverage: sight must hold for the post's
// own node and fail for every other node, adjacent or not.
func TestPostSightIsNodeOnly(t *testing.T) {
	post := game.NodeID(5)
	adjacent := []game.NodeID{4, 6, 12}

	if !PostSight(post, post) {
		t.Error("PostSight(post, post) = false, want true — a post always sees its own node")
	}
	for _, target := range adjacent {
		if PostSight(post, target) {
			t.Errorf("PostSight(post=%d, target=%d) = true, want false — adjacency grants no sight (GDD §7.2)", post, target)
		}
	}
}

// sectorMajorityFixture builds a MatchState with nPerSector nodes per
// sector (all four sectors), leaving every node's Post nil — callers stake
// the specific nodes their case needs.
func sectorMajorityFixture(nPerSector int, players int) MatchState {
	sectors := []game.Sector{game.SectorOldDocks, game.SectorIronLow, game.SectorMistHeights, game.SectorNorthVale}

	var nodes []Node
	for _, sector := range sectors {
		for range nPerSector {
			nodes = append(nodes, Node{ID: game.NodeID(len(nodes)), Sector: sector})
		}
	}

	seats := make([]Player, players)
	for i := range seats {
		seats[i] = Player{Seat: game.SeatID(i)}
	}

	return MatchState{Graph: Graph{Nodes: nodes}, Players: seats}
}

// TestSectorMajorityWinnerByPostCount: the seat with strictly more live
// posts in a sector wins it, regardless of how many other seats hold fewer.
func TestSectorMajorityWinnerByPostCount(t *testing.T) {
	s := sectorMajorityFixture(3, 3) // 3 nodes per sector, Old Docks is nodes 0-2
	s.Graph.Nodes[0].Post = &Post{Owner: 0}
	s.Graph.Nodes[1].Post = &Post{Owner: 0}
	s.Graph.Nodes[2].Post = &Post{Owner: 1}

	got := SectorMajority(s)
	if seat, ok := got[game.SectorOldDocks]; !ok || seat != 0 {
		t.Fatalf("SectorMajority()[OldDocks] = (%v, %v), want (0, true)", seat, ok)
	}
	if _, ok := got[game.SectorIronLow]; ok {
		t.Fatalf("SectorMajority()[IronLow] present with no live posts, want absent")
	}
}

// TestSectorMajorityTieScoresNobody is GDD §16 verbatim: "ties score
// nobody" — a sector split evenly between two seats has no entry at all.
func TestSectorMajorityTieScoresNobody(t *testing.T) {
	s := sectorMajorityFixture(2, 2)
	s.Graph.Nodes[0].Post = &Post{Owner: 0}
	s.Graph.Nodes[1].Post = &Post{Owner: 1}

	got := SectorMajority(s)
	if seat, ok := got[game.SectorOldDocks]; ok {
		t.Fatalf("SectorMajority()[OldDocks] = (%v, true), want absent on a tie", seat)
	}
}

// TestSectorMajorityOnlyCountsLivePosts: a node with no Post at all
// contributes to nobody's count, even in a sector that otherwise has posts.
func TestSectorMajorityOnlyCountsLivePosts(t *testing.T) {
	s := sectorMajorityFixture(3, 2)
	s.Graph.Nodes[0].Post = &Post{Owner: 0}
	// Nodes 1 and 2 stay unowned.

	got := SectorMajority(s)
	if seat, ok := got[game.SectorOldDocks]; !ok || seat != 0 {
		t.Fatalf("SectorMajority()[OldDocks] = (%v, %v), want (0, true) — sole post wins outright", seat, ok)
	}
}
