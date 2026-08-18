package bots

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// testSeed builds a distinct match seed per byte value — this file's own
// analogue of internal/rules/rng_test.go's unexported helper of the same
// name, which this package cannot reach.
func testSeed(b byte) [32]byte {
	var s [32]byte
	for i := range s {
		s[i] = b
	}
	return s
}

// drawSeed derives a distinct-but-deterministic seed from base, for taking
// several independent Sample draws against the same view — never used to
// pick the order that actually advances a driven match, only to widen the
// corpus tests' coverage of what Sample can produce from one view.
func drawSeed(base [32]byte, draw int) [32]byte {
	s := base
	s[31] ^= byte(draw)
	return s
}

// TestRoutesIncludesEmptyRouteAndRespectsBudget checks Routes' two
// structural guarantees over fixtureView's 4-node line graph (bot_test.go):
// the empty route is always present (GDD §9.1), no route exceeds this
// round's step allowance, and — since the fixture graph does have edges —
// Routes finds at least one route past the empty one.
func TestRoutesIncludesEmptyRouteAndRespectsBudget(t *testing.T) {
	cfg := game.DefaultConfig()
	v := fixtureView(4, 2)
	budget := rules.Steps(v, cfg)

	routes := Routes(v, cfg)

	foundEmpty, maxLen := false, 0
	for _, rt := range routes {
		if len(rt.Nodes) == 0 {
			foundEmpty = true
		}
		if len(rt.Nodes) > maxLen {
			maxLen = len(rt.Nodes)
		}
		if len(rt.Nodes) > budget {
			t.Errorf("route %v has length %d, exceeding this round's step budget %d", rt.Nodes, len(rt.Nodes), budget)
		}
	}
	if !foundEmpty {
		t.Error("Routes did not include the empty (stay-put) route")
	}
	if maxLen == 0 {
		t.Error("Routes never extended past the empty route, on a graph with edges from the current position")
	}
}

// TestRoutesExcludesRumouredSteps asserts Routes never plots through a
// Rumoured node — GDD §7.1's "a Rumoured node has no edges, so you cannot
// plot a route through or into it" — by placing one directly adjacent to
// the seat's own position and confirming no returned route ever visits it.
func TestRoutesExcludesRumouredSteps(t *testing.T) {
	cfg := game.DefaultConfig()
	v := game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 3}},
			2: {Fog: game.FogRumoured, Type: game.NodeAlley}, // no Edges — matches GDD §7.1
			3: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{1}},
		},
	}

	for _, rt := range Routes(v, cfg) {
		for _, n := range rt.Nodes {
			if n == 2 {
				t.Fatalf("route %v steps into Rumoured node 2", rt.Nodes)
			}
		}
	}
}

// TestActionsAlwaysIncludesNothing checks the one candidate Actions is
// guaranteed to offer regardless of view or draft: rules.Legal places no
// restriction on ActionNothing anywhere.
func TestActionsAlwaysIncludesNothing(t *testing.T) {
	cfg := game.DefaultConfig()
	v := fixtureView(4, 2)
	draft := game.Order{Round: v.Round, Stance: game.StanceOrder{Stance: game.StanceNeutral}}

	for _, a := range Actions(v, cfg, draft) {
		if a.Kind == game.ActionNothing {
			return
		}
	}
	t.Error("Actions did not include ActionNothing")
}

// TestActionsForcesNothingUnderPushingOn is GDD §9.1/§15.0's rule that
// Pushing On may never be combined with an action ("you have no idea where
// you'll be") — Actions must collapse to exactly [ActionNothing] rather
// than trialling (and rejecting) every other candidate one by one.
func TestActionsForcesNothingUnderPushingOn(t *testing.T) {
	cfg := game.DefaultConfig()
	v := game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{2}},
			2: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{99}},
			// 99 is absent from Nodes — a real, adjacency-reachable Hidden
			// ending, unlike an arbitrary NodeID with no edge leading to it.
		},
	}
	bias := game.SectorOldDocks
	draft := game.Order{
		Round:     v.Round,
		Route:     []game.NodeID{2, 99},
		PushingOn: game.PushingOn{Steps: 1, Bias: &bias},
		Stance:    game.StanceOrder{Stance: game.StanceNeutral},
	}

	actions := Actions(v, cfg, draft)
	if len(actions) != 1 || actions[0].Kind != game.ActionNothing {
		t.Fatalf("Actions under a declared Pushing On = %v, want exactly [ActionNothing]", actions)
	}
}

// TestPushingOnsOnlyWhenRouteEndsHidden checks PushingOns' own gate (GDD
// §9.1): a route ending on a node the view already discloses offers only
// the not-declared zero value, while a route ending on a genuinely Hidden
// node offers real declarations too, each within the legal 0-2 blind-step
// range.
func TestPushingOnsOnlyWhenRouteEndsHidden(t *testing.T) {
	v := fixtureView(4, 2)

	knownEnding := PushingOns(v, []game.NodeID{3}) // node 3 is Known in fixtureView
	if len(knownEnding) != 1 || pushingDeclared(knownEnding[0]) {
		t.Fatalf("PushingOns for a Known ending node = %+v, want only the not-declared zero value", knownEnding)
	}

	hiddenEnding := PushingOns(v, []game.NodeID{2, 99}) // 99 is absent from v
	sawDeclared := false
	for _, p := range hiddenEnding {
		if pushingDeclared(p) {
			sawDeclared = true
			if p.Steps < 0 || p.Steps > 2 {
				t.Errorf("PushingOn %+v declares a blind step count outside the legal 0-2 range", p)
			}
		}
	}
	if !sawDeclared {
		t.Error("PushingOns for a Hidden ending node never declared a real continuation")
	}
}

// TestSampleImmobilisedPlayerYieldsLegalOrder is issue #191's own stated
// case: "a view where the player is immobilised still yields a legal order
// (the empty route is legal) rather than an error." The fixture node has no
// Edges at all, so Routes' only candidate is the empty route, and Sample
// must never panic or otherwise fail to produce something rules.Legal
// accepts, across many different BotRNG draws.
func TestSampleImmobilisedPlayerYieldsLegalOrder(t *testing.T) {
	cfg := game.DefaultConfig()
	v := game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeAlley}, // no Edges
		},
	}

	for seed := range byte(20) {
		r := rules.NewBotRNG(testSeed(seed), game.SeatID(0), 4)
		o := Sample(v, cfg, r)

		if len(o.Route) != 0 {
			t.Fatalf("seed %d: an isolated node produced a non-empty route %v", seed, o.Route)
		}
		if err := rules.Legal(v, o, cfg); err != nil {
			t.Fatalf("seed %d: illegal order %+v: %v", seed, o, err)
		}
	}
}

// TestSampleWithHeldItemsAndSuppressedDealAlwaysLegal exercises the item-
// discard path CodeRabbit's review flagged as undertested on this PR: a
// view holding every discardable item (all eight minus Muscle, which
// Sample must never declare regardless), with a Rumoured, a Known and a
// Hidden neighbour so Decoy, Police Band and Bolt Hole each have a real
// target to draw, and Suppress.Items set so a discard that only makes Deal
// affordable is never the point of discarding here. This is also the
// scenario most likely to drive legalActionsOrRelax's fallback path (a
// sampled discard set making the whole draft illegal for a reason
// unrelated to Action) if one still exists.
func TestSampleWithHeldItemsAndSuppressedDealAlwaysLegal(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Items = true

	v := game.PlayerView{
		Round: 4,
		You: game.SelfState{
			Position: 1,
			Items: []game.ItemID{
				game.ItemShiv,
				game.ItemMuscle,
				game.ItemPoliceBand,
				game.ItemCirculationPermit,
				game.ItemTornMap,
				game.ItemDecoy,
				game.ItemBoltHole,
				game.ItemGuardContact,
			},
		},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{2, 3}},
			2: {Fog: game.FogRumoured, Type: game.NodeWarehouse}, // Police Band's own target case — no Edges
			3: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{4}},
			4: {Fog: game.FogKnown, Type: game.NodeAlley}, // Bolt Hole's own distance-2 target
		},
	}

	for seed := range byte(60) {
		r := rules.NewBotRNG(testSeed(seed), game.SeatID(0), 4)
		o := Sample(v, cfg, r)

		if err := rules.Legal(v, o, cfg); err != nil {
			t.Fatalf("seed %d: illegal order %+v: %v", seed, o, err)
		}
		for _, d := range o.Items {
			if d.Item == game.ItemMuscle {
				t.Fatalf("seed %d: Muscle declared as a discard: %+v", seed, o.Items)
			}
		}
	}
}

// TestSampleCollapsesUnderMaximalConstraint is issue #191's "given a view
// where exactly one order is legal, the generator produces exactly that
// order" acceptance criterion — worked against the most constrained view
// this rule set can actually produce.
//
// That view turns out not to have literally one legal order. rules.Legal
// accepts ActionNothing, ActionVanish and ActionSurveil unconditionally —
// GDD §9.2 lists both as legal "Anywhere", with no fog, cargo, balance or
// node-type precondition — and every one of the three Stance values, at a
// stake of 0, is equally unconditional (GDD §9.3). No Config knob removes
// either freedom: Suppress.Leases and Suppress.Items only remove Stake Post
// and Deal, and DockersStrike only removes Pickup. So under the rules
// engine as it stands, no game.PlayerView has exactly one legal order —
// Action and Stance remain a live 3-way choice even at the single most
// constrained node the map can produce. This is a property of GDD §9.2-9.3
// as written, not a gap in Legal or in this generator, so it is not filed
// back to M1 the way a real projection defect would be (issue #191's own
// "every projection defect found is filed as its own issue" instruction).
//
// This test instead asserts every field this view CAN force collapses to
// its one legal value regardless of the BotRNG seed — route, Pushing On,
// item discards, add-ons, abandoned cargo, contract choice — and confirms
// Action and Stance never wander outside that always-legal trio.
func TestSampleCollapsesUnderMaximalConstraint(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Leases = true
	cfg.Suppress.Items = true

	round := game.RoundNumber(cfg.Rounds) // final round: also blocks Buy Ledger
	v := game.PlayerView{
		Round: round,
		You: game.SelfState{
			Position:      1,
			Balance:       0,
			DockersStrike: true,
		},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeAlley}, // no Edges, no Market
		},
	}

	for seed := range byte(40) {
		r := rules.NewBotRNG(testSeed(seed), game.SeatID(0), int(round))
		o := Sample(v, cfg, r)

		if err := rules.Legal(v, o, cfg); err != nil {
			t.Fatalf("seed %d: illegal order %+v: %v", seed, o, err)
		}
		if len(o.Route) != 0 {
			t.Errorf("seed %d: forced-empty route produced %v", seed, o.Route)
		}
		if pushingDeclared(o.PushingOn) {
			t.Errorf("seed %d: pushing on declared at a non-Hidden ending node: %+v", seed, o.PushingOn)
		}
		if len(o.Items) != 0 {
			t.Errorf("seed %d: item discards produced with no items held: %v", seed, o.Items)
		}
		if !o.AddOns.Equal(game.AddOns{}) {
			t.Errorf("seed %d: add-ons produced at zero balance: %+v", seed, o.AddOns)
		}
		if o.AbandonCargo {
			t.Errorf("seed %d: cargo abandoned with none carried", seed)
		}
		if o.ContractChoice != nil {
			t.Errorf("seed %d: contract choice produced with no offer pending: %d", seed, *o.ContractChoice)
		}
		switch o.Action.Kind {
		case game.ActionNothing, game.ActionVanish, game.ActionSurveil:
		default:
			t.Errorf("seed %d: action %s was not suppressed by DockersStrike/Suppress.Leases/Suppress.Items", seed, o.Action.Kind)
		}
		if o.Stance.Stake != 0 {
			t.Errorf("seed %d: nonzero stake at zero balance: %+v", seed, o.Stance)
		}
	}
}

// TestSampleCorpusFromGoldenMatches is issue #191's central acceptance
// criterion: every order Sample produces, accepted by rules.Legal, over a
// corpus of views taken from a full 15-round golden match at 2, 3, 4 and 5
// players. The match is driven entirely by Sample itself — every seat,
// every round — rather than by a hand-scripted seat 0 the way
// internal/rules/golden_test.go's own scenario is: that is the more
// meaningful test here, since it means every order this generator ever
// produces during a real 15-round match, for every seat, is asserted
// legal, not just a curated sample of them.
//
// Beyond the one order that actually advances the match each round, this
// draws drawsPerView-1 further samples from the same view with different
// BotRNG seeds, purely to widen the corpus for the fails-closed variety
// assertions below — a single draw per view could not be expected to
// reliably hit a multi-step route, a non-Nothing action and a
// ContractChoice all within one run.
func TestSampleCorpusFromGoldenMatches(t *testing.T) {
	cfg := game.DefaultConfig()
	const rounds = 15
	const drawsPerView = 6

	var (
		orders              int
		sawMultiStepRoute   bool
		sawNonNothingAction bool
		sawContractChoice   bool
	)

	for _, players := range []int{2, 3, 4, 5} {
		seed := testSeed(byte(0x50 + players))

		s, err := rules.NewMatch(seed, cfg, players)
		if err != nil {
			t.Fatalf("%d players: NewMatch: %v", players, err)
		}

		for round := 1; round <= rounds; round++ {
			seatOrders := make(map[game.SeatID]game.Order, players)

			for seat := range players {
				sid := game.SeatID(seat)
				v := rules.Project(s, sid)

				for draw := range drawsPerView {
					r := rules.NewBotRNG(drawSeed(seed, draw), sid, round)
					o := Sample(v, cfg, r)

					if err := rules.Legal(v, o, cfg); err != nil {
						t.Fatalf("%d players, round %d, seat %d, draw %d: illegal order %+v: %v",
							players, round, seat, draw, o, err)
					}
					orders++
					if len(o.Route) >= 2 {
						sawMultiStepRoute = true
					}
					if o.Action.Kind != game.ActionNothing {
						sawNonNothingAction = true
					}
					if o.ContractChoice != nil {
						sawContractChoice = true
					}
					if draw == 0 {
						seatOrders[sid] = o
					}
				}
			}

			rng := rules.NewRNG(seed, round)
			s, _, err = rules.Resolve(s, seatOrders, cfg, rng)
			if err != nil {
				t.Fatalf("%d players, round %d: Resolve: %v", players, round, err)
			}
		}
	}

	if orders == 0 {
		t.Fatal("corpus produced zero orders — the driven loop ran over nothing")
	}
	if !sawMultiStepRoute {
		t.Error("corpus never produced a route of length >= 2 — fails closed")
	}
	if !sawNonNothingAction {
		t.Error("corpus never produced a non-Nothing action — fails closed")
	}
	if !sawContractChoice {
		t.Error("corpus never produced a ContractChoice — fails closed")
	}
}

// TestGeneratorOnlyUsesEnumeratorRulesSymbols is issue #191's own stated
// acceptance criterion: "a test fails if the package's import of
// internal/rules is used for anything other than Legal, Affordances, Steps
// and *RNG." Scoped to this package's non-test .go files — the generator's
// own production code — since the test files legitimately need the wider
// rules API (NewMatch, Resolve, Project, NewRNG) to build a golden-match
// corpus to test the generator against, which is a different thing from
// what the generator itself is built out of.
func TestGeneratorOnlyUsesEnumeratorRulesSymbols(t *testing.T) {
	allowed := map[string]bool{
		"Legal":       true,
		"Affordances": true,
		"Steps":       true,
		"BotRNG":      true,
		"NewBotRNG":   true,
		"BotPurpose":  true,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/bots directory: %v", err)
	}

	fset := token.NewFileSet()
	filesScanned := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		filesScanned++

		// The local name this file's own import binds internal/rules to —
		// scanning for the literal identifier "rules" would silently miss
		// an aliased or dot-import of the same package.
		local := ""
		for _, imp := range file.Imports {
			if imp.Path.Value != `"github.com/garnizeh/cinzal/internal/rules"` {
				continue
			}
			local = "rules"
			if imp.Name != nil {
				if imp.Name.Name == "." {
					t.Fatalf("%s: dot-imports internal/rules, which this check cannot see through", name)
				}
				local = imp.Name.Name
			}
		}
		if local == "" {
			continue // this file does not import internal/rules at all
		}

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != local {
				return true
			}
			if !allowed[sel.Sel.Name] {
				t.Errorf("%s: references rules.%s at %s, outside {Legal, Affordances, Steps, BotRNG, NewBotRNG, BotPurpose}",
					name, sel.Sel.Name, fset.Position(sel.Pos()))
			}
			return true
		})
	}

	if filesScanned == 0 {
		t.Fatal("scanned zero non-test .go files in internal/bots — the directory walk found nothing, which would make this test vacuously pass")
	}
}
