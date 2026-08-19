package rules_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// This file is issue #201's own suite: determinism and RNG index accounting
// for bot-populated matches, extending #77/#81's hand-scripted-orders suite
// (internal/rules/determinism_test.go) to matches driven by bots.Decide. It
// lives in internal/rules's own external test package for the same reason
// bots_runner_golden_external_test.go and bots_operator_golden_external_test.go
// already do: it needs both rules.MatchState and internal/bots itself, and
// package rules_test compiles as neither internal/rules nor internal/bots.
//
// D32 (docs/decisions/D32-bot-rng-stream.md) is what makes this suite
// tractable at all: bot draws never touch Resolve's own *RNG (they draw from
// a distinct *rules.BotRNG, constructed fresh per (seat, round) and
// discarded after one Decide call), so nothing here needs a new prediction
// oracle for bot-authored orders. determinism_test.go's own predictor/
// exemption framework (consumptionPredictors, consumptionExemptions) stays
// exactly what it is — unexported, package rules, proven once, generally,
// against ConsumptionTable — and does not need reproducing here. What this
// suite checks instead, empirically, per fixture: that a refold (which
// never calls Decide, only Resolve, per RFC §8.2's Autopilot-orders-live-in-
// the-log derivation) reproduces the exact same per-round Consumed(purpose)
// count, for every row in rules.ConsumptionTable, that the live recording
// (which did call Decide) produced. That is RFC §16.2's "rng.seq consumed
// == predicted" invariant, restated as "refold's actual == live run's
// actual" — the same property D32 itself names as what makes the
// mid-match-handover fixture's refold assertion hold.

// idleHandOrder is a hand-driven seat's order in the mid-match-handover
// fixture: no bot involved, no BotRNG draw authoring it — the literal
// "a returning player took the seat back" case RFC §8.2 describes.
var idleHandOrder = game.Order{
	Action: game.ActionOrder{Kind: game.ActionNothing},
	Stance: game.StanceOrder{Stance: game.StanceNeutral},
}

// seatRole picks which tier decides seat's order for round, or the zero
// Tier (bots.Tier's own reserved-invalid value) to mean "hand-driven this
// round" — idleHandOrder, not a Decide call.
type seatRole func(round int, seat game.SeatID) bots.Tier

// allBotSeats is every seat, every round, played by the same tier — the
// three per-tier golden fixtures.
func allBotSeats(tier bots.Tier) seatRole {
	return func(int, game.SeatID) bots.Tier { return tier }
}

// handoverAfterRound is the harder case issue #201 names by itself: seat 0
// plays tier through round cutover, then reverts to hand-driven (idle) for
// every round after — Autopilot handing back to a returning player.
// Seats other than 0 stay bot-driven the whole match, so the fixture is a
// real, live 15-round match throughout, not a match that goes idle at the
// handover.
func handoverAfterRound(tier bots.Tier, cutover int) seatRole {
	return func(round int, seat game.SeatID) bots.Tier {
		if seat == 0 && round > cutover {
			return 0
		}
		return tier
	}
}

// orderLog is a full match's recorded orders, one map[SeatID]Order per
// round number — exactly the shape RFC §7.1's state = fold(Resolve,
// initial(seed, cfg), orderLog) folds over.
type orderLog map[int]map[game.SeatID]game.Order

// botRoundResult is one round's outcome, live or refolded: the state
// Resolve returned, that round's own event stream, and every
// ConsumptionTable purpose's Consumed() count on that round's own *RNG.
type botRoundResult struct {
	state    rules.MatchState
	events   []game.Event
	consumed map[rules.Purpose]int
}

// consumedForAllPurposes reads every row of rules.ConsumptionTable off rng
// — this is the suite's only "prediction": what the live run actually drew,
// which the refold must reproduce exactly (see this file's own doc
// comment).
func consumedForAllPurposes(rng *rules.RNG) map[rules.Purpose]int {
	m := make(map[rules.Purpose]int, len(rules.ConsumptionTable))
	for _, row := range rules.ConsumptionTable {
		m[row.Purpose] = rng.Consumed(row.Purpose)
	}
	return m
}

// recordBotDrivenMatch plays one real 15-round match — real
// rules.NewMatch/ProjectView/Resolve calls, real bots.Decide calls for
// every seat role names as bot-driven this round — and returns each
// round's outcome plus the order log it produced along the way.
func recordBotDrivenMatch(seed [32]byte, cfg game.Config, players int, role seatRole) (rounds []botRoundResult, log orderLog, err error) {
	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		return nil, nil, fmt.Errorf("rules.NewMatch: %w", err)
	}

	log = orderLog{}
	rounds = make([]botRoundResult, 0, cfg.Rounds)

	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := game.SeatID(0); int(seat) < players; seat++ {
			tier := role(round, seat)
			if tier == 0 {
				orders[seat] = idleHandOrder
				continue
			}
			v := rules.ProjectView(s, seat, cfg)
			orders[seat] = bots.For(tier).Decide(v, cfg, rules.NewBotRNG(seed, seat, round))
		}
		log[round] = orders

		rng := rules.NewRNG(seed, round)
		next, events, rerr := rules.Resolve(s, orders, cfg, rng)
		if rerr != nil {
			return nil, nil, fmt.Errorf("round %d: rules.Resolve: %w", round, rerr)
		}
		rounds = append(rounds, botRoundResult{state: next, events: events, consumed: consumedForAllPurposes(rng)})
		s = next
	}

	return rounds, log, nil
}

// refoldBotDrivenMatch is state = fold(Resolve, initial(seed, cfg),
// orderLog) (RFC §7.1), literally: it replays a previously-recorded log
// through Resolve alone — bots.Decide is never called, and this function's
// own signature has no seatRole parameter to call it with.
func refoldBotDrivenMatch(seed [32]byte, cfg game.Config, players int, log orderLog) (rounds []botRoundResult, err error) {
	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		return nil, fmt.Errorf("rules.NewMatch: %w", err)
	}

	rounds = make([]botRoundResult, 0, cfg.Rounds)
	for round := 1; round <= cfg.Rounds; round++ {
		rng := rules.NewRNG(seed, round)
		next, events, rerr := rules.Resolve(s, log[round], cfg, rng)
		if rerr != nil {
			return nil, fmt.Errorf("round %d: rules.Resolve: %w", round, rerr)
		}
		rounds = append(rounds, botRoundResult{state: next, events: events, consumed: consumedForAllPurposes(rng)})
		s = next
	}

	return rounds, nil
}

// canonicalJSON renders v as a stable byte string for equality comparison
// across independent runs — encoding/json sorts map keys on marshal
// regardless of the key's underlying type, which is what makes this immune
// to Go's deliberately randomised map iteration (RFC §6.3), the same
// technique determinism_test.go's own canonicalState/canonicalEvents use.
func canonicalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	return string(b)
}

// botRoundDigest hashes one round's canonical state, its own event stream,
// and its own full per-purpose consumption map — folding the RNG
// index-accounting property into the same per-round pin
// TestGoldenFixturesPerRoundDigestMatchesCommittedFixture
// (determinism_test.go, issue #80) already established, so a regression in
// any of the three is caught by round, not just by final state.
func botRoundDigest(t *testing.T, r botRoundResult) string {
	t.Helper()
	sum := sha256.Sum256([]byte(canonicalJSON(t, r.state) + canonicalJSON(t, r.events) + canonicalJSON(t, r.consumed)))
	return hex.EncodeToString(sum[:])
}

func seedByte(b byte) [32]byte {
	var s [32]byte
	s[0] = b
	return s
}

// botDeterminismFixture is one scenario: which seed, and which tier decides
// each seat's order each round.
type botDeterminismFixture struct {
	name string
	seed [32]byte
	role seatRole
}

const botFixturePlayers = 4

// botDeterminismFixtures is issue #201's own fixture list: one per tier
// (every seat that tier, the whole match), plus the harder case the issue
// names by itself — a seat that is bot-driven for rounds 1-7 and
// hand-driven for 8-15 (handoverAfterRound), the literal shape of Autopilot
// handing back to a returning player (RFC §8.2).
var botDeterminismFixtures = []botDeterminismFixture{
	{name: "drifter", seed: seedByte(0xb1), role: allBotSeats(bots.Drifter)},
	{name: "runner", seed: seedByte(0xb2), role: allBotSeats(bots.Runner)},
	{name: "operator", seed: seedByte(0xb3), role: allBotSeats(bots.Operator)},
	{name: "handover", seed: seedByte(0xb4), role: handoverAfterRound(bots.Runner, 7)},
}

// botFixtureRecording is recordBotDrivenMatch's memoized output for one
// fixture.
type botFixtureRecording struct {
	rounds []botRoundResult
	log    orderLog
	err    error
}

// botFixtureCache memoizes recordBotDrivenMatch once per fixture name
// (sync.OnceValue, so concurrent subtests share one recording instead of
// racing to produce their own) — the same shape determinism_test.go's own
// determinismScriptCache uses, for the same reason: the recording is an
// immutable, pure function of (seed, cfg, players, role), safe to share
// read-only across every test that needs it.
var botFixtureCache = func() map[string]func() botFixtureRecording {
	cache := make(map[string]func() botFixtureRecording, len(botDeterminismFixtures))
	for _, fx := range botDeterminismFixtures {
		cache[fx.name] = sync.OnceValue(func() botFixtureRecording {
			rounds, log, err := recordBotDrivenMatch(fx.seed, game.DefaultConfig(), botFixturePlayers, fx.role)
			return botFixtureRecording{rounds: rounds, log: log, err: err}
		})
	}
	return cache
}()

func runBotFixture(t *testing.T, name string) botFixtureRecording {
	t.Helper()
	get, ok := botFixtureCache[name]
	if !ok {
		t.Fatalf("no bot determinism fixture registered for %q", name)
	}
	rec := get()
	if rec.err != nil {
		t.Fatalf("recordBotDrivenMatch(%q): %v", name, rec.err)
	}
	return rec
}

// --- fails closed: round 15, a full order log, a non-empty event stream ---

// TestBotGoldenFixturesReachRoundFifteenWithFullOrderLog is issue #201's own
// fails-closed criterion: a harness that silently resolved zero rounds, or
// dropped seats from the log, would otherwise satisfy every other assertion
// in this file trivially (two empty streams compare equal, and an all-zero
// index table matches an unpopulated prediction — the issue's own words).
func TestBotGoldenFixturesReachRoundFifteenWithFullOrderLog(t *testing.T) {
	cfg := game.DefaultConfig()
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)

			if len(rec.rounds) != cfg.Rounds {
				t.Fatalf("recorded %d rounds, want %d", len(rec.rounds), cfg.Rounds)
			}
			if len(rec.log) != cfg.Rounds {
				t.Fatalf("order log has %d round entries, want %d", len(rec.log), cfg.Rounds)
			}

			total := 0
			for round, ro := range rec.log {
				if len(ro) != botFixturePlayers {
					t.Errorf("round %d: order log has %d seat entries, want %d", round, len(ro), botFixturePlayers)
				}
				total += len(ro)
			}
			want := cfg.Rounds * botFixturePlayers
			if total != want {
				t.Fatalf("order log has %d total entries, want %d (%d rounds x %d players)", total, want, cfg.Rounds, botFixturePlayers)
			}

			final := rec.rounds[len(rec.rounds)-1]
			if final.state.Round != game.RoundNumber(cfg.Rounds) {
				t.Fatalf("final Round = %d, want %d", final.state.Round, cfg.Rounds)
			}

			totalEvents := 0
			for _, r := range rec.rounds {
				totalEvents += len(r.events)
			}
			if totalEvents == 0 {
				t.Fatal("event stream is empty across all 15 rounds — the match ran silent, which is not the same as a match that ran")
			}
		})
	}
}

// --- the refold property: byte-identical, without ever calling Decide ---

// TestBotGoldenFixturesRefoldReproducesRecordedStateWithoutCallingDecide is
// issue #201's own point (its acceptance criteria call this out by itself):
// refoldBotDrivenMatch's signature has no seatRole and never calls Decide —
// it replays the recorded order log through Resolve alone. If this test
// passes, "a refold reads bot orders from the log; it does not re-run the
// bot" (RFC §8.2's Autopilot derivation) holds for real, not just by
// construction.
func TestBotGoldenFixturesRefoldReproducesRecordedStateWithoutCallingDecide(t *testing.T) {
	cfg := game.DefaultConfig()
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)

			refolded, err := refoldBotDrivenMatch(fx.seed, cfg, botFixturePlayers, rec.log)
			if err != nil {
				t.Fatalf("refoldBotDrivenMatch: %v", err)
			}
			if len(refolded) != len(rec.rounds) {
				t.Fatalf("refold produced %d rounds, want %d", len(refolded), len(rec.rounds))
			}

			for i := range rec.rounds {
				round := i + 1
				if got, want := canonicalJSON(t, refolded[i].state), canonicalJSON(t, rec.rounds[i].state); got != want {
					t.Fatalf("round %d: refold state diverged from the recorded live run — refoldBotDrivenMatch never calls Decide, so this state comes only from {seed, config, order log}", round)
				}
				if got, want := canonicalJSON(t, refolded[i].events), canonicalJSON(t, rec.rounds[i].events); got != want {
					t.Fatalf("round %d: refold event stream diverged from the recorded live run", round)
				}
			}
		})
	}
}

// --- the RNG index-accounting property (RFC §16.2, D32) ---

// TestBotGoldenFixturesPerRoundRNGConsumptionMatchesRefold is this file's
// own restatement of #77/#81's standard for bot-populated matches: for
// every round and every rules.ConsumptionTable row, the refold's
// Consumed(purpose) — read off a *rules.RNG that never saw a single
// bots.Decide call — matches the live run's own. D32's own text names this
// exact property as what its mid-match-handover fixture needs, and this is
// what proves it holds, not merely follows from the type signature.
//
// Fails closed the same way #81 was held to: a fixture whose total
// consumption across every round and every purpose is zero passes every
// per-round comparison trivially (0 == 0) without ever exercising the
// property under test.
func TestBotGoldenFixturesPerRoundRNGConsumptionMatchesRefold(t *testing.T) {
	cfg := game.DefaultConfig()
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)
			refolded, err := refoldBotDrivenMatch(fx.seed, cfg, botFixturePlayers, rec.log)
			if err != nil {
				t.Fatalf("refoldBotDrivenMatch: %v", err)
			}

			totalConsumed := 0
			for i := range rec.rounds {
				for _, row := range rules.ConsumptionTable {
					want := rec.rounds[i].consumed[row.Purpose]
					got := refolded[i].consumed[row.Purpose]
					totalConsumed += want
					if got != want {
						t.Errorf("round %d: refold Consumed(%q) = %d, want %d (from the live recording) — RFC §16.2's rng.seq consumed == predicted invariant must hold regardless of bot population (D32)", i+1, row.Purpose, got, want)
					}
				}
			}
			if totalConsumed == 0 {
				t.Fatalf("%s fixture consumed zero RNG draws across every round and every ConsumptionTable purpose — too quiet to be a check", fx.name)
			}
		})
	}
}

// --- the live path is itself reproducible from a seed alone ---

// TestBotGoldenFixturesLiveRunIsReproducibleFromSeed is issue #201's own
// acceptance bullet: "re-running the same seed through the live path
// (calling Decide) produces the same order log as the recorded one." This
// is the property cmd/simulate's sweeps lean on (D32's own Consequences
// section) — a second, fully independent recording, calling Decide again
// at every seat and every round, must produce byte-identical orders.
func TestBotGoldenFixturesLiveRunIsReproducibleFromSeed(t *testing.T) {
	cfg := game.DefaultConfig()
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)

			rounds2, log2, err := recordBotDrivenMatch(fx.seed, cfg, botFixturePlayers, fx.role)
			if err != nil {
				t.Fatalf("second recordBotDrivenMatch: %v", err)
			}
			if len(rounds2) != len(rec.rounds) {
				t.Fatalf("second live run produced %d rounds, want %d", len(rounds2), len(rec.rounds))
			}

			if got, want := canonicalJSON(t, log2), canonicalJSON(t, rec.log); got != want {
				t.Fatal("re-running the same seed through the live path (calling Decide) produced a different order log than the recorded one")
			}
		})
	}
}

// --- golden pins: regenerating either table without a stated PR reason is ---
// --- the exact failure mode determinism_test.go's own goldenHashes/       ---
// --- goldenPerRoundDigests comments already warn against — same standard  ---
// --- here.                                                                ---

// botGoldenFinalHashes pins each fixture's final canonical state.
var botGoldenFinalHashes = map[string]string{
	"drifter":  "ea7b0ed283337f1c8be5dd9f7ab5ae90829b87d9eb459c82d540cbf5417fd874",
	"runner":   "3faca4e857cbed02080e0d03ada0e6d8394aea994be0c4b1159b8dcccffb98ac",
	"operator": "4a69cbf6b95b2e67df6823b2dc1a0e8ec8b757cbfcf1a9aad2657062b59eb821",
	"handover": "769aadc4a1924553ae34a3126b70831e5f954eca09529a4255abc0483d671737",
}

// TestBotGoldenFixturesFinalStateMatchesCommittedHash is the "fixtures
// exist, one per tier plus the handover case, and reach final scoring"
// acceptance criterion, plus the golden-fixture pin itself.
func TestBotGoldenFixturesFinalStateMatchesCommittedHash(t *testing.T) {
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)
			final := rec.rounds[len(rec.rounds)-1].state

			sum := sha256.Sum256([]byte(canonicalJSON(t, final)))
			got := hex.EncodeToString(sum[:])
			want, ok := botGoldenFinalHashes[fx.name]
			if !ok {
				t.Fatalf("no committed final-state hash for fixture %q", fx.name)
			}
			if got != want {
				t.Errorf("%s fixture's final-state hash = %s, want %s — either a genuine, deliberate change (regenerate and say why in the PR) or an unintended regression", fx.name, got, want)
			}
		})
	}
}

// botGoldenPerRoundDigests pins, per fixture, one digest per round — each
// entry hashes that round's state, its own event stream, AND its own full
// per-purpose consumption map (botRoundDigest), so a regression that only
// ever moves the RNG accounting without moving the visible state is still
// caught, by round.
var botGoldenPerRoundDigests = map[string][]string{
	"drifter": {
		"6cd42cdd961d147cd8d43cf3d82c1063fb0e82617014192550d38c094e51d604",
		"b89b64ad931eabf4fb37ea08485c692378ec7f0f55804b8a2b00a37d9c56136c",
		"a8288e076325a4bb8b851546470b5d648f45f29aca62f2640ee2e64d18e96788",
		"e6d46ed73a5c18d9d1eed590c15e754be165c32a4cfae0d1526258e8fe2c5355",
		"8f8d82c4e3a4356fbae25d74fbebc1fadaf7d07556a30a4a15e1cbfa5172f100",
		"e91b372670ed6ba9ed11bfc09b172c43d97599b91b45f961cbbfdfcb06fa4a72",
		"6ba9b70ddcd1cbd72f22a225936ed79efc7ef6970d3108443eb29553830e7b93",
		"68e901fd092c74eca04db883a8b5e85bd90c7b1c50b5f98f445c9ed1dbb94f59",
		"108c59f9e998c62d1a1c9797b228983af2feffdee067cff6a3d7062e12cdd923",
		"387a7460336483eb7ac4257686d709d84ab0475cccb2e4f34987988e710a3800",
		"76a825edb350fbb2277a0e80e072068ff82016dc86166c2e876210878fc4e077",
		"f7dcd0f284feb95f57c203e75ca5c16b4643920d77feba34e39337a4f0285e0c",
		"59066cda4a9255e8c4b3e6f0e442e9fc38117a83121ba3264070e5a7ad34cb23",
		"f56e8c2a30a4cbccbb9d60b3c2c838647071014a613e697bb67f30980e2c062b",
		"87c890bb9f61c612c58d5767d6e36e8a5c330807efb070c155d31b258d1bb5da",
	},
	"runner": {
		"7593ceca845b04209ba4ce8fdc7ed6ec98842fad6cc82d57b10951196d19aaf4",
		"6698c84de0e7f4055c86cd697ea3c6f40e722cc14d3f7a801e4fce0de809b6ba",
		"60d4a5cd1b6c8cbdcc08a834a919a13620e2597a6dd99d8dd6a17d89d17cfc41",
		"5e984099f2d6f7733fc82ff2a757fd1d6690a7e07193ff370eeef233c88abe98",
		"ae9f29c45b2fe9da40e7cec0f9a4824923dc2ce294efa397e6e61a43689104db",
		"c509a4536e828aba90458d4c7b60ce388abefa92495f7f3c133b2ee2ca2edb78",
		"4cf4a0e47803a2238d92f48c359c98c5013b959aff288766eaf70e79987deb04",
		"72124f4e83438da9968a5a520e2169c4411e7f9971c1f0a5329de98645cf9b54",
		"02c6d6958efb9bcd9ba98615f8c3594f8cf615c20afb71a622749213741fcf1d",
		"a2d25e77a051f4aae86a18b1b54b18d5a62610e5ddf9164c2c957dcc0c564e40",
		"1e7b80042d7a6a71d524377911822471846e219826402846193a59d7d7b08305",
		"83ad595c79ed2f68ebdcbe5746c9262f46a8da8cbc55c7f06c5a46b614f0f705",
		"3abd229d25514543ebe29b1f9289d909b2834f6d77c2863dfff0308a702e24dc",
		"33eca984fc81d9bfeb8ad99fe4f7b3d0e29d47c2bd6738ffff53143e67482219",
		"ed33674b0fcc5d6601264439949c8e7c6b71d96e4fb8c2fdaf93bb2ed42364b0",
	},
	"operator": {
		"501893efba90ead63306589c0266b6ff812cf6a994f078853817a6f4d1b64541",
		"aa63b8fbfa144a369de10477e59c29798920efcaf453df8a1e589a955730bf4d",
		"1891146c85dd6f1cf0203d8c550aa74fbcfc792c1142a119d5d10a1ba982bc42",
		"c45c761200295dcf4117ee32dc22a1bb354296603b282e3518628ce7e59ff65c",
		"ab0862b9ff3655b9ddb925d163f566b00e21bc9896a7d68f7965ce63b2d978ca",
		"3805136c818e29723a6ab09c0b87d6cb3a289ea709b09043ff7c6ed9d86c38b5",
		"c23dccbb88baf9dab79389a0f74ef3a30628b5222150acd08454168e5af72cbf",
		"9915721b3755a8181322be4dc3cc03531c8abad58bcb9a3fdff8b35e0e05f196",
		"ccef897d8e6f56ef85b98b491fddada05920c0bb9438d9cf24d35f0395873dee",
		"5360c2ba9184f2f6c690d2cea62f3eecca230a3d056253b98bb1a81988f9db1b",
		"8f85b97e2b4106447752a3a2f94c9c5f3dc43233f32ea55e451c36af7d6c8918",
		"34b1f74f971fc997a92bbe27c502cb4bf5a4851f6e612e6ace7ed5c0aca804b3",
		"6548addfab160666775573b4be31daec7b70960652e31d4e68f3357569f2fa3f",
		"867bf7dbfa7985c1a7e1ae131296e57909c17b433fd5a0cca66b43af21c39675",
		"070bcf4530fe0f099a7d10ff38cc63ca07f8297783cf93de06ebf49a369db30b",
	},
	"handover": {
		"ab70bc071d19260a8658b1642c8cf1ee48643773926af2376156822f584ff742",
		"9dd80dc700062635642a42d6d6557bf28d0ac3a1d204aa7f2bf7e7211ad1e0f4",
		"bb4678d2513e06ed882b85ad1c6fbc05332c807066e28ca42a5f7f04be7be9b5",
		"b66053e3908d0669aba454d634345d9cf11be38be66fc03928b2c22514410a4e",
		"779bc50d7ccad7c1759798f3e90360eba3d1ea78ffd4494f56d248d101d6bbad",
		"e08bd911326e0424e48801f9defc7abf81dc4a8381996396a2135c2aab0756cd",
		"41a538ab873bd8b87250f256d1d82d46737065434c2aadac7d071cb92f88b9dd",
		"db0d49ee96aba0c3a5bc5e68a97c0bc387a703a546e54ee471687f33adf13d91",
		"fbec3d33b83c03cb431aa3128edb3055ccf05ce5993f90f95a6302f64de1fb20",
		"f2e6e6a058e190223e2be7c5618877b14d8690c06acfe9fdb3fc24adb77becd0",
		"868bde3966c9b077d8b60077fba08a40c8d4c2c9d1f7a1b643c648b9dfcb3ebb",
		"3d7be8547f8c62d2c71d18a7994eae2bab86ae47fef426ad78361f1988a04664",
		"628d98a059664a3966d8c5738f09530836fe5309b5f8853a8a6bdf0b856092bf",
		"7ca0bf54040db5a7351edfe99c216e31ff536f7ba7f8e547033458a7e11feec6",
		"66ca0cc3a428a446532f45a648eb4477bd51313ab37a573d2454ad78c9713c47",
	},
}

// TestBotGoldenFixturesPerRoundDigestMatchesCommittedFixture is issue #80's
// own standard, restated for bot-populated matches: a divergence names the
// round it first appeared in, rather than only "somewhere in 15 rounds."
// This is also what CI's replay job (.github/workflows/ci.yml, `make
// replay`) reproduces on a second OS and architecture — it runs the whole
// internal/rules package, so this file's own tests ride the same matrix
// issue #80 already set up, with no separate wiring needed.
func TestBotGoldenFixturesPerRoundDigestMatchesCommittedFixture(t *testing.T) {
	cfg := game.DefaultConfig()
	for _, fx := range botDeterminismFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rec := runBotFixture(t, fx.name)
			want, ok := botGoldenPerRoundDigests[fx.name]
			if !ok {
				t.Fatalf("no committed per-round digests for fixture %q", fx.name)
			}
			if len(want) != cfg.Rounds {
				t.Fatalf("%s fixture has %d committed per-round digests, want %d", fx.name, len(want), cfg.Rounds)
			}

			for i, r := range rec.rounds {
				got := botRoundDigest(t, r)
				if got != want[i] {
					t.Fatalf("round %d: digest = %s, want %s — first diverging round (of %d)", i+1, got, want[i], cfg.Rounds)
				}
			}
		})
	}
}
