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
	4: "a6378ef0149df86d4e0c09f2bb6a243c9b2e3500836ea42b675346c8536f7589",
	5: "b5df23f552a201403adaebcbea1a3f539cebe7c8fd48218b0121351763a72eae",
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
		"045fee8263200bface8f7b78d28ae16c94f2bd092260f5851e4800de40155b95",
		"a304c2dea5129f37f2eb02a71efb4e71ae8a0e0d330333a4da725d869aa098f9",
		"32e8d749276505978af96374ccb21565ce12c51cf51a9bad17bed2447729ed1e",
		"458e2cbe1dc9dc631e5b3b83836671efe28c420df158df2bd2f0f069a8569b9d",
		"f120ca41d0be429249cf5e28dcd5ee7cb15064e0c1d91cb0ada1991755edecd5",
		"6bb095a87c528599a1d71ac47fa34f77e801c9b0941bb95ea5a7f120f619871a",
		"6a03faf067cb640ced4a2dd9d4c270404b92372cc697b3b9ddfac720f67936a1",
		"6b8080b7dba1050e57a2005db3180d8e6ce831d67de1fe4234e5f430bf10d57c",
		"9f41f3ef97b0580ba58a7064d0ed5db7ab403a9be2d09cf8e26f05cb07a810cb",
		"45fd26801ffcb696112bdab2bae4476eb2f406ef1f0698a884b6be3affd37cd7",
		"60b684cf1cbd18ffe94cde561471eee61e349f133aae2601e0b73daa2d1739ee",
		"d757ef8fe3f4460e86e2a0886381e242be7bc9db58648acaf297280dd6a93865",
		"f7ca543fd32fe95253fd1ec7159291bedb5ba8c44e97e23d218464ccd52a1b5e",
		"1c7c402a9b564b9da1371a683d0c7ef4101783bf00e619af0d45508e9abbd83c",
	},
	3: {
		"55e2ee7a74126de6020761708d5c10e0c3b3fcb6de0f3f428a4cc46259356b99",
		"f23dc13c565a36ce618d7077a5e5119b186ff6b6a93925b98d25d041840d3d1e",
		"0ec36b4d70f6af7861770a35e03a45ac3a8cb10a0fcc34679569ce0602eba5df",
		"4f86b288894f7412709c134dec0140d87ef7ba8db908a706ac7519b2624b4a4c",
		"42029ef28cf02051a52ca79faffdf1675abd0f4d5b325d94208777f90cc5c2a1",
		"833fad2645da927e464a76fee03cd943234d6fe2f9bfaecbb581360b45b355fb",
		"be43f080721809d1906735a5947435657723ade8b6792681d61ec5c43a65761f",
		"1721750b893647d37141120b4b6f84b08554cca53f2fbb3bb13f6f12cd7e824c",
		"351ef280eba4bada20758e6b38c016d51fdb90efd7d7edb1c2a9306ae2c01811",
		"58cc446cabc6ec3837862a68b097db403a71172e15f6ec8592dac230f857cc88",
		"9bf5f757ade390f56e338b61d3597fb59aee7797d8aa5bb69a5a418c5282deb6",
		"bef2ffb48e360fc130c0e391f6db93837d13bf69232e94af3088bb005c18beb1",
		"04c5a87f5ecc9a0752930c5fc521a5520cd9639d26df784ea0d59a3fab3d62ab",
		"c088d40cd4b0c2cf0ec792f9a091ffea27f99a06f3c827196a51caf97d291c28",
		"112566a24c92958a3f2e2452f514a5eefa2af81f89b8317dda54fdb2aef1c15e",
	},
	4: {
		"add5e33e72e61d482ab708000d6e16a026eb76f10babde5d136968ce91ea358b",
		"6e06a6cea19f53a4c143465ecce3ccaae33ccb1219bb5973482576b54155576f",
		"05f4853edbc824fc392b1f44decc238a3615452f3e2713d695f190029131ff32",
		"01abfbf06e75abf94bafe61b96149591ef53bc4b8cf4a6b96e37ff793bd9d1a6",
		"9f47e146dbef6e04ec02114aba2311fd538ad1d084da77e5c0ba94edb6dafe39",
		"d5bd8b41b8f67b27ce719e3b9084af66f13c4ea847d4e376a122deb1b1a95e86",
		"2038c109734c2dd65f23abd829ad7de3a0825b0b8a817a34989b1fc05415eb8b",
		"d9385f5b212572e132e6905f893073ca02bb9f66c076a98e82e63eda9ce3d8d9",
		"575b6911a84a8340d0a8a5162cd416fd31f06250ea844ee8c6cb0faef447fff3",
		"7b0b68f34642853033031eaab7dc8847befefe36d0efae5982dafdde59280cf3",
		"6e2c2af3ececd5e6d3dbc783a5f3d9316c14981ead0c16f5e121c4bdce7ccdb5",
		"0895b2164395932e83450a6367e91f45405982386db4bf09cd5662d1b9653780",
		"9a8a056ed7f78acedcdb9183a4f887c1287e6e67b44693ea6063909abb0232be",
		"261fab68a62bec7d4a457446e5464f7aad0fdd5a3d506af5fb8231f236b301a6",
		"023395bd905dfa843e60828ad4f8ab2b9f21fe6f0154f4bfc1b6a95b180d0657",
	},
	5: {
		"80d0afd4a6a56eb7cecf1d60ca8150a33553b02fc299b27ba1b0fed13cdcf403",
		"1959d64d271d95b45bf8e4ca7b3c243741bbb8f0610d85cbac31a5c1e05c9566",
		"4aef8f26e92502cdd13d8c69f808027624fa7ba4857811547bb1970042afa1ac",
		"b762210b996b22282e89eecce8d9c11f96381ba49eb590b316d4ab726f76581f",
		"c910052a153a88671a8574c28d4834dddc4ddaefbb3da4b26697cf9ee2e7f33d",
		"f6ace5ceeca85370278a7bbd39534f7f2807738cf25ed02765e9ad3e7c2b73dd",
		"58069e847a3f7281fa873656966ef0e549c358124e7c53921fac16017928d766",
		"ada55a39d62301f033ea8821e401474f4e390abca7d220234249ca6498e02271",
		"b663c98daaf2db12acf8a7c971db6dbe1b895b62d12eb3f7537493309306834a",
		"b3ebd23955fc5209e227b495753c61aca4d07170473119f9e3a803740dcb3e0f",
		"87afac523f50bed388a6a6fe5779cec8228ec28de6c4844ece56497b34f86723",
		"ff12ec71e17316358e26ed7d2e74db248bc34cf425177bae2e8b05c6900c6d3a",
		"7cb2b886ae048161e2639d7e94cb50c53142010003c8b4aea075adc5af6af6c3",
		"b68af269a499f433a2c6066bcae1322b4f909d51b5685f1190ff2c83bd810f43",
		"408e94fb51126663aa6d0cdace90c8d30f5dd9c549a115a35c3bb925e35b141d",
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
