# Handoff: issue #63 (internal/rules: contracts) — paused on a spec gap

**Branch:** `task/63-contracts`, based on `main` @ `140a53a` (D23 — starting-fog
seeding, docs-only).
**Status:** WIP, NOT merge-ready. Blocked on
[D24 (issue #120)](https://github.com/garnizeh/cinzal/issues/120), filed
and open, before the acceptance criteria can actually be satisfied.
Issues #63 and #85 have been updated to reference it; #73 (Word of Work,
which will reuse `GenerateOffer`) got a cross-reference comment.
**Delete this file** before opening the real PR for #63 — it's a handoff
note for whichever agent/session resumes this branch, not project
documentation.

## What #63 asks for

GitHub issue #63: `internal/rules: contracts — offers, cooldown, tier
gating, cargo` (M1 — Rules core). Full body is on the issue; the short
version: implement the contract subsystem end to end — offer generation
(D6 tier-mix + D7 pool fallback), the Contact Cooldown (GDD §8.2), the
tier table (GDD §8.3), and cargo (GDD §8.4: Bound vs Loose, collection
eligibility, the once-per-contract Deadline Pause, loose-crate heat).

All five blockers (#57, #60, #44/D6, #45/D7, #115/D23) are closed. D23
landed as a **decision doc only** (PR #119) — the actual fog-seeding
algorithm in `internal/rules/initial.go` was left for whoever picked up
#63, per D23's own consequences section.

**Scope decision made with the user before implementation started:** build
the *full* pure contract/cargo rule module — not just offer generation —
as standalone, directly-testable pure functions. `Resolve()` (#67),
Pickup/Deliver actions (#70), `writeTrail` (#71), and Upkeep (#74) are
separate open issues that will later call these functions from inside the
round pipeline; this task does not build that pipeline or touch
`game.Order`'s shape.

The original implementation plan (design phase, before coding started) is
summarized in full further down in this file, since the plan-mode file
that produced it lives outside the repo (`~/.claude/plans/`, not
committed) and won't be visible to a fresh session.

## What's implemented and working (this commit)

- **`internal/rules/graph.go`** — `Graph.distances(src) []int`, a plain BFS
  on the live navigable graph (GDD §9.1a item 0): skips any node with
  `SinkholeRounds > 0`, respects `Node.Edges` already reflecting Bridge
  Down. Used by both D23's fog seeding and the contract distance-band
  checks. Fully tested (`graph_test.go`, 6 tests, all passing).

- **`internal/rules/initial.go`** — D23's two-step fog-seeding algorithm
  implemented in `seatPlayers`: opening sight (`FogInSight` on start +
  neighbours, GDD §7.2 run at round 0) then constraint 7 (`FogKnown` on
  every still-Hidden Warehouse within graph distance ≤2). Signature changed
  to `seatPlayers(g gen.Graph, graph Graph, cfg game.Config, players int)`
  so it can call `graph.distances()` instead of re-deriving BFS from
  `gen.Graph`. **This part works correctly** — verified directly (see the
  debug trace below).

- **`internal/game/config.go`** — `ContractTier` gained `OfferWeight int`
  (D6), validated positive in `Config.Validate`, defaulted to `1` in
  `DefaultConfig()`. `internal/game` tests all pass (59 tests).

- **`internal/rules/rng_purpose.go`** — `PurposeContractOffer` split into
  `PurposeContractOfferTier` (`contract.offer.tier`, 2 draws always) and
  `PurposeContractOfferPick` (`contract.offer.pick`, 0-3 draws, one per
  filled slot), per D6's own deferred table edit. `rng_test.go` updated to
  match (it referenced the old constant).

- **`internal/rules/contracts.go`** — `ContractOffer` type,
  `eligibleTiers`, `weightedTierDraw` (D6's weighted independent draw),
  `contractCandidates` (Known-Warehouse × any-Border pool builder, navigable
  graph, D7's distance bands), `cascade` (D7's per-slot fallback, target
  down to 0, without-replacement `used` map), `GenerateOffer` (the full
  Phase-2 algorithm: cooldown/full-slots hold, D6 slot assignment, D7
  cascade in slot order), `RoundsToNextOffer`/`offerDue` (HUD number,
  `LastOfferRound == 0` = "never offered" = always due — this is what makes
  the opening offer unconditional on the *cooldown* axis), `nextContractID`
  (slot-index 0/1 scheme), `AcceptOffer`/`DeclineOffer` (cooldown restart,
  pure — clone-and-return, never mutate the input).

- **`internal/rules/cargo.go`** — `CanCollect` (Bound needs a matching
  origin/destination contract, Loose needs none), `Contract.WithDeadlinePause`
  (once-per-contract, GDD §8.4), `NextLooseCrateHeldRounds`/
  `LooseCrateHeatFires` (threshold at 2 consecutive rounds), `Deliver`/
  `Penalize` (GDD §8.3 payout/penalty lookups).

- **`docs/project/cinzal-architecture-rfc.md`** — RFC §6.4's
  `contract.offer` row split into the two new purpose rows, per D6/D7's own
  text saying this table edit "lands with #63". Changelog entry added,
  revision bumped r19 → r20.

- **Tests**: `graph_test.go` (6), `contracts_test.go` (12 fast tests + the
  1000×4-seed sweep, see below), `cargo_test.go` (6), `initial_test.go`
  updated (`TestInitialFogSeededByD23` replaces the old
  `TestInitialFogAllHidden`, which asserted everything stayed `FogHidden` —
  no longer true post-D23). `config_test.go` updated for `OfferWeight`.

All of the above pass individually. `make lint`, `make packages`,
`make purity`, `make fog`, `make debug-isolation`, `make secrets` all pass
clean. **`gofmt` was violated once by `cargo_test.go`'s struct-literal
alignment — already fixed, re-verified clean.**

## The blocker: D7's guarantee doesn't actually hold

`TestGenerateOfferOpeningOfferNeverEmpty` (the AC #1 regression test — 1000
seeds × {2,3,4,5} players, asserts `GenerateOffer` delivers ≥1 contract for
every seat at setup) **fails**, first hit at `players=2 seed=1 seat=1`
(seed via `seedFromInt(1)`, sha256-derived, see `rng_test.go`).

**This is not an implementation bug.** Debug trace (reproduced with a throwaway test, since deleted — rerun by
adding it back temporarily if needed):

```
seat=1 infamy=0 position=11 contracts=0 lastOfferRound=0
known warehouses: [7]
all borders: [0 4 9]
distances from warehouse 7:
  border 0: dist=5
  border 4: dist=2
  border 9: dist=5
eligible tiers: [0]   (Tier I only — Infamy 0)
```

Tier I's band is `[3,4]` (GDD §8.3). None of the three Border distances
(5, 2, 5) fall in it. Since Infamy 0 only makes Tier I eligible, D7's
cascade for every slot targets Tier I and can't go anywhere — the pool is
genuinely empty and the offer correctly **holds**, per the algorithm as
literally specified in D6+D7. `GenerateOffer` is doing exactly what it's
supposed to; the rule it's following has a hole.

**Root cause, traced into D7's own text** (`docs/decisions/D07-contract-pool-fallback.md`):

D7's Reasoning section claims *"reachability is the only thing left that
can empty a pool"*, resting on *"every Known-Warehouse-to-Border distance
guaranteed ≥ 3"* (citing GDD §6.1 constraint 6's "every delivery costs at
least 3 steps"). But D7's own text, one paragraph later, already flags this
as unresolved: *"constraint 6's own two sentences aren't obviously the same
fact — 'not adjacent' alone only forces distance ≥ 2, and whether the
stated 3-step floor holds by construction or needs a sharper constraint
that isn't spelled out is a map-generation question, not this decision's."*

The debug trace above is a **concrete counterexample**: warehouse 7 has a
Border at distance 2 — below every tier's minimum, confirming constraint 6
really only guarantees ≥2, not ≥3.

There's a second, independent problem even if that floor did hold: D7's
"bands cover everything ≥3 with no gap" argument is about the *union* of
all four tiers. It's true for a Legend (all four eligible) but not for
anyone below — a Nobody's eligible set is Tier I alone (width 2, `[3,4]`);
Known adds Tier II (`[3,6]` combined); a reachable pair can easily sit
outside even the *union* of what a low-Infamy seat is eligible for (e.g.
distance 5, 7, 8, 9, 10 — all missed by a Nobody, several missed by a
Known-tier player too).

So issue #63's own acceptance criterion — *"Opening offer generates for
every seat, on ≥ 1000 seeds... the §8.1 deadlock is a regression test, not
a memory"* — is currently **false** under D6+D7+D23 as decided, for a
structural reason those decisions didn't fully close (D7 flagged it and
moved on; nobody came back to it because #63 is the first place it bites).

## User's decision on how to proceed

Presented three options (accept the gap and weaken the test / implement
as-decided and flag the gap in the PR / patch it inline as a documented
judgment call). **User chose: file a new decision (in this repo's
`docs/decisions/` process) and pause #63 there** — not to invent a rule fix
inline. Explicitly asked to commit all WIP work first so the branch isn't
lost while the decision resolves. This commit is that.

## Next steps (in order)

1. ~~File the decision as a GitHub issue~~ **Done — [D24 is issue #120](https://github.com/garnizeh/cinzal/issues/120)**,
   open, labels `decision`+`area:rules`, options A/B/C/D drafted (accept
   the gap / widen the cascade generally / fix map-gen's constraint 6 /
   scope the fix to the opening offer only — see the issue for full text).
   No `## Decision`/`## Reasoning` yet — add those, plus a
   `docs/decisions/D24-*.md` file, once the maintainer actually rules on
   it.

2. **Deliberately NOT done — catalogue rows deferred.** Checked D23's
   actual history (not just the written convention): its filing commit
   (`6a98d63`) created `D23-starting-fog-seeding.md` with `Status: decided`
   directly, updated the README/plan.md catalogue, and closed its issue —
   all in **one** PR, never passing through a separately-committed "open"
   row. D16–D22's bare-text "open" rows are a one-time bootstrap
   (`4824d8f`, project start), not the pattern a mid-implementation
   decision like this one actually follows. So: no `docs/decisions/`
   catalogue edit now — issue #120 alone is the "this is open" record in
   the meantime, matching how D20–D22 work today. (An earlier version of
   this session's work *did* commit a catalogue-row edit for D24 and even
   drafted a second, separate PR to land it on `main` ahead of the
   decision — reverted, on the user's call, once the D23 precedent above
   was actually checked instead of assumed.)

3. ~~Update dependent issues~~ **Done** — #63 and #85 both note the new
   block in their bodies; #73 (Word of Work, which will reuse
   `GenerateOffer`) got a cross-reference comment, not a body edit (it's
   informational, not a hard block on starting #73).

4. **Once D24 is decided, one PR does everything** (matching `6a98d63`
   exactly): write `docs/decisions/D24-*.md` (status `decided`,
   `## Decision` + `## Reasoning` filled in), add its catalogue row to
   `docs/decisions/README.md` and `docs/project/cinzal-implementation-plan.md`
   §3.2 already as `decided` (never an intermediate `open` commit), close
   #120, implement whatever it settles on (most likely a small change to
   `GenerateOffer`'s guaranteed-slot logic in `contracts.go`), fix
   `TestGenerateOfferOpeningOfferNeverEmpty` if its assertion needs to
   change shape, rerun the full sweep, then proceed to `make check` /
   `make test` (full, not the fast subset) and open the real PR for #63.
   Also resolve #63's and #85's bodies back to a clean
   "blocked by" list (drop the inline explanation paragraph, since the
   issue link + closed-decision convention carries it from there, matching
   how #115/D23 reads on #63 today) and delete this handoff file.

## Gotchas hit this session, worth not re-learning

- **`cmd | tee file` masks the real exit code.** `run_in_background`'s
  reported exit code for a piped command reflects the *last* command in the
  pipe (`tee`, which always succeeds), not `go test`. The
  `TestGenerateOfferOpeningOfferNeverEmpty` failure was reported as "exit
  code 0" this way and only caught by actually reading the log file
  content, not trusting the notification. **Always read the actual test
  output, never trust a piped background command's reported exit code.**
- The two 1000-seed sweeps (`internal/rules/gen`'s existing ones, and this
  task's `TestGenerateOfferOpeningOfferNeverEmpty` / the now-deleted
  fog-only duplicate in `initial_test.go`) are each genuinely slow —
  budget several minutes, run them backgrounded, and read the log directly
  rather than polling exit codes.
- `internal/rules/gen`'s `bfsDistances` (in `gen/builder.go`) is a
  different, not-reusable BFS — different `Node`/`Graph` shape, no
  `SinkholeRounds` (Sinkholes can't exist pre-Setup). `graph.go`'s
  `distances()` is deliberately a second, `rules`-package-local
  implementation for exactly this reason, not a missed reuse opportunity.

## Original plan summary (for reference — full detail was in the design-phase plan file)

Files touched/added, and why, matches exactly what's listed under "What's
implemented and working" above — the plan and the implementation didn't
diverge except for discovering the D7 gap mid-way through writing
`contracts_test.go`'s sweep test. The plan's verification section called
for `make check` + `make test` at the end, which is where this session
stopped (the sweep failure was caught before reaching the full `make
check` run — `make lint` and the other fast gates were already run and
pass clean; `make test`'s full run is what surfaced the failure).
