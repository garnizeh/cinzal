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
	"drifter":  "f1ad2b97c10df004ba877a188f048ee76a17efa4ebe207943ef6f96809a1880b",
	"runner":   "8e1814bf57a581c76f2007565de3055f0c45f2ad7ae72ca5a59d0bf2005defbf",
	"operator": "55e3d1bcba1dd7b6e6777d1127395eff447792a390debeb956591b9bb3990265",
	"handover": "9a15776294e40e672e5ed452b92f0471b2008faf9a7c22cf422b3548fe33fb98",
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
		"449d994ebbcc465c2c23cc7e894ea956bb8c19c40752a485361798d515432380",
		"6c4c5ecb708c81f83a3a401565fe62a119cfcb5304616ac45089b73369aec7bc",
		"5ed62182879e7a8e25d8dd899231306c469ce6497c43908efaa5d2b72c0ad669",
		"721dbe9158c7b801c7f066e64faeaa46d4315b1694da4cc9032f20677c5181ca",
		"08c22793350846568b9b13995dadfa216386d768967bb1022536d449a2c41c06",
		"eb6147524bf291a8f25638d0a48b8061467f92a0db8a37804b88ac52358e578b",
		"c12a405b5a6a461f481a13899767961ec8988f3c0324357e8024b46fdf708ce9",
		"9e402a0f2907f5b2c0103b035126e507af2d0d89fdd7afc231b5c04e369f5d43",
		"10a751f00c3658e4b04fbfc9442cd614180066d157b03fa0a32dd92dd96f9066",
		"afc0bbbce628788222b7fdd46338cdc9a4c6220640f33a07cb56b4a4ee3393a7",
		"904c9076ab0492a97417b5bf99ec849fc888490b6fef79a7aca9271e571be170",
		"bbf79edf69f6e2801fba091411872464d6149bec260ae6fbd40912e03026c85a",
		"0dad47f85c11feaf64be4aad87881ca47efb2a176ee7036a2144c76e44fe9573",
	},
	"runner": {
		"090580a6f05195e2b9daed1739c901e142212eeebefbafe226d37aa0e6ec8741",
		"7940a97ac2b089e5adc8ee8ca4761e4eba46f97b89e850fed92f39b2e4af290b",
		"6c7973b19cb0aeb4b6e4b702031a0132f5c5916cbdfad6220257ce9ff66b6033",
		"e63be1f40eaf4c359d73b435b659fbb6a463088829b11044c90c7e6ceef04e1d",
		"838cf448657326b886d3ee4a23f4ac48c89bccde326fad53fd4ed7fa1f26e055",
		"ac9622f1039d1d47c3034e6580920eb1d9c73937da9f41994db026652568c85f",
		"465e9687891d6560482141df00c122b250e7c1c5e275b3d077269799a61f5725",
		"fcd004dc4c5352754e449349fb577864c29cff5cf412409b56106092aac14e05",
		"4c134b5e60ebf2884bcadd1328642d87298b977086a737953c8d20873b907d81",
		"e98ac5cc2a4accc15b94643c400a1f7afb5f30e655708f1e0938c0007219377d",
		"6e540ffc126a0d3fcd693a64b3175ad1c84956f7785e24802ec37ac8fdde4fb5",
		"378cb0baa7dd54ba1d4c31da1bd6db38204b41f8e85f3ead616f78b293030e25",
		"fca8fe4476fa3813a642003e39ee053e5f002be48084c9e24bcdae7c0037bf1a",
		"758a847dc3f3c876b29b48acc385ca266163ee92c58e724dc2fa1d6d47d4b3b2",
		"9323900f61838a24bf0abaf6a36e1117ca537e3e05b895ea533e80e31af066e4",
	},
	"operator": {
		"d492a1eea9a4fdadf9980e5e3292c6af687154dab2394d1fef66600e0b0900e3",
		"38596c1e4786c0c9ceb117b1badb0627c81e9debb5fadd581de4c5ae1a4a1024",
		"2f00e124e496adad38c34131990e26a422198bb0d4357e8400ccfaee8606096b",
		"22857e6fdcea8b8658ae71041d6540399f63629dc061395cf23bfdeda270f18b",
		"41074a0bc7d2d9a621ca5af476d27bf4b1d067ebec7255d325ac2345c79868c7",
		"9773c882c74478909ec2cb95b7cb4fe1718e49a8a6fb21940bee04bbff5b1cf7",
		"c24a9a263a29bf23a6b4f4d488d1a69274891e1240e498ef1da008eb18e2e650",
		"9d177db4a5b05aab86a3cbbe45ce58cc434eacda2dcbc172315d42c2951fc000",
		"7ef102180bd94f0ac1a99e9942c13c4036a41b35640f99c0d1d88283ac2d77f1",
		"f15f70045018e35d26f5491bc3c4ae947d75956cb196e64e7c11e2e8405f839a",
		"8702bd8ce362d613fb54aa090babffbc96600f645abea5b6033226973607587a",
		"a2b9725da2d3fb6dff3c41ed7efdf0c2b1f35a3a0b0ad4ec9949df36e45d51da",
		"93b6cc8f205f60f504b5165bad1719983f300fa8618edc0ddc4645a99972b206",
		"d249d3a2f5c65cc8acd9d6c8857342278b1b59aa0881d76677f8ebb52e6cd881",
		"8c7a42fe7cb2101b8449ad6fd37c1a84d38e95900edc7925df05493ca0bdb5c1",
	},
	"handover": {
		"fae1f46ed9f22ee36ad7881b06fceb451baa5d3a475567ecc9fc6606bdc853bf",
		"46d0c1a626fd9d5a41cdcc829301f35543d6368e25f05fba7599d9c8ef4ed57d",
		"7bae0765718930052c91d11a70e026d28150d464719ef025293c1985266536b3",
		"fec80f137d51e73a4c273c005959c769212a7373025e8fd9a037b8a38c120566",
		"876b892b6707f5b67ab97724379bfdfacea0693a879b799456d5f35e487baa06",
		"ba56c05ba16ddf9b677d0633ef0ae7e425a3c738cc97df16de33d0203c97dc81",
		"6d549c694d218a2a160f7af0e18f732ec0d7f0f172e7c1103dad5107e6bb64f9",
		"b3f3a76bafb1c7a13bbed8bb6f0531125dcc2b228a1a43d6b381081ed4480541",
		"5a4ef9406a35a8905cfa82c7b4e1b695831aa5c59a561d0f7b792aeb8b8cfdec",
		"bfee425d64708ab1f325fd81ecce62066a3b6ef7471dbc40919e79129cbd3e58",
		"da4983e33abbf86810a3e3f307743642e4af5669dbd20159d14724059b08880c",
		"7696f798697c3bebd13c0b36e49146ca56b55096b7cbb66abec10ee9e3d20319",
		"de697ca8f9971e68742ab69d5c0da335c13fc1a49c659e8632537f9eb891c0b7",
		"0121b201bf81fd4d3a4c7a861504555c189bfa465a8bf70375850668b61e760a",
		"9e43f358b5bfad00ee8555d86c9e00efc4a8812d4d7dfb38e13553224c3081a7",
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
