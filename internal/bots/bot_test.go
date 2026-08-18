package bots

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// fixtureView builds a small, hand-constructed PlayerView over a 4-node
// line graph (1-2-3-4, all Known), rather than deriving one from a
// rules.MatchState via rules.Project: this package's own no-MatchState rule
// (TestPackageNamesNoMatchState below) applies to its test files too, so
// fixtures here are built the same way any other consumer of game.PlayerView
// alone would build one.
func fixtureView(round game.RoundNumber, at game.NodeID) game.PlayerView {
	edges := map[game.NodeID][]game.NodeID{
		1: {2},
		2: {1, 3},
		3: {2, 4},
		4: {3},
	}

	nodes := make(map[game.NodeID]game.NodeView, len(edges))
	for id, e := range edges {
		nodes[id] = game.NodeView{Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: e}
	}

	return game.PlayerView{
		Round: round,
		You:   game.SelfState{Position: at},
		Nodes: nodes,
	}
}

// driveRounds calls b.Decide once per round 1..n, alternating the seat
// between nodes 2 and 3 of fixtureView's graph — both two-edge nodes — so a
// bot that ever moves has, across n rounds, ample opportunity to.
func driveRounds(t *testing.T, b Bot, cfg game.Config, seed [32]byte, seat game.SeatID, n int) []game.Order {
	t.Helper()

	orders := make([]game.Order, n)
	at := game.NodeID(2)
	for round := 1; round <= n; round++ {
		v := fixtureView(game.RoundNumber(round), at)
		r := rules.NewBotRNG(seed, seat, round)
		orders[round-1] = b.Decide(v, cfg, r)

		if at == 2 {
			at = 3
		} else {
			at = 2
		}
	}
	return orders
}

// TestDecideIsDeterministic asserts Decide called twice with the same
// (view, config, BotRNG state) returns identical orders — the acceptance
// criterion issue #190 states as a deep equality over game.Order, including
// Route element order.
func TestDecideIsDeterministic(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{0x11}
	view := fixtureView(4, 2)

	for _, tier := range []Tier{Drifter, Runner, Operator} {
		t.Run(tier.String(), func(t *testing.T) {
			b := For(tier)

			got := b.Decide(view, cfg, rules.NewBotRNG(seed, game.SeatID(0), 4))
			want := b.Decide(view, cfg, rules.NewBotRNG(seed, game.SeatID(0), 4))

			if !got.Equal(want) {
				t.Fatalf("Decide is not deterministic for %s: %+v != %+v", tier, got, want)
			}
			if !equalRoute(got.Route, want.Route) {
				t.Fatalf("Route element order diverged for %s: %v != %v", tier, got.Route, want.Route)
			}
		})
	}
}

func equalRoute(a, b []game.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestBotHasNoCrossRoundState is the executable form of the no-memory rule:
// one Bot driven through fifteen rounds must produce the identical order
// sequence a freshly constructed Bot produces when driven through the same
// fifteen views.
//
// The first sequence is taken from a Bot that has already been driven
// through this exact fifteen-view run once before, rather than from an
// instance used for the first time: if an implementation ever accumulated
// state across Decide calls, two never-yet-used instances driven through
// the same view sequence would still evolve identically call-for-call and
// this test would not notice — the state would be a pure function of call
// count, not of history. Reusing an already-driven instance for the first
// comparison arm is what would expose that history-dependence.
//
// Fails closed: a bot that returned fifteen zero-valued orders would pass
// the equality assertion below trivially, so this test separately asserts
// the driven sequence is non-empty and that at least one order in it has a
// non-empty Route (issue #190's own fails-closed criterion).
func TestBotHasNoCrossRoundState(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{0x22}
	const rounds = 15

	for _, tier := range []Tier{Drifter, Runner, Operator} {
		t.Run(tier.String(), func(t *testing.T) {
			used := For(tier)
			_ = driveRounds(t, used, cfg, seed, game.SeatID(1), rounds)
			first := driveRounds(t, used, cfg, seed, game.SeatID(1), rounds)
			second := driveRounds(t, For(tier), cfg, seed, game.SeatID(1), rounds)

			if len(first) != rounds || len(second) != rounds {
				t.Fatalf("driven sequence has length (%d, %d), want (%d, %d)", len(first), len(second), rounds, rounds)
			}

			sawMove := false
			for i := range first {
				if !first[i].Equal(second[i]) {
					t.Fatalf("round %d: a freshly constructed %s diverged from the first instance: %+v != %+v", i+1, tier, first[i], second[i])
				}
				if len(first[i].Route) > 0 {
					sawMove = true
				}
			}
			if !sawMove {
				t.Fatalf("%s: every one of %d driven orders had an empty Route — a bot that never moves would pass every equality assertion above and this test would not have caught it", tier, rounds)
			}
		})
	}
}

// TestDecideProducesLegalOrders is issue #190's own stated obligation:
// "Decide must return an order that rules.Legal(v, o, cfg) accepts... a
// test obligation rather than a runtime one." Driven over the same fixture
// and seat as TestBotHasNoCrossRoundState, so a change to decideStayOrMove
// that broke legality would be caught by the same corpus.
func TestDecideProducesLegalOrders(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{0x33}
	const rounds = 15

	for _, tier := range []Tier{Drifter, Runner, Operator} {
		t.Run(tier.String(), func(t *testing.T) {
			at := game.NodeID(2)
			for round := 1; round <= rounds; round++ {
				v := fixtureView(game.RoundNumber(round), at)
				r := rules.NewBotRNG(seed, game.SeatID(2), round)
				o := For(tier).Decide(v, cfg, r)

				if err := rules.Legal(v, o, cfg); err != nil {
					t.Fatalf("round %d: %s produced an illegal order %+v: %v", round, tier, o, err)
				}

				if at == 2 {
					at = 3
				} else {
					at = 2
				}
			}
		})
	}
}

// TestForPanicsOnUndeclaredTier asserts For fails closed on a Tier outside
// the three declared constants, rather than silently returning a nil Bot a
// caller would only discover on the first Decide call.
func TestForPanicsOnUndeclaredTier(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("For(Tier(99)) did not panic")
		}
	}()
	For(Tier(99))
}

// TestPackagePackagesDependOnlyOnGameAndRules is issue #190's
// "go list -deps ./internal/bots contains internal/game and internal/rules
// and no other internal/ package" acceptance criterion, checked the same
// way internal/rules/state_test.go's TestGameDoesNotImportRules checks its
// own package-graph property: by shelling out to go list rather than
// trusting a hand-maintained list, so a new internal/ import anywhere in
// this package is caught without the test needing to change.
func TestPackagePackagesDependOnlyOnGameAndRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/garnizeh/cinzal/internal/bots").Output()
	if err != nil {
		t.Fatalf("go list -deps internal/bots: %v", err)
	}

	deps := strings.Fields(string(out))
	if len(deps) == 0 {
		t.Fatal("go list -deps internal/bots reported nothing — the check ran over no packages, which is not the same as passing")
	}

	const (
		module = "github.com/garnizeh/cinzal/internal/"
		game   = module + "game"
		rules  = module + "rules"
		self   = module + "bots"
	)
	sawGame, sawRules := false, false
	for _, d := range deps {
		if !strings.HasPrefix(d, module) {
			continue // standard library or a third-party module
		}
		switch {
		case d == game || strings.HasPrefix(d, game+"/"):
			sawGame = true
		case d == rules || strings.HasPrefix(d, rules+"/"):
			sawRules = true
		case d == self:
			// this package itself
		default:
			t.Errorf("internal/bots depends on %s, which is under neither internal/game nor internal/rules", d)
		}
	}
	if !sawGame {
		t.Error("internal/bots does not depend on internal/game")
	}
	if !sawRules {
		t.Error("internal/bots does not depend on internal/rules")
	}
}

// TestPackageNamesNoMatchStateNowhere is issue #190's source-level stand-in
// for the type-level CI gate issue #195 adds later: "internal/bots names
// rules.MatchState nowhere." It parses every .go file in this directory,
// including test files — a fixture built from rules.MatchState would be
// exactly the first place this boundary quietly leaks — and fails if any
// identifier named MatchState appears anywhere in the AST. Comments are not
// walked by go/parser's AST, so prose discussing MatchState by name (as this
// very doc comment does) does not trip the check; only a real reference to
// the identifier would.
func TestPackageNamesNoMatchStateNowhere(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/bots directory: %v", err)
	}

	fset := token.NewFileSet()
	filesScanned := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		filesScanned++

		ast.Inspect(file, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == "MatchState" {
				t.Errorf("%s: names MatchState at %s — internal/bots must never reference rules.MatchState", name, fset.Position(ident.Pos()))
			}
			return true
		})
	}

	if filesScanned == 0 {
		t.Fatal("scanned zero .go files in internal/bots — the directory walk found nothing, which would make this test vacuously pass")
	}
}
