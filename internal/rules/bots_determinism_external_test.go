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

// idleHandOrderForRound is a hand-driven seat's order in the mid-match-
// handover fixture: no bot involved, no BotRNG draw authoring it — the
// literal "a returning player took the seat back" case RFC §8.2 describes.
// Round is set to the round it is submitted for, matching every bot tier's
// own Decide (drifter.go/runner.go/operator.go all set Round: v.Round on
// the order they return) — RFC §11.1a: "Round is the round this form was
// rendered for."
func idleHandOrderForRound(round int) game.Order {
	return game.Order{
		Round:  game.RoundNumber(round),
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
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
				orders[seat] = idleHandOrderForRound(round)
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
	"drifter":  "e92a6413edfb0529eff9a9e477fad41051e2a8cd4431521af90ab8dc5f043fa8",
	"runner":   "004b9a0d9584651253ffde2f6fa09391f1843cde1844c4b2194732928cd1ec7f",
	"operator": "d05280d8fafb65d4096cd22057734ed1b8e087c3ac5ad357ede85a7fde5ffe33",
	"handover": "91147b295fb3967bbc98bf05d8517b33a30cab7f8263c6b87e852ef353e66f07",
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
		"68b680ca6b74f03d505fca7c7a15d131742b9c7da440b7c44c71bcfcfff791dd",
		"cbac94dce289425dec85cbd5cffbb494a41fb45b841bd6348dfeced0e3724b44",
		"0b1e4bb34643371261cf9a515c91da4a654ebaab19030d3c02685627705b712d",
		"54b656842453dd18116c61d63ca7cfd7ab4115ee6c2ddae91ed43bb07bc04f7d",
		"b3fb5765e3d156e78ad7d6bf75e281f173c825da07f1b59990b7925a31602e7b",
		"c0a1bed9c79353c8c4be36e565a232d61242a4aac36d2151f568724be6afb5ac",
		"fbb076f42fca5c96de077e24e6e78e868d712204046269485845cae0a1a01d90",
		"ff9825c3e70e3bfd6626ed1767c61f490bff4c1791f564153e28483aa3f602b9",
		"3ee64b72790656ebb0bae22b6b5f42978e6b6ee5ece5754475da897afcaea9d7",
		"57089952617d49997a1404f936dbad0df2526dbee822fdd0aff10b13a8cc19c7",
		"15e2df66486f8c2257e2d6baae21299ba4ce265ac206555c57c385f8dedac0cb",
		"cb2dd627dd6901cecd95034be8a0640a8bfb09377e53bbca124808ceefaad90e",
		"4b13c88b7d673d3c6cd624fb4a9adfb73f96086c073289ccfab63abee2e7301a",
		"6a694812f82d3524e06fe0c9f8773973ba21b26a4f7b0d77b3d0d97b07d362ce",
		"9eb747c39814de9b2f72c8b9eaebfb527d01aa15ad7fc7f99732c285e9d6d3f3",
	},
	"runner": {
		"090580a6f05195e2b9daed1739c901e142212eeebefbafe226d37aa0e6ec8741",
		"7940a97ac2b089e5adc8ee8ca4761e4eba46f97b89e850fed92f39b2e4af290b",
		"d6b1c343fee7a09140634c85bf14067d235a57571f89e61825a6461d529f9ecc",
		"a5d11509a647765ebe45b14d3b206107ceeecc0fd3a9884a1aae0b37bdedca0d",
		"8f0154fc908359076bacf8a7a670ad3da495cf413476cc5081abbbe886253f8f",
		"5f0f4afca295c91151aa5266b90d912ccf1a34b93f8d14c760002e32c559663a",
		"21fb8503bd44123cb64663220ee119d7b267de73b704dfb405036c20ab630c6c",
		"136482ce5348496ca5789ddf923069ff96b238bc280cdb8b2a0543b58943fdef",
		"bc48acfe461aa7956e8d474d95b0dca2abd50343f21bd88f357b8ea05fc3ed5e",
		"e5aae83a0d446a5651599f1b3ddde4b61a16a9e238028287ebd94ef9e0340afc",
		"6ab4aad34b10e9eb58e38c922b214e59d558a3a229a9fbb5e0b9a45a78d93e2f",
		"619cd3f151126b2b6de892cb7aaa47d784b61ba26ee3d91b58ddd18076c7631d",
		"10c43812f758c6a6e8b0b79f4d137581a8ff9aadd092ab703e08b16750f92a68",
		"ae70886d5a4f02bd0a8ec5a45cd00f1b0841af15b88e380a396d88c2e993e175",
		"3d4c44b9236454e417ea961c9b645a7d0f818abda1a58ec86d7b34bcba88705b",
	},
	"operator": {
		"22b5ba3191fe107d3d91a28c7d321d8dabac6216e2cbd89d28ba3c222aec996c",
		"46aa0c80280cc6b7c7d46fa61508432916e3ecfc7894cb19c8dc9438f16d762b",
		"d7948a40545c116babede75439aa471818c80b6837d8fa80d4276fed9ee747e8",
		"7dd8b6635e4c677b495855ed588bf117464e1cfddb9be756090ccd0652be8885",
		"13a074242121fd6004b9edd969aa391b20364fbd1ff75327cb36d70f154517d6",
		"f2866414c134f84aec0c24f444a2ae1f6128ae1ee489a514b7cfb80eb975f1f4",
		"f374ef00077aa1170f2ab954bdaa3487c6153d7dc990402ede0c876b871968b9",
		"d191b6f67b2da9cfd02fab25b8c094aa281a9b82e2bd5005169a2c21dd9a9362",
		"c2fd94338ce3b5e9c6c6728840adae254dec4a2f439f29f61fb918e4eb72e5d3",
		"f4088bb0d8d77abf002dec10ffd6c1cf5988b995ed5f39bf3817c8458d659b7b",
		"7d8e480f9496dec12b52600d09fce423d082f7b8c419f72bec0c3cbc5da72687",
		"70ff702d188a6cc26d1141f0e946125825ea07efb4e02ab1d62cd9ebad4d7358",
		"a2ad877818ba3f87de5e5affc99b75bc6f6490caa666c4d19da13a8edc9f5eef",
		"fea7f54d1eca22457567bff209c78c8c2bc0a886e8d2a90ae8b33c72bf1b03b0",
		"9109206bba2db8873f4948216a32881acb01349a1e1941a8db0ba006f604f349",
	},
	"handover": {
		"f4393ddd06462859df19e2ccc20ad08426e44274dae4df0bed54b5c0408a39cc",
		"9dd9d0fec4f2244d88083353499045f89fff1aa20f81869bd25decf7d2f09831",
		"ba241846889c02eed7d6f3f33c65a73a963ec050d0ed2a234326ef9c131b750c",
		"702db979c3045e3e288465ff464ad989f0209f87a812893bbc9ed476bd91d7ac",
		"a12a9dae154e71585760d1ef51baaeb8dad63c801ce09714acb24a3bb1c910a1",
		"66126b45f064feeae5b4fc347e1a504d74016838d29a654109836f3516ff3cc4",
		"a05e5b31fa369d2328dde1697c3c09376cfb35cbcfc2a5108b5135646e3807a0",
		"ddf632ca6eaca21aa5966d770dedab82d3feff074604b08f13aa03c764484bb5",
		"7c26b186ceadf74d7ea190519b88f5399b9d3433b3e610a9a40061bf0d83dd2a",
		"7947a413f5889681e89f40982162dc292b3b98c2827d12284044ace79774395c",
		"97d7f51953c25e8264112ab72f2233e36969faed83516ade9a9a4f80ad2e9b0f",
		"c613656a994f6a7f9145e921126d67c9471815416f5bb5b65b2c8c91d9583354",
		"2944f3783980f22442bf2f5052254896fd8660b96d41eb5bc98844ae119dec05",
		"e2a43f8bd81e287baa241e443c5e9b3dbbadad844dc9f328dfac0b0fd51fa338",
		"891331744a56e1c7188506ab89536c2b1db99be46f89c877cf64959cfd6986a8",
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
