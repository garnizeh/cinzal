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
		"076bdc17802da07ad6af5014e6c10a52f69b2b68acaa156301fa92bd59169eea",
		"5fbf1d3436d221451d75829c71adda256553f8de60352ce19b7dc17f96bc698b",
		"4d0f28758bd4f473798781adca78fd1f51e4a474cee2382fbc74bbdc6a462b62",
		"17a84cc5937a9e91a0825e070e867cb98b8a4de25033e133829b213f84de390e",
		"ad08258addb45503267b9112c81577bfc0b80a9c7e09040fbf3e9ed251e7a6f9",
		"1199611edf22ce734e21265a886399e60b2aa27ffa56130ae8ec91a0d9822ee3",
		"6c84ed7f0e54d71f819636af39fdc6e88aac9143ff423bfd6634f303673f0743",
		"0d7e667b9ef1119a1c15c8602e22fde851dcc0e8a6e0f95e74e284037f901127",
		"5caa6871dd3ad1ca6861a35b937771a9ad3a210452b3a144d767a6994cebd34b",
		"d589f7ad86c207e2830a8a5feb3507a59f74bb957394bcb06c4f2658711d3dd3",
		"919bcd47bf853d88835225d1c19afc1709abecc7c4da0003be01e18f8b311175",
		"aaf4b9fb04afd001a61f393be7473c1c0217024c185eaaf7d91bb26e12832ce4",
		"985050a48e3e6bd4e70cb487f0c9643ae3777598f7a306ae75b8979ccdf03138",
		"1c7c402a9b564b9da1371a683d0c7ef4101783bf00e619af0d45508e9abbd83c",
	},
	3: {
		"55e2ee7a74126de6020761708d5c10e0c3b3fcb6de0f3f428a4cc46259356b99",
		"ef17a39ded35b7bed5496631a247c8c0239d2717e9d221586ddb7b759f19330e",
		"045b9a48a04d4d3befa68bb1ebdf554aaffae825ba21f9441f064fb1592dbd69",
		"5512d4f4018bdb09b5799c9bdf31c9cbb437ac826713850c2d47c606074325b7",
		"36605b88be7b5b666fa4a80a0e4e6fb76bf1b14f3e5df86d3b0f7a870dfbd25f",
		"6827a1a3b55c96f25ab927547f418854ce934a7b385f356f827f6870b3bb12e3",
		"04fb2c6c8bec8f6174b5c1b8030e0dacd275f47658fd4ca079d97fd2270d1209",
		"7618dbec581c5cf232f10cd8b3ea08881ceb9ba5cbd70d1713dd8efbecaee206",
		"359bcc8b923ccb077aa2529ea4be250baab4fcf119cfc6296f14ac65cd4f418d",
		"4657bc936b4eab5e130dd49705f4738c77802f0c0064e2a38dc9b72457d1cf8e",
		"0ea51d22772f91b84f3c307f9f551edddcb60485f920d7b2c98925c4864daf80",
		"6e61bd97f794cf4180b800133cc3f47f1ead09c1ce50bb0d11e1f8f6fdec3ab2",
		"aac66bc9af3fd61b6b1222271d7b2727b0fc69f0c3ac285f883937ed2b59564a",
		"50d8afd94ed650e4146c074a750df947579dad4170fa89a5e67310b88a97f4e7",
		"1f2424a6cd25b028476ac850bc3e87f1c6715e54a5d5bdd13b5788510ed3b201",
	},
	4: {
		"5a9d954d01ff01e37f0f863666db23710e34e873d78cf9191da15a2d69c83b3c",
		"1f69ccaaeb2d97383ac2f2255de60189a1ba0532be8aaf63f07427f61fa5ae2d",
		"b74cf0055033fb0c0c5629d39d72b28060bb70f419b08882faefba8c52b8e957",
		"637d4bab5524df4f2ef637db4fb4e33a4d31f0de7a4ca553c0e878847ede669e",
		"b9433ad266b2da01bcba38dccbc965ade3d89b3cfa8d6fa8bc8c2b10f2890daa",
		"4fafe94a62d657da277a72fce7e6b11478f90086795b92c3daf7aa58cab4c720",
		"b0367724b17f570f5ed8acb7c2d8dc74d56eed2d8a269406990683c783d712f0",
		"3479b07ab8ccb0ec49e66a67ec6eb620790dc7f95545d1a0edd934d090e8f63b",
		"bea6c0a7cb0f07f7bcd55c503df910ff7bde7d8e8c0e2d50f14217f6fcd1a203",
		"1ac69d2d9e4bcb899b278a47ee880339f7acf22616308293d6c244f7c0fff358",
		"a22c44a45e6d143f9a40493a145e4d0e89432aa9cf8d0e665557595c37495bc4",
		"4ae981b8ffb1c25d4f2d6a8f8ec85675abc4a3a27940e2b4f95b164c952bacf0",
		"a4c60f9b7ade13faed8a2715f5f07790d3132e53a8ca41f4d425b1b81ae57079",
		"c2c98c7aa72c0c535035e9963f214613e83df50391a669645e7fe363f650967e",
		"10f6e36bb68240f1200405ec10015864055067eeba79902b8ca81dcaf03a0f9e",
	},
	5: {
		"80d0afd4a6a56eb7cecf1d60ca8150a33553b02fc299b27ba1b0fed13cdcf403",
		"23053ad8cdb39492441674d099403258d27a237a92e64461527eb4e8cffd03e5",
		"9a9e0856fdca4e1828f48be5bd14b7ff00b366c5b26bf350cd3a1348a33d9c06",
		"c5b380764f078117f8e1df6be9821826fde94c50f9687831596154556b28a4b7",
		"d2c9d7648a15e6b6e14802e0c189d341afb3f1b1f95af0d6ed154d8091e1c84a",
		"ed96a4fbbad5296971e2d9e28e6e61fc4d1f6ce8d753907ee86b2a2c44c271b8",
		"9a8e4518de2019c084cf8bfd8aed1c719d56993ca5f6d2917824e20f6dd1c99a",
		"87ef9f8c5cf5c1e19d26a379ca08b38591ce85a7073d90295897cc356cdadc9a",
		"cdd4f5e1480d5af07ce62333e075a4b55633f248c03539616d68b919f62e7d2e",
		"a22f7d0788fecad20c4f4f0b5d564668b31944e74da7afe16beda91df4f88bbd",
		"b73c72de541419ff185fbc2d6d054c5b81a7780bf3c3884830b28497943c8e3d",
		"87bad6ef105d9bcc9ebe73d5f888a38bccaba862d359bd8ce5542bc144e4cc90",
		"ac5bb16b12e12842bb4ea59e5b0d92a28c87eec2e019720a5b92f3f3803e9b25",
		"f83ce874383cf7194b1df0a08dc475831ef2b9a9559ebf3db8700b80050467db",
		"96a5555b8d9083bd5ac8b2fca98ee461b5b0ecea3416ed15263d0a627cde93ce",
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
func TestPerRoundRNGConsumptionMatchesPredictions(t *testing.T) {
	for _, fx := range determinismFixtures {
		t.Run(fmt.Sprintf("%dp", fx.players), func(t *testing.T) {
			_, log, cfg := runDeterminismScript(t, fx.players)

			s, err := initial(fx.seed, cfg, fx.players)
			if err != nil {
				t.Fatalf("initial() error = %v", err)
			}
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
					if got != want {
						t.Errorf("round %d: Consumed(%q) = %d, want %d (predicted from this round's own observable facts)", round, purpose, got, want)
					}
				}

				s = next
			}
		})
	}
}
