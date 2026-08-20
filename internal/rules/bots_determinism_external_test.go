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
		"8d8cd813d34330644b5b67a42458da68e78e3bd08ed3fd50d83bf39467d7d0db",
		"2fc33cdd4074bfc8acbafaded136e9ec40ac722cf03df469580e7ff23487cf4c",
		"1bfb5240601c297d50c978b00d3030ee682383b3b800d7125538774bf95c7f14",
		"d4e524253f654d079a291aaf608bfda9bb1b1c6eb205e93977c3b63331afbf6f",
		"dce78c9a15909e05f9a21ee53e259004ba9ac9a644e823dbe1f5b29a67036dc8",
		"3561b847f16d9bc9933688a11e5c4a17a62cfdb6daa23356e490b3eb3d2b5a1c",
		"9dc682b81f9fb76de400374423d4dab39d8729565c74dfc112841b3ebd8c3373",
		"65794beea67e04c9643e454ebd1e4ff07f3242e4c34d4dc76bdaacdb77634614",
		"192909230eb81b56450d8eb6c94e6ae2444690b651f9694442808907c5a0890b",
		"f9cbc2bf7e10825a51937d65c2d1bc667adee71ad0c17d98335a40b39f469c8d",
		"38bc9eb2ab5411dde47186be5f2178bd99a173c02901af334ed50bca72130f06",
		"b41cebe9462252ae902a2a8c93ea5c5d6d0c90fad1aba8f217d476623b06392c",
		"169e9a5f7b3fffcdf7ff7394104f6626525652a4d32737f2565e3330c3c664f4",
		"2c7a5a9a4377443141e2796e92e37c97d55193a9619d5fa21b08e6b075d99de6",
		"df0c9cf664dbc3e362241f8ad41cd4af1250c2f51363b8881d49d7c21e9fd8c5",
	},
	"runner": {
		"0a13fd690bd10ec2642e2153c2287a6e1ff2f10ff02d381554b522f5a87c38e2",
		"019440a097df702015ab694b97e8f37dc2bd04418672260a4fdca27d002c63e4",
		"63e5d5fba65939318de8ff3d6e8fcc28e349fd725a61dab40c2b5b42c01415d3",
		"39bab07d940a7eb2bb6a2db8b9d85b1042d1314280003fdfe33bd206be0d4b96",
		"a42c630b6cf8c10a2c70b7e7dcc1d6f29390454bfc3681037d1ff6772d5d9b78",
		"75381313b68f37b01a3b25e8f80f05d09f473014adaa12ab0db6a01964dda2b7",
		"d82c1df24abd56af6cfa0b18d8fce3890dda4d49c91527145b64538248344a21",
		"f5607b4aba44e388d44c1408dfb09ed6bec84ed2e05f92dde41e0d72ccf828ce",
		"650b8381bc5b9a161ae3e26400d4755001e18c5685655bc5d32d933d105e1be2",
		"2666644ce2a64162836ebb50eac11dea7ca09e8c777a358fa638f4f1fc68397f",
		"b7bed9108d42a37b12415b6b3f82186b95c124efc71504a5a5acca5e119b882a",
		"81feeedc632b0a960c97106cca4aff2eaa920098b4acdb8cd277b39c4261646d",
		"0a3e753b775342b299efdfb962957361db97ab2d990668cdc9b4c8f28980dc29",
		"fce8f55cbbba500b4532c33e3da96d9b255c788792e98819a4aac9c15b6bb99b",
		"d79de7890c29c00406d61135ca96791fe9c2f78ceaa853a557aba5000044510c",
	},
	"operator": {
		"409e33d77d36536a2751605107f2d7dd41910cd65a6ff181e1a71e7c56aa124d",
		"5725b838523d11ddc0544547bdb54931d194ac00d4fe529bf14b4d1e4664462a",
		"4db839ef4446cfd6f701330530090e8c14ca8c1773df4e4960be9137996cf2d1",
		"6e8bc07448ce6d9e708fb326397c5c50c894373b02b19c127079cf0a9a64fdf8",
		"1c790cebb282ef9b064361828181c5948cecbfc5d1c36bce78ccfe22b5e5fa88",
		"7f5566859aa96612166c6d204f3cf73cdb4cb9dbba7581700a0d73d1c27d071b",
		"81ecbd119d860230dc5fbe40c0b9a1990a739ea79c2659cbf656732556784b9f",
		"3c5e2ed8229ca496da79338a221dfb5a13ba3c99833d01608436e9f387c17f3f",
		"6a358173c74f88a9d3e5ed6e0d72a251202b8c73b1cf51fa81955b4f5dd6187f",
		"21562187341eaf61cea7851521d295444caed2f19e2b1858105fb8eb5cdab116",
		"e91c126841ff5b3cf3c6b01a62a8897c90bc50b9899e374870da4997dacc214a",
		"0c7b207562ec2762b5ce02952ba7f972ed8714ba34acba542b4076bcb88c5e9b",
		"46c69bfe5dbc3b45f656a0914d311594f5c42e71c488e9f5ad11d734d7064b06",
		"7ce066ecafd62fcdd1295a123e6d71add01685fcbe287a01fe3314ae87c4940a",
		"64be2eb6c90d780584863641d41c10d2f472a7821ea85693b7082e131a91fe89",
	},
	"handover": {
		"7974d3b9b93783c22426981b94182270424caed0d9d819140fbf6e430ac55b5a",
		"2313c0ca1a5f79618f145556d8218458fa134be5b4c82cd3223567d7c459e9aa",
		"3d37b69d1226032411e5cc14afb0b9a877fb5dceecebe636ea2caa45ea58119c",
		"72f34e4f83ea8156c990b164e568d1e8d590e0d456f0513c39df28c94dcb2a0c",
		"4ece0993db4a20fe4c30584ae5a6e8ee3a475ce99c3caeaba8c78c968eca19ab",
		"e381a3220e3e62dcd0ee01b765499474906490405cb824ea087005efff536131",
		"0b98b80c9d04b080528591a39d5e9cacefa5c20fbf9e8735dccc66bfdbe7119d",
		"eaed389a50ba1868ca67c9716c3ac65a79e0e00772ad1988043b54bb4ff46709",
		"5a9288cab2aa86c4b77acbc76813270fd415f254049929ead69e9250e851cec4",
		"3e654ac57d055e5bc2ad030b42657835c30aba25ad53ae00dda71ecfd43ac30c",
		"c574ba7df27ccbe5271a102e0e4cdfde1aa7ed2e3f932b0af38979adc9f6c59f",
		"244c004bad5ef3541dbcc1f02ee2495ef36af8d4f8e67f9e3e686667db2bea82",
		"b7b0b47bedcc526da097a4c6cd3a45534e7061e7c807a86b4e3c3aebe0e90beb",
		"a2a34b89213272dbf1246acde3857ae6d6f1fee879f63ac9deb4c798b64bab22",
		"e13a0ec18084f493bbf2719adc07f1a18acbca1b38f968f7607e85fc9174a42f",
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
