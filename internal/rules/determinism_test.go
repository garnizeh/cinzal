package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is issue #77's determinism suite: "seed + order log reproduces
// a match exactly, forever" as a checked property (RFC §16.1-16.2), not an
// intention. It is deliberately separate from golden_test.go (#76's own
// acceptance test, GDD-band scoring at 4 players only) — the two share
// fogAwareRoute and the shape of a scripted seat-0 bot loop, but this file's
// job is RNG index accounting and byte-identical replay, never scoring.

// --- fixtures ---

// roundOrders is one round's submitted orders, keyed by seat.
type roundOrders map[game.SeatID]game.Order

// orderLog is a full match's recorded orders, one roundOrders per round
// number — exactly the shape RFC §7.1's state = fold(Resolve,
// initial(seed, cfg), orderLog) folds over. Recording this once (via
// runDeterminismScript, below) and then replaying it through foldLog is
// what makes "fold(log) == fold(log)" (RFC §16.2) a test of the fold
// itself, never of a live decision loop that merely happens to be
// deterministic too.
type orderLog map[int]roundOrders

// determinismFixture is one player-count's scripted scenario.
type determinismFixture struct {
	players int
	seed    [32]byte
}

// determinismFixtures covers every player count the game supports (GDD §1:
// 2-5), each seeded independently so a fixture that needs adjusting (a
// seed whose generated map this script can't complete 15 rounds against)
// never has to share a seed with another player count.
var determinismFixtures = []determinismFixture{
	{players: 2, seed: testSeed(101)},
	{players: 3, seed: testSeed(102)},
	{players: 4, seed: testSeed(103)},
	{players: 5, seed: testSeed(104)},
}

// recordDeterminismScript plays one full 15-round match — seat 0 scripted,
// every other seat idle — exactly the shape
// TestGoldenMatchFinalScoreLandsInGDDBands (golden_test.go, issue #76)
// already established, generalized to an arbitrary player count and a
// dynamically-located Border (rather than a hardcoded node) so it isn't
// pinned to one seed's own map layout. It both plays the match live
// (recording the order log as it goes) and returns the final state, so
// callers that only need the log or the pinned final output can use either
// return value without re-deriving the other.
//
// This is not a copy for its own sake: golden_test.go's own scenario is
// scoped to GDD-band scoring at 4 players (issue #76's acceptance
// criterion) and must not be rewritten to serve a second, unrelated goal
// (CLAUDE.md: no gratuitous refactors of a working test). This suite's
// goal — RNG index accounting and byte-identical replay across every
// player count — genuinely needs its own driver.
//
// It returns a plain error rather than calling t.Fatalf directly, so it can
// be memoized once per player count (determinismScriptCache, below) and
// shared across every test that needs the recording, instead of each test
// independently replaying the identical real match.
func recordDeterminismScript(seed [32]byte, players int) (final MatchState, log orderLog, cfg game.Config, err error) {
	cfg = game.DefaultConfig()

	s, err := initial(seed, cfg, players)
	if err != nil {
		return MatchState{}, nil, cfg, fmt.Errorf("initial() error = %w", err)
	}
	homeSector := s.Graph.Nodes[s.Players[0].Position].Sector

	var knownBorder game.NodeID
	foundBorder := false
	for _, n := range s.Graph.Nodes {
		if n.Type == game.NodeBorder {
			knownBorder, foundBorder = n.ID, true
			break
		}
	}
	if !foundBorder {
		return MatchState{}, nil, cfg, fmt.Errorf("seed %x at %d players produced a map with no Border node at all — needs a different seed", seed, players)
	}

	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	postsStaked := 0
	const maxPosts = 2
	var crateAt *game.NodeID

	// blocksNeeded mirrors golden_test.go's own helper of the same name and
	// the same reasoning: the fewest lease blocks whose rounds survive from
	// round through cfg.Rounds's own Upkeep decrement.
	blocksNeeded := func(round game.RoundNumber) int {
		remaining := cfg.Rounds - int(round) + 1
		for blocks := 1; blocks <= 4; blocks++ {
			if blocks*cfg.LeaseBlockRounds > remaining {
				return blocks
			}
		}
		return 4
	}

	log = orderLog{}

	// offerCutoffRoundsBeforeEnd: stop taking new Tier 0 offers once fewer
	// than this many rounds remain, mirroring golden_test.go's own reasoning
	// (predictPressureD6's doc comment below repeats it) — a contract still
	// active at match end costs GDD §16's -2 penalty instead of its RP, so a
	// fixed "leave 2 rounds of slack" cutoff scales with cfg.Rounds rather
	// than hardcoding an absolute round number that silently stops meaning
	// the same thing if cfg.Rounds ever changes.
	const offerCutoffRoundsBeforeEnd = 2
	offerCutoffRound := game.RoundNumber(cfg.Rounds - offerCutoffRoundsBeforeEnd)

	for round := game.RoundNumber(1); round <= game.RoundNumber(cfg.Rounds); round++ {
		var contractChoice *int
		if round <= offerCutoffRound && len(s.Players[0].Contracts) < 2 {
			for i, o := range s.Players[0].PendingOffer {
				if o.Tier == 0 {
					idx := i
					contractChoice = &idx
					break
				}
			}
		}

		p := s.Players[0]

		var target game.NodeID
		var action game.ActionKind
		hasTarget := false
		stakeHere := false

		readyToStake := round >= 10 && postsStaked < maxPosts
		if p.Cargo == nil && readyToStake && s.Graph.Nodes[p.Position].Post == nil && s.Graph.Nodes[p.Position].Sector == homeSector {
			stakeHere = true
		} else if p.Cargo == nil && readyToStake {
			for _, n := range s.Graph.Nodes[p.Position].Edges {
				if s.Graph.Nodes[n].Post == nil && s.Graph.Nodes[n].Sector == homeSector {
					target, action, hasTarget = n, game.ActionStakePost, true
					break
				}
			}
		}
		if !stakeHere && !hasTarget {
			switch {
			case p.Cargo != nil && p.Cargo.Bound:
				if idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == p.Cargo.Contract }); idx >= 0 {
					target, action, hasTarget = p.Contracts[idx].Destination, game.ActionDeliver, true
				}
			case p.Cargo != nil:
				target, action, hasTarget = knownBorder, game.ActionDeliver, true
			case crateAt != nil:
				target, action, hasTarget = *crateAt, game.ActionPickup, true
				crateAt = nil
			case len(p.Contracts) > 0:
				target, action, hasTarget = p.Contracts[0].Origin, game.ActionPickup, true
			}
		}

		var route []game.NodeID
		if hasTarget {
			route = fogAwareRoute(s.Graph, p.Fog, p.Position, target)
		}

		maxSteps := cfg.StepsByTier[infamyTierIndex(p.Infamy, cfg)]
		routeThisRound := route
		if len(routeThisRound) > maxSteps {
			routeThisRound = routeThisRound[:maxSteps]
		}
		arrived := hasTarget && len(routeThisRound) == len(route)

		order := game.Order{Route: routeThisRound, Stance: game.StanceOrder{Stance: game.StanceNeutral}, ContractChoice: contractChoice}
		switch {
		case stakeHere:
			order.Action = game.ActionOrder{Kind: game.ActionStakePost}
			order.AddOns.RenewBlocks = blocksNeeded(round)
			postsStaked++
		case arrived:
			order.Action = game.ActionOrder{Kind: action}
			if action == game.ActionStakePost {
				order.AddOns.RenewBlocks = blocksNeeded(round)
				postsStaked++
			}
		default:
			order.Action = game.ActionOrder{Kind: game.ActionNothing}
		}

		orders := roundOrders{0: order}
		for seat := 1; seat < players; seat++ {
			orders[game.SeatID(seat)] = idleOrder
		}
		log[int(round)] = orders

		next, events, err := Resolve(s, orders, cfg, NewRNG(seed, int(round)))
		if err != nil {
			return MatchState{}, nil, cfg, fmt.Errorf("round %d: Resolve() error = %w", round, err)
		}
		for _, e := range events {
			if e.Kind == game.EventDeadRunnerCrate || e.Kind == game.EventSpilledLoadCrate {
				node := e.Node
				crateAt = &node
			}
		}
		s = next
	}

	return s, log, cfg, nil
}

// determinismScriptResult is recordDeterminismScript's memoized output.
type determinismScriptResult struct {
	final MatchState
	log   orderLog
	cfg   game.Config
	err   error
}

// determinismScriptCache memoizes recordDeterminismScript once per player
// count (sync.OnceValue, so concurrent subtests share a single recording
// rather than racing to produce their own). The recorded log is an
// immutable, pure function of (seed, players) — replaying the identical
// real 15-round match once per fixture and sharing the result across every
// test that needs it (TestFoldLogIsPureSameLogProducesSameFinalState,
// TestGoldenFixturesAreByteIdenticalAcrossFiftyInProcessRuns,
// TestGoldenFixturesFinalStateMatchesCommittedHash,
// TestPerRoundRNGConsumptionMatchesPredictions) changes nothing any of them
// assert, since none of them ever mutate what they get back.
var determinismScriptCache = func() map[int]func() determinismScriptResult {
	cache := make(map[int]func() determinismScriptResult, len(determinismFixtures))
	for _, fx := range determinismFixtures {
		cache[fx.players] = sync.OnceValue(func() determinismScriptResult {
			final, log, cfg, err := recordDeterminismScript(fx.seed, fx.players)
			return determinismScriptResult{final: final, log: log, cfg: cfg, err: err}
		})
	}
	return cache
}()

// runDeterminismScript is the *testing.T-facing entry point every test in
// this file actually calls: a cached lookup by player count (see
// determinismScriptCache) that turns a stored recording error into
// t.Fatalf, so callers never have to check an error themselves.
func runDeterminismScript(t *testing.T, players int) (final MatchState, log orderLog, cfg game.Config) {
	t.Helper()
	get, ok := determinismScriptCache[players]
	if !ok {
		t.Fatalf("no determinism fixture registered for %d players", players)
	}
	result := get()
	if result.err != nil {
		t.Fatalf("recordDeterminismScript(%d players): %v", players, result.err)
	}
	return result.final, result.log, result.cfg
}

// foldLog is state = fold(Resolve, initial(seed, cfg), orderLog) (RFC
// §7.1), literally: it replays a previously-recorded log — never a live
// decision loop — from a fresh initial() call. This is the function
// TestFoldLogIsPureSameLogProducesSameFinalState and
// TestGoldenFixturesAreByteIdenticalAcrossFiftyInProcessRuns actually
// exercise repeatedly; runDeterminismScript is only ever called once per
// fixture, to produce the log in the first place.
func foldLog(seed [32]byte, cfg game.Config, players int, log orderLog) (MatchState, []game.Event, error) {
	s, err := initial(seed, cfg, players)
	if err != nil {
		return MatchState{}, nil, err
	}

	var allEvents []game.Event
	for round := 1; round <= cfg.Rounds; round++ {
		var events []game.Event
		s, events, err = Resolve(s, log[round], cfg, NewRNG(seed, round))
		if err != nil {
			return MatchState{}, nil, err
		}
		allEvents = append(allEvents, events...)
	}
	return s, allEvents, nil
}

// canonicalState renders s as a stable byte string for equality comparison
// across independent runs. encoding/json over every exported field (state.go
// confirms MatchState and everything it embeds are exported top to bottom)
// rather than reflect.DeepEqual: the two Sight/Obscured maps buried in
// game.SeatArchive are genuine Go maps, and encoding/json sorts map keys on
// marshal regardless of the key's underlying type — which is exactly what
// turns Go's own deliberately-randomised map iteration (RFC §6.3) from a
// hazard for THIS comparison into a non-issue, while still leaving it fully
// able to catch that same hazard if it leaks into Resolve's own resolution
// order (that's map-iteration-hazard test's job, not this function's).
func canonicalState(t *testing.T, s MatchState) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(MatchState) error = %v", err)
	}
	return string(b)
}

func canonicalEvents(t *testing.T, events []game.Event) string {
	t.Helper()
	b, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("json.Marshal([]game.Event) error = %v", err)
	}
	return string(b)
}

// --- RFC §16.2's two purity invariants ---
//
// "resolve(s, o) == resolve(s, o)" — RFC §16.2's first invariant, plus
// issue #67's own "the input is byte-identical afterwards" — is already
// covered exactly: resolve_test.go's TestResolveIsDeterministic (its own
// doc comment quotes RFC §16.2 verbatim) and TestResolveDoesNotMutateInput.
// Not duplicated here; see TestFoldLogIsPureSameLogProducesSameFinalState
// below for this file's own contribution, the second invariant, run across
// every player count rather than the fixed two-seat fixture those two
// tests use.

// TestFoldLogIsPureSameLogProducesSameFinalState is RFC §16.2's second
// invariant, "fold(log) == fold(log)": refolding the identical recorded
// order log from a fresh initial() twice produces byte-identical final
// states and event streams, at every player count.
func TestFoldLogIsPureSameLogProducesSameFinalState(t *testing.T) {
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			_, log, cfg := runDeterminismScript(t, fx.players)

			s1, events1, err1 := foldLog(fx.seed, cfg, fx.players, log)
			s2, events2, err2 := foldLog(fx.seed, cfg, fx.players, log)
			if err1 != nil || err2 != nil {
				t.Fatalf("foldLog() errors = %v, %v", err1, err2)
			}

			if canonicalState(t, s1) != canonicalState(t, s2) {
				t.Error("fold(log) produced two different final states from the identical recorded log")
			}
			if canonicalEvents(t, events1) != canonicalEvents(t, events2) {
				t.Error("fold(log) produced two different event streams from the identical recorded log")
			}
		})
	}
}

// --- the map-iteration hazard, tested rather than trusted ---

// TestGoldenFixturesAreByteIdenticalAcrossFiftyInProcessRuns is the issue's
// own acceptance criterion: "each golden fixture runs >= 50 times
// in-process with byte-identical results." Go deliberately randomises map
// iteration order (RFC §6.3), so running the identical fold 50 times in one
// process is a real detector for a stray `for k := range m` anywhere in
// resolution — a bug that a single run, or 50 runs each in a fresh process,
// would never surface.
func TestGoldenFixturesAreByteIdenticalAcrossFiftyInProcessRuns(t *testing.T) {
	const runs = 50
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			_, log, cfg := runDeterminismScript(t, fx.players)

			var want string
			for i := range runs {
				final, _, err := foldLog(fx.seed, cfg, fx.players, log)
				if err != nil {
					t.Fatalf("run %d: foldLog() error = %v", i, err)
				}
				got := canonicalState(t, final)
				if i == 0 {
					want = got
					continue
				}
				if got != want {
					t.Fatalf("run %d produced a different final state than run 0 for the identical seed/log — a stray map-range in resolution (RFC §6.3) looks exactly like this", i)
				}
			}
		})
	}
}

// --- golden final-state pin ---

// goldenHashes pins each fixture's final canonical state — issue #77: "an
// unintended rule change fails these... regenerating one is a deliberate
// act with a stated reason in the pull request." A hash rather than the
// full JSON blob: this is a drift detector, not a restatement of GDD
// scoring (#76 already owns the scoring bands), so it deliberately checks
// nothing about *what* the numbers mean — only that they haven't moved.
// Regenerating a hash here without a stated reason in the PR is exactly
// the failure mode the issue calls out by name.
var goldenHashes = map[int]string{
	2: "8261d780866835c1adc90a7dd4984a6515e02db99fe0ae316b2aac25992f0f8e",
	3: "790bee67c5ed42be3509d7e1abbf3496af00159e1e6507187ab2a403d2e91e7d",
	4: "749bf75a40b358f26d187e2dc5c3cd235b1990122c59a7a4e974bfe8561f17de",
	5: "c1887bd3edf6c847ee0fff961cc0d060aacf9a035bf972ba18093d5f4dd1a9f8",
}

// TestGoldenFixturesFinalStateMatchesCommittedHash is the "fixtures exist
// at 2, 3, 4 and 5 players and reach final scoring" acceptance criterion,
// plus the golden-fixture pin itself.
func TestGoldenFixturesFinalStateMatchesCommittedHash(t *testing.T) {
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			final, _, cfg := runDeterminismScript(t, fx.players)

			if int(final.Round) != cfg.Rounds {
				t.Fatalf("final.Round = %d, want %d", final.Round, cfg.Rounds)
			}
			breakdowns := FinalScore(final)
			if len(breakdowns) != fx.players {
				t.Fatalf("len(FinalScore()) = %d, want %d", len(breakdowns), fx.players)
			}

			sum := sha256.Sum256([]byte(canonicalState(t, final)))
			got := hex.EncodeToString(sum[:])
			want := goldenHashes[fx.players]
			if got != want {
				t.Errorf("%dp fixture's final-state hash = %s, want %s — either a genuine, deliberate rule change (regenerate and say why in the PR) or an unintended regression", fx.players, got, want)
			}
		})
	}
}

// --- per-round digest matrix (issue #80's exit demonstration) ---

// goldenPerRoundDigests pins, per player count, one sha256 digest per round
// — issue #80's own exit demonstration: unlike goldenHashes above (final
// state only), this is what lets a divergence name the ROUND it first
// appeared in when this suite runs on a second OS/arch in CI
// (.github/workflows/ci.yml's replay job). Each entry hashes both the
// state after that round's Resolve call and that round's own event stream
// — state alone would miss an events-only regression, events alone would
// miss a state-only one. Regenerating this table without a stated reason
// in the pull request is the same failure mode goldenHashes's own comment
// already warns about, here as much as there.
var goldenPerRoundDigests = map[int][]string{
	2: {
		"5e8547fffdc4be318aa9776cbbb5a2fe683a9079ce91ee568af439dda3035c98",
		"89924abaae303c646ef7fd14ef02e58dc3ceb400d7718e13c35f67c54fa83673",
		"6a0fd8e6f92952319979dc178baf21bcced8d97fb55439561a6e08ad62b526cb",
		"0989f28ecf20ee454f47f68d5f30b3c12637e73ba11073de05f080dae8ee54af",
		"1496f0df5ba0c51654f165048af79a0f8b45115dda074ab264fc3f58808f7774",
		"65dbab700edbe80a3f3dc8a68bc04458fad982bf0cabb48e8e53f4cc233389d5",
		"af32470ee0e55731b0a78500fb1a7739bcb0a98e0e0f3f9008e3361fe8de4fef",
		"2e54c0acf4758b1d0a1cd552abca2c39eeae4fc83c2fec773f8733dd84f4e5ae",
		"12cd8e4307877ba90a8aa74efa7f79c3ad654263581effb8d94da19ca9a75baa",
		"271159bc7e7c0067e7def21bf36364949fc37b09c77e866e44bea93f9e14b78a",
		"9a73a762940c4e8d76c9f44a03463a00ba8431c9ad89fbb16ea098287048f300",
		"a88c9539d45a704d8dc6559743451407beb82fe1efe8952fdb5fc6c2324b42d4",
		"4e0780023824764fa679015124807f7fe939113e006f8cef3c68b8c9ef438457",
		"d06738c71ff36a85ff394b748b672f3f5b089fc5994c944f982dc9e95aa9b8bf",
		"1c7c402a9b564b9da1371a683d0c7ef4101783bf00e619af0d45508e9abbd83c",
	},
	3: {
		"55e2ee7a74126de6020761708d5c10e0c3b3fcb6de0f3f428a4cc46259356b99",
		"5130aa573f00c4a9da0f758313ef3dba451dbd13a8f77437f36914d9fe41d939",
		"b5b23e3c2e7ea899cc2bb61eb78a5c785dc27cd041477e85d36e3371ad909b66",
		"ada7402a9ed0d7c56e9e8ffda1d5a4a902965a7302249a0a063f66523aebbd06",
		"2a2dc35a12469132c1697b543a692c3498f57e2982b554321e2d524c35b7e20d",
		"5eb7d8dc6a4efa38de332593d26bb40c05c54b8375135a601c7cbb5fedd8221d",
		"d273a0d9cd6a9167af239ba50b6e10b1bbd8cd2a2a9e1a5fd4805621a0ca345c",
		"121f365638215048284882c2f609e41c3a2639aed7b4e33d8e7034e592636a5e",
		"e6706902f4d47b6cf4d593095ff7f54043d6b9ee449eeaaf7e44e209b7f1075e",
		"c0e955f6ea36a05f6f0b2a92de3b389110fc005a58829b7f220a56d54b8e6906",
		"071447c36be0103744cfb9c75ea41280a9005cee25a8285e04faef22d26026ff",
		"c29e096f48a2932087eb333ec774b8974831f548e3770c6672c7d5544620da76",
		"b0870b590154bfacce6e896b0d9ecb56b779f773e8096a6e4c13ee7ad1e13c01",
		"d1bc5702b310789438259c5786d860a97ab9a3c023a5e71cfb9072b2f084cd9a",
		"d12f209a7bea7bef48bfd85790000e665600d46d464ecb72f004776f826518ba",
	},
	4: {
		"5a9d954d01ff01e37f0f863666db23710e34e873d78cf9191da15a2d69c83b3c",
		"c70d2013ddd26e56df948fdd632ae2a162c146897e130b058ce046ce20a5d3e7",
		"6b42519bf6272e79b18f15ba79147a10f17de1c935f4e2e474bb487fb604717b",
		"121f96b2a997fdce2a86b052cc158a04dfdeb928e8a0ab016f4aa97f7d98361a",
		"c12d60e1fea66d91fedfff8f94bd4c94882c344c8fe07ae7e3be26e602d3660d",
		"f37ce73c4bc328d36d5247f4df2bf903b74e3f4f5aab4d2b40abd8c6b64e5f5e",
		"a117660028851ed2ffe4ce136a7aebb4d67d3201f553d590ed8379b2064f3c96",
		"96fbe81b5430c17acf1a48cf0d7332b2cc426e552538bd8bbd6a8e8e271ffa3e",
		"a27a33a64a307d57a0b632e4b2c6fad91b5c724ecacdf6f0436a2d16002516fc",
		"69596f62f55fbce234cff6ff00659f9be35d523f5bda40e7c62d0a7a877e05f6",
		"09760ee8b23840be1dd70dbbbbfe288536cf3dfb735fbdc446680381afeda3fd",
		"9483043edc5b3e97d4800de60e5841a6a39bdefb3a4457f5b9dabdc35e3b2dde",
		"a8ae2b90bcc512b27a2fcba65b19c5733de80412a722b4ab0f7362afa200a883",
		"1d28248dae4bdea3cda2a58c6cd9930dd3d7553de180a90dd11c2f740141a6d3",
		"5ac4a0d32278bc48924e916f42c37ae6d457694a6f985115c324e56a51979388",
	},
	5: {
		"80d0afd4a6a56eb7cecf1d60ca8150a33553b02fc299b27ba1b0fed13cdcf403",
		"39ef5ecc9d68c9197a92ed06e2fa3b3f02217aedd3190b14c25635ee909b53ab",
		"a1a6d8b57958a8e9759abff84c1d4896f7dea9c21c4b095abb93bf63287f8776",
		"77f9031867c961e769337deea990b26751d7a7f9829565627ebb7740e71beae6",
		"f81937d04fda4dc7b2d744778ecfa7ed5eae41582828f454aee5bf47bdc694ee",
		"5b02157af181ea80f6cf269a42cb336f5bd49b91839291418be039927a98a47a",
		"50e7eb764ec83b8f2a9233882d0dab92cfe62e6cd5f838c6ad7c7993d736fc23",
		"e954e2cb51bf3624e6c763fb22dcf57c9acbaa2ebc86afbc5d9fca5448e69a69",
		"69e6e2274aa343ba9b13457b7af009c83d83ce6e1df203c0aa1aa5dc9c822979",
		"0905ebc39ebd0e0e10da5d10682b28b1a08fe53b0535c6f6ebedae19b29118e0",
		"7aef52add08429f5ae3756171b021063086f3ea6e31de78636c60ec66dccc189",
		"a1a039d3e0722ee2e2d7f94978168d4e9aa46ee4ecbdabfd97cd4e775ba1e244",
		"3094cb7a9d939dfac9eeb0cff232d614f933d88f31e8c531c829b13f94ad3fdb",
		"4df08133cf4f4268559e2a1fa1298c29dc3c8420702ea6c01a9ca84a9229c61b",
		"6a8b66eea8d072abb079cf2f19e27e7e6dcfa886903b45a6772401e0c583444b",
	},
}

// roundDigest hashes one round's canonical state plus its own event stream
// — canonicalState/canonicalEvents (above) already make this immune to
// Go's randomised map iteration, the same property TestGoldenFixtures...
// FiftyInProcessRuns relies on.
func roundDigest(t *testing.T, s MatchState, events []game.Event) string {
	t.Helper()
	sum := sha256.Sum256([]byte(canonicalState(t, s) + canonicalEvents(t, events)))
	return hex.EncodeToString(sum[:])
}

// TestGoldenFixturesPerRoundDigestMatchesCommittedFixture is issue #80's
// acceptance criterion made literal: "a divergence names the round it first
// appeared in." It replays each fixture from a fresh initial() — never the
// memoized recording's own final state — round by round, comparing each
// round's digest against the committed table and stopping at the FIRST
// mismatch: every later round is definitionally wrong too once state has
// diverged, so reporting only the first is signal, not information loss.
//
// Fails closed, per the issue's own acceptance bullet: a missing or short
// fixture, cfg.Rounds == 0, or a match that produced zero events end to end
// all fail the test rather than silently comparing nothing.
func TestGoldenFixturesPerRoundDigestMatchesCommittedFixture(t *testing.T) {
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			_, log, cfg := runDeterminismScript(t, fx.players)
			if cfg.Rounds == 0 {
				t.Fatal("cfg.Rounds == 0 — a digest computed over zero rounds is not a check")
			}
			want := goldenPerRoundDigests[fx.players]
			if len(want) != cfg.Rounds {
				t.Fatalf("%dp fixture has %d committed per-round digests, want %d (cfg.Rounds) — missing or short fixture", fx.players, len(want), cfg.Rounds)
			}

			s, err := initial(fx.seed, cfg, fx.players)
			if err != nil {
				t.Fatalf("initial() error = %v", err)
			}

			totalEvents := 0
			for round := 1; round <= cfg.Rounds; round++ {
				var events []game.Event
				s, events, err = Resolve(s, log[round], cfg, NewRNG(fx.seed, round))
				if err != nil {
					t.Fatalf("round %d: Resolve() error = %v", round, err)
				}
				totalEvents += len(events)

				got := roundDigest(t, s, events)
				if got != want[round-1] {
					t.Fatalf("round %d: digest = %s, want %s — first diverging round (of %d)", round, got, want[round-1], cfg.Rounds)
				}
			}
			if totalEvents == 0 {
				t.Fatalf("%dp fixture produced zero events across all %d rounds — an empty event stream must fail, not pass", fx.players, cfg.Rounds)
			}
		})
	}
}

// --- per-round RNG index-count accounting (RFC §16.2, the heart of #77) ---

// roundFacts is everything a per-round consumption predictor is allowed to
// read: the state entering Resolve (pre — Resolve clones before mutating
// anything, so this is genuinely untouched), the state Resolve returned
// (post), this round's own []game.Event stream, and cfg. A predictor must
// never re-derive the engine's own decision path (issue #77's own design
// constraint) — only read facts already decided by the time Resolve runs
// (deck order fixed at Setup, this round's entry snapshot) or already
// public in the output (events, state diffs).
type roundFacts struct {
	pre    MatchState
	post   MatchState
	cfg    game.Config
	events []game.Event
}

type consumptionPredictor func(f roundFacts) int

// eventCardForRound and incidentLiveForRound reuse the package's own
// non-consuming peeks (eventCardThisRound, trail.go; incidentCardThisRound,
// incidents.go) rather than re-deriving deck indexing — these are exactly
// the free re-reads buildGlobalEventContext/buildIncidentContext themselves
// already rely on, so using them here is reading an already-decided fact,
// not writing a second implementation of one.
func eventCardForRound(f roundFacts) (EventCardID, bool) {
	return eventCardThisRound(f.post.Round, f.pre.Graph.EventDeck)
}

func incidentLiveForRound(f roundFacts) (IncidentCardID, bool) {
	if f.pre.UnstableSector == nil {
		return 0, false
	}
	return incidentCardThisRound(f.post.Round, f.pre.Graph.IncidentDeck)
}

func predictEventDragnet(f roundFacts) int {
	card, live := eventCardForRound(f)
	if !live || card != EventDragnet {
		return 0
	}
	return min(2, len(borderNodeIDs(f.pre.Graph)))
}

func predictEventBridgeDown(f roundFacts) int {
	card, live := eventCardForRound(f)
	if !live || card != EventBridgeDown {
		return 0
	}
	return min(1, len(navigableEdges(f.pre.Graph)))
}

// predictOneDrawEventCard returns a predictor for any event card that, per
// the ConsumptionTable, consumes exactly one draw when it is this round's
// live card and zero otherwise — Festival, Scaffolding, Shipping Boom, and
// Fence's Windfall are identical in shape, differing only in which card they
// watch for.
func predictOneDrawEventCard(want EventCardID) consumptionPredictor {
	return func(f roundFacts) int {
		card, live := eventCardForRound(f)
		if !live || card != want {
			return 0
		}
		return 1
	}
}

// predictIncidentSector mirrors nextUnstableSector's own two guards
// (incidents.go): live at all (ctx.live), and a further round left to
// announce it for.
func predictIncidentSector(f roundFacts) int {
	if _, live := incidentLiveForRound(f); !live {
		return 0
	}
	if int(f.post.Round) >= f.cfg.Rounds {
		return 0
	}
	return 1
}

func predictIncidentSinkhole(f roundFacts) int {
	card, live := incidentLiveForRound(f)
	if !live || card != IncidentSinkhole {
		return 0
	}
	if len(nodesInSector(f.pre.Graph, *f.pre.UnstableSector)) == 0 {
		return 0
	}
	return 1
}

// predictCrateNode reads the round's own output events rather than peeking
// any card: PurposeCrateNode is shared between Dead Runner (a global event)
// and Spilled Load (a sector incident), and both announce themselves
// publicly with a dedicated event kind the moment they draw — the exact
// "observable facts... never re-deriving decision logic" shape issue #77's
// own design note suggests for this row by name.
func predictCrateNode(f roundFacts) int {
	n := 0
	for _, e := range f.events {
		if e.Kind == game.EventDeadRunnerCrate || e.Kind == game.EventSpilledLoadCrate {
			n++
		}
	}
	return n
}

// predictPressureD6 counts Legend-tier seats using the FINAL post-Resolve
// state, which is honest only because this suite's golden fixtures never
// let a contract miss its deadline: pressure() runs before upkeep()
// (resolve.go's own step order), and upkeep's contract-deadline step is the
// only thing in a round that can still move Infamy after pressure() has
// already rolled (upkeep.go's expireContract, Tier IV only —
// game/config.go's PenaltyInfamy is 0 for every other tier). Every fixture
// here only ever accepts Tier 0 offers and stops accepting new work with
// enough rounds left to deliver (runDeterminismScript, mirroring
// golden_test.go's own precedent), so this gap never actually opens for
// these fixtures — documented here rather than silently assumed.
func predictPressureD6(f roundFacts) int {
	if f.cfg.Suppress.InfamyTiers {
		return 0
	}
	if card, live := eventCardForRound(f); live && card == EventShiftChange {
		return 0
	}
	n := 0
	for _, p := range f.post.Players {
		if TierOf(p.Infamy) == game.TierLegend {
			n++
		}
	}
	return n
}

func predictMarketStock(f roundFacts) int {
	if !MarketRefreshDue(f.post.Round+1, f.cfg) {
		return 0
	}
	n := 0
	for _, node := range f.pre.Graph.Nodes {
		if node.Type == game.NodeBlackMarket {
			n++
		}
	}
	return 3 * n
}

// predictContractOfferTier reuses offerDue (contracts.go) directly against
// the FINAL post-Resolve state — exactly the state prepareNextRound itself
// consults, since it runs after every other step in Resolve including
// upkeep. "2 always drawn even when only one tier is eligible" (D6, the
// ConsumptionTable row's own Notes) is why this needs no further branch on
// how many tiers are eligible. contract.offer.pick is deliberately not
// predicted alongside this — see consumptionExemptions.
func predictContractOfferTier(f roundFacts) int {
	if int(f.post.Round) >= f.cfg.Rounds {
		return 0
	}
	n := 0
	for _, seat := range bySeat(f.post) {
		p := f.post.Players[seat]
		if offerDue(f.post, seat, f.cfg) && len(p.Contracts) < 2 {
			n++
		}
	}
	return 2 * n
}

// consumptionPredictors is every ConsumptionTable row this suite predicts
// honestly from observable round facts. See consumptionExemptions for the
// rest of the non-Setup-only rows, and
// TestConsumptionPredictorsAccountForEveryNonSetupRow for the completeness
// check that ties the two together against the real table.
var consumptionPredictors = map[Purpose]consumptionPredictor{
	PurposeEventDragnet:        predictEventDragnet,
	PurposeEventBridgeDown:     predictEventBridgeDown,
	PurposeEventFestival:       predictOneDrawEventCard(EventFestival),
	PurposeEventScaffolding:    predictOneDrawEventCard(EventScaffolding),
	PurposeEventShippingBoom:   predictOneDrawEventCard(EventShippingBoom),
	PurposeEventFencesWindfall: predictOneDrawEventCard(EventFencesWindfall),
	PurposeIncidentSector:      predictIncidentSector,
	PurposeIncidentSinkhole:    predictIncidentSinkhole,
	PurposeCrateNode:           predictCrateNode,
	PurposePressureD6:          predictPressureD6,
	PurposeMarketStock:         predictMarketStock,
	PurposeContractOfferTier:   predictContractOfferTier,
}

// consumptionExemptions names, for every non-Setup-only ConsumptionTable
// row this suite does NOT predict per round, the existing targeted test(s)
// that cover it instead — per issue #77's own instruction: "it's acceptable
// for the per-round table-driven assertion to skip it only if it already
// has dedicated targeted-scenario coverage elsewhere... Do not silently
// skip — either predict it or name its substitute coverage." Every one of
// these is entangled with the engine's own movement/collision/eligibility
// decision path deeply enough that predicting an exact per-round count
// honestly, from outside, in a general scripted match would mean
// re-deriving that decision path — exactly the "second independent oracle"
// this suite's own design is required to avoid.
var consumptionExemptions = map[Purpose]string{
	PurposeConfrontD6: "confront_test.go's TestConfrontationTotalSumsEveryTerm and " +
		"TestResolveConfrontationsMeleeLosersDrawPushbackInSeatOrder, plus " +
		"turfWarTotal's own reuse of the same purpose (incidents.go) — entangled " +
		"with movement/collision detection and Turf War; a per-round count would " +
		"mean re-deriving both.",
	PurposeConfrontTiebreak: "ordering_test.go's TestByFairnessCoinFiresOnlyAtFourthLevel " +
		"and TestByFairnessCoinOrdersATiedGroupOfAnySize — entangled with which " +
		"contended actions or confrontation groups actually tie this round.",
	PurposePushbackHop: "confront_test.go's TestPushbackStationaryNeutralConsumesOneHop, " +
		"TestPushbackStationaryEvasiveConsumesTwoHops, and this issue's own new " +
		"TestResolveConfrontationsMeleeTwoStationaryEvasiveLosersPushbackHopInSeatOrder " +
		"— entangled with collision/pushback resolution and how far each loser " +
		"already traversed this round.",
	PurposePushonEdge: "movement_test.go's TestPushOnStepStopsEarlyWithNoLegalEdge and " +
		"TestAdvanceInterleavesPushOnAndScavengeLazily — entangled with per-step " +
		"route legality (which edges are navigable from wherever a seat's blind " +
		"step actually lands).",
	PurposeScavengeD6: "movement_test.go's TestScavengeSkipsAlreadyKnownNode and " +
		"TestAdvanceSkipsScavengeWhenBlindStepLandsOnKnownNode — entangled with " +
		"per-step fog state at the exact node a blind step lands on.",
	PurposeIncidentRelocate: "incidents_test.go's TestResolveSnatchJobRelocatesAndConsumesRNG " +
		"— entangled with incidentEligible's own post-movement position/discard " +
		"filter, which this predictor has no honest way to derive without " +
		"re-deriving that filter.",
	PurposeIncidentRiot: "incidents_test.go's TestApplyRiotPermutation* family and " +
		"trail_test.go's sight-gated trail-entry tests — entangled with " +
		"writeTrail's own sight computation (D04's sight-gated filter).",
	PurposeContractOfferPick: "contracts_test.go's TestGenerateOfferRNGAccounting and the rest " +
		"of that file's GenerateOffer/cascade coverage — entangled with fog- and " +
		"graph-distance-dependent candidate pools (contractCandidates), and which " +
		"tier target slots 1 and 2 land on is itself an RNG draw this predictor " +
		"cannot see in advance, so an honest count is not derivable from outside " +
		"without re-deriving cascade.",
	PurposeItemTornMap: "items_test.go's TestRevealTornMapConsumesMinFourHidden — named " +
		"directly in issue #77's own truncation table. Never exercised by this " +
		"suite's golden fixtures, which never discard Torn Map, so a predictor " +
		"here would only ever assert zero.",
}

// setupOnly reports whether row's Phase marks it as never firing inside
// Resolve's round loop at all — every gen.* and deck.* row (rng_purpose.go)
// says "Setup only" in Phase, verbatim, and initial.go/resolve.go read
// end-to-end confirm those really are the only 14 rows initial() alone
// draws. Checked by substring against the real table data, not a hardcoded
// list of Purpose names, so a new Setup-only row is picked up automatically
// — mirroring TestPurposeTableMatchesDeclaredConstants's own data-driven
// shape (rng_purpose_test.go, issue #56).
func setupOnly(row PurposeRow) bool {
	return strings.Contains(row.Phase, "Setup only")
}

// TestConsumptionPredictorsAccountForEveryNonSetupRow is issue #77's own
// closing requirement: "a new consumer needs one or the other" — a
// predictor or a documented exemption — "so adding a 35th purpose without
// wiring it in here fails loudly," the same standard #56's own
// TestPurposeTableMatchesDeclaredConstants already holds the Purpose
// constant list to. It iterates the real ConsumptionTable, not a
// hand-maintained copy, so a new row is caught automatically.
func TestConsumptionPredictorsAccountForEveryNonSetupRow(t *testing.T) {
	checked := 0
	for _, row := range ConsumptionTable {
		if setupOnly(row) {
			continue
		}
		checked++

		_, predicted := consumptionPredictors[row.Purpose]
		_, exempt := consumptionExemptions[row.Purpose]
		switch {
		case !predicted && !exempt:
			t.Errorf("Purpose %q (Phase %q) has neither a per-round predictor nor a documented exemption in determinism_test.go — issue #77 requires one or the other for every row that can fire inside Resolve's round loop", row.Purpose, row.Phase)
		case predicted && exempt:
			t.Errorf("Purpose %q is listed both as predicted and as exempt — pick one", row.Purpose)
		}
	}
	if checked == 0 {
		t.Fatal("found zero non-Setup-only rows in ConsumptionTable — this would make the check above vacuous")
	}

	// The reverse direction: a predictor or exemption naming a Purpose that
	// ConsumptionTable no longer lists as a live, non-Setup-only row — the
	// row was deleted or reclassified Setup-only — would otherwise sit here
	// stale forever, since the forward loop above only ever walks the table
	// and would simply stop mentioning it.
	live := map[Purpose]bool{}
	for _, row := range ConsumptionTable {
		if !setupOnly(row) {
			live[row.Purpose] = true
		}
	}
	for purpose := range consumptionPredictors {
		if !live[purpose] {
			t.Errorf("predictor for Purpose %q is stale: the row is gone from ConsumptionTable or is now Setup-only", purpose)
		}
	}
	for purpose := range consumptionExemptions {
		if !live[purpose] {
			t.Errorf("exemption for Purpose %q is stale: the row is gone from ConsumptionTable or is now Setup-only", purpose)
		}
	}
}

// TestPerRoundRNGConsumptionMatchesPredictions is #77's own heart: RFC
// §16.2's invariant, "rng.seq consumed == predicted, per the §6.4 table,
// asserted per round," run across every round of every golden fixture.
//
// Fails closed per issue #81's own acceptance criterion: a fixture whose
// recorded consumption is empty must fail rather than trivially matching an
// empty prediction. The per-round `got != want` check above already catches
// the case where the tracker is broken (returns 0) while a real draw was
// predicted, in whichever round that first happens — but a fixture that
// happens to never exercise a single predicted purpose across all 15 rounds
// would sail through that check with every comparison at 0 == 0, having
// tested nothing. totalPredicted/totalActual, summed across every round and
// every predicted purpose, guards against exactly that: predicted == 0 means
// the fixture itself is too quiet to be a check; actual == 0 while predicted
// > 0 means every per-round comparison above should already have failed, so
// reaching this line means the summation disagrees with itself — either way,
// fail rather than pass on an empty summary.
func TestPerRoundRNGConsumptionMatchesPredictions(t *testing.T) {
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			_, log, cfg := runDeterminismScript(t, fx.players)

			s, err := initial(fx.seed, cfg, fx.players)
			if err != nil {
				t.Fatalf("initial() error = %v", err)
			}
			totalPredicted, totalActual := 0, 0
			for round := 1; round <= cfg.Rounds; round++ {
				pre := s
				rng := NewRNG(fx.seed, round)
				next, events, err := Resolve(s, log[round], cfg, rng)
				if err != nil {
					t.Fatalf("round %d: Resolve() error = %v", round, err)
				}

				f := roundFacts{pre: pre, post: next, cfg: cfg, events: events}
				for purpose, predict := range consumptionPredictors {
					want := predict(f)
					got := rng.Consumed(purpose)
					totalPredicted += want
					totalActual += got
					if got != want {
						t.Errorf("round %d: Consumed(%q) = %d, want %d (predicted from this round's own observable facts)", round, purpose, got, want)
					}
				}

				s = next
			}
			if totalPredicted == 0 {
				t.Fatalf("%dp fixture predicted zero total draws across every round and every predicted purpose — the script never exercises a single predicted consumer, which would make this test vacuous", fx.players)
			}
			if totalActual == 0 {
				t.Fatalf("%dp fixture recorded zero total draws across every round and every predicted purpose, despite predicting %d — failing closed rather than trivially matching an empty prediction", fx.players, totalPredicted)
			}
		})
	}
}
