package rules

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestAdversarialGDDSection15_0 is issue #79's "one adversarial test per
// row" acceptance criterion, GDD §15.0's illegal-payload table, client
// bypassed: every case builds a raw game.Order by hand and calls Legal
// directly, the same shape legal_test.go's individual tests already use —
// this table exists to make the row-to-test mapping visible in one place,
// not to replace those more detailed tests.
//
// Rows 2 and 4 share one entry: legal.go's own doc comment on
// ReasonHiddenNodeNotLast states the three rows "reduce to the same check"
// (a step out of a Hidden node without Pushing On is indistinguishable,
// structurally, from more-than-one-Hidden-node-or-not-last) — one case
// below exercises both at once, exactly as legalRoute does.
func TestAdversarialGDDSection15_0(t *testing.T) {
	cfg := legalTestConfig()

	cases := []struct {
		name string
		o    game.Order
		want Reason
	}{
		{
			name: "row 1: a step between non-adjacent nodes",
			// Node 0's edges are [1, 4] — 2 is not adjacent.
			o:    game.Order{Route: []game.NodeID{2}, Action: game.ActionOrder{Kind: game.ActionNothing}},
			want: ReasonNonAdjacentStep,
		},
		{
			name: "rows 2 and 4: a step out of a Hidden node without Pushing On, and more than one Hidden node / a Hidden node that isn't last",
			// 0 -> 1 -> 2 -> 3 (Hidden) -> 1: a further step out of the
			// Hidden node with no Pushing On declared.
			o:    game.Order{Route: []game.NodeID{1, 2, 3, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}},
			want: ReasonHiddenNodeNotLast,
		},
		{
			name: "row 3: route longer than your step allowance",
			// Legend tier (Infamy 9): 2 steps. 0<->1 is a mutual edge, so
			// only the length trips this, not adjacency.
			o:    game.Order{Route: []game.NodeID{1, 0, 1, 0, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}},
			want: ReasonStepAllowanceExceeded,
		},
		{
			name: "row 5: an action illegal at the ending node's type",
			// Deliver at node 1, an Alley, not a Border.
			o:    game.Order{Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionDeliver}},
			want: ReasonIllegalActionAtNode,
		},
		{
			name: "row 6: Pushing On combined with an action",
			o: game.Order{
				Route:     []game.NodeID{1, 2, 3},
				PushingOn: game.PushingOn{Steps: 1},
				Action:    game.ActionOrder{Kind: game.ActionDeliver},
			},
			want: ReasonPushingOnWithAction,
		},
		{
			name: "row 7: add-ons or a lease exceeding your balance at submission",
			o:    game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}, AddOns: game.AddOns{BuyLedger: true}},
			want: ReasonInsufficientBalance,
		},
		{
			name: "row 8: staking beyond your post cap",
			o:    game.Order{Action: game.ActionOrder{Kind: game.ActionStakePost}},
			want: ReasonPostCapExceeded,
		},
		{
			name: "row 9: a route plotted through nodes not Known to that player",
			// Node 4 is only Rumoured — GDD §7.1: "no edges, so you cannot
			// plot a route through or into it."
			o:    game.Order{Route: []game.NodeID{4}, Action: game.ActionOrder{Kind: game.ActionNothing}},
			want: ReasonNodeNotKnown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := legalTestView()

			// Row 7 and row 8 each need a view mutated beyond the shared
			// baseline; every other row is legal against legalTestView()
			// unmodified except for the field(s) the order itself declares.
			switch c.want {
			case ReasonInsufficientBalance:
				v.You.Balance = 0
			case ReasonPostCapExceeded:
				postCap := cfg.PostCapByPlayers[len(v.Others)+1]
				for i := range postCap {
					v.You.Posts = append(v.You.Posts, game.Post{Node: game.NodeID(100 + i), RoundsRemaining: 3})
				}
			}
			if c.want == ReasonStepAllowanceExceeded {
				v.You.Infamy = 9
			}

			wantReason(t, Legal(v, c.o, cfg), c.want)
		})
	}
}

// TestAdversarialKnownNodeRowLogsViaReason locks GDD §15.0's last row: "a
// route plotted through nodes not Known to that player... logs it, because
// a well-formed client cannot produce this." internal/rules can never write
// an actual log line — scripts/check-rules-purity.sh hard-fails on any
// log/log.* import in this package, so that responsibility belongs to
// whatever calls Legal from internal/match. What this package owes is the
// distinguishable machine-readable signal a caller would key a log line
// off of: Reason must be exactly ReasonNodeNotKnown, not merely "some
// rejection occurred" — the row's whole point is that this specific case is
// suspicious enough to log, unlike an ordinary non-adjacent step.
func TestAdversarialKnownNodeRowLogsViaReason(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{4}, Action: game.ActionOrder{Kind: game.ActionNothing}} // node 4 is Rumoured, not Known
	err := Legal(v, o, legalTestConfig())

	wantReason(t, err, ReasonNodeNotKnown)

	// Confirm this is genuinely a distinct value from an ordinary reject —
	// the caller cannot key a "log this" decision off a Reason that some
	// harmless illegal payload could also produce.
	if ReasonNodeNotKnown == ReasonNonAdjacentStep {
		t.Fatal("ReasonNodeNotKnown must be a distinct Reason value from ReasonNonAdjacentStep")
	}
}

// --- malformed shapes GDD §15.0 does not enumerate (issue #79) ---

// TestAdversarialRepeatedNodeInRouteIsLegal: a route that revisits a node
// (A -> B -> A) is not one of GDD §15.0's illegal payloads — it is
// ordinary, legal movement as long as every step's edge exists, the same
// way walking to a dead end and back is legal in life. Nothing in legalRoute
// tracks visited nodes, and nothing should: rejecting it would invent a
// rule GDD never states.
func TestAdversarialRepeatedNodeInRouteIsLegal(t *testing.T) {
	v := legalTestView()
	// 0 -> 1 -> 0 -> 1: nodes 0 and 1 have a mutual edge (legalTestView),
	// so every step is adjacency-legal despite the route revisiting node 0.
	o := game.Order{Route: []game.NodeID{1, 0, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (a repeated node is legal movement, not a GDD §15.0 row)", err)
	}
}

// TestAdversarialNegativeStakeRejected is the "negative stake" malformed
// shape: game.Order.Stake is documented "Cr$ 0-6", and nothing enforced the
// floor before this issue. Left unchecked, resolveLoser's
// `staked := min(o.Stance.Stake, p.Balance)` goes negative too, and
// `p.Balance -= staked` then *increases* the loser's balance on a loss —
// exploitable, not merely untested.
func TestAdversarialNegativeStakeRejected(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:  []game.NodeID{1},
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: -1},
	}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonNegativeStake)
}

// TestAdversarialZeroStakeAccepted is the negative baseline for the check
// above — 0 is the documented floor, not the first illegal value.
func TestAdversarialZeroStakeAccepted(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:  []game.NodeID{1},
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 0},
	}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (stake 0 is the documented floor)", err)
	}
}

// TestAdversarialBlindStepCountOfThreeRejected is the "blind step count of
// 3" malformed shape: GDD §9.1 caps Pushing On's blind continuation at 0-2,
// and nothing enforced that before this issue — a declared count of 3 sailed
// through Legal and advance (movement.go) would have walked the extra blind
// step anyway.
func TestAdversarialBlindStepCountOfThreeRejected(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:     []game.NodeID{1, 2, 3}, // ends Hidden, the only legal anchor for Pushing On
		PushingOn: game.PushingOn{Steps: 3},
		Action:    game.ActionOrder{Kind: game.ActionNothing},
	}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonBlindStepCountExceeded)
}

// TestAdversarialBlindStepCountOfTwoAccepted is the positive baseline: 2 is
// the documented ceiling, not the first illegal value.
func TestAdversarialBlindStepCountOfTwoAccepted(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:     []game.NodeID{1, 2, 3},
		PushingOn: game.PushingOn{Steps: 2},
		Action:    game.ActionOrder{Kind: game.ActionNothing},
	}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (2 blind steps is the documented ceiling)", err)
	}
}

// TestAdversarialItemTargetOnUnseenNodeRejected is the "item target naming
// a node the player cannot see" malformed shape: a NodeID absent from the
// view entirely (never disclosed at any fog tier — distinct from node 4's
// Rumoured or node 3's genuinely-Hidden-but-known-to-exist neighbour in
// legalTestView) must be rejected exactly like a Hidden target, not treated
// as some third, unchecked case. legalItemTargets's own `v.Nodes[d.Target]`
// lookup already handles this generically for every item — this test
// confirms it rather than adding new code.
func TestAdversarialItemTargetOnUnseenNodeRejected(t *testing.T) {
	v := legalTestView()
	v.You.Items = []game.ItemID{game.ItemDecoy}
	// 999 is not a neighbour of any node in legalTestView — absent from
	// v.Nodes at every fog tier, not merely Rumoured or Hidden-but-adjacent.
	o := game.Order{Items: []game.ItemDiscard{{Item: game.ItemDecoy, Target: 999}}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonInvalidItemTarget)
}

// TestAdversarialOrderForSeatNotInMatchIsInert is the "an Order for a seat
// not in the match" malformed shape. validate (validate.go) iterates the
// real seats list and only ever reads orders[seat] — a stray extra key
// keyed by a SeatID with no corresponding Player is never read at all, by
// construction, not by a check that has to reject it. This confirms that:
// Resolve with a bogus extra order produces byte-identical output (state
// and events) to the same call without it.
func TestAdversarialOrderForSeatNotInMatchIsInert(t *testing.T) {
	s := resolveTestState()
	cfg := legalTestConfig()
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1}},
	}
	bogus := map[game.SeatID]game.Order{
		0:  {Route: []game.NodeID{1}},
		99: {Route: []game.NodeID{1, 2, 3}}, // seat 99 has no Player in resolveTestState
	}

	seed := testSeed(0x79)
	wantNext, wantEvents, err := Resolve(s, orders, cfg, NewRNG(seed, int(s.Round)+1))
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	gotNext, gotEvents, err := Resolve(s, bogus, cfg, NewRNG(seed, int(s.Round)+1))
	if err != nil {
		t.Fatalf("Resolve() with a bogus extra seat error = %v, want nil", err)
	}

	if !reflect.DeepEqual(wantNext, gotNext) {
		t.Fatalf("Resolve() with an order for a seat not in the match produced a different state:\nwithout: %+v\nwith:    %+v", wantNext, gotNext)
	}
	if !reflect.DeepEqual(wantEvents, gotEvents) {
		t.Fatalf("Resolve() with an order for a seat not in the match produced different events:\nwithout: %+v\nwith:    %+v", wantEvents, gotEvents)
	}
}

// A note on the two GDD §15.0-adjacent malformed shapes that are not tested
// anywhere in this package, deliberately:
//
// A stale Order.Round (RFC §11.1a) is rejected "in the submit transaction"
// (RFC §8.1) — the HTTP/DB layer inside internal/match, which is still a
// doc.go stub (D01). internal/rules reads o.Round in exactly one place
// (legalBalance's Ledger-final-round check) and never compares it against
// s.Round/next.Round for staleness at all — by RFC §11.1a's own design, that
// comparison happens before an order ever reaches this package, so there is
// no code path here a test could exercise. This gets its real test once
// internal/match exists.
//
// The Known-node row's "log it" clause is covered above
// (TestAdversarialKnownNodeRowLogsViaReason) via the Reason distinction
// rather than an actual log call — see that test's doc comment.
