# D38 — R9's "Board going unused" leading indicator has no computed row: define one as map utilization, ship it as a diagnostic, or leave it unimplemented?

**Status:** decided
**Blocks:** any future `MapByPlayers` retuning task that cites R9's leading indicator; M5/M5.5's telemetry instrumentation for GDD §22 rows 15-16
**Decided:** 2026-08-20
**Issue:** [#233](https://github.com/garnizeh/cinzal/issues/233)

## The question

GDD §20's R9 paragraph ends with an instruction: *"Watch for the Board going unused (§22) as the leading indicator."* §22's confrontations-per-match row repeats the pointer in its own "Fails if" column: *"> 12 → map too small (R9); Board goes unused as leading indicator."*

[#233](https://github.com/garnizeh/cinzal/issues/233) found, while executing [#229](https://github.com/garnizeh/cinzal/issues/229)'s own "check whether that already has a computed row" bullet, that neither pointer resolves to anything named that: §22's twenty numbered rows do not contain one, and nothing in `internal/telemetry` computes anything shaped like it. [D37](D37-five-player-confrontation-load.md) then had to read the indicator through two other §22 rows while deciding the 5-player node count, and recorded the gap as load-bearing rather than tidy-up: *"A future revisit — M5.5's or otherwise — should have the real row."*

## Why it is open

- **No operational definition exists.** `#233` lists four candidate readings and notes they are not interchangeable: the share of nodes that record zero player-round presence; the share never Known or in sight to any player; the share of node *types* whose function never triggers; or something narrower tied to §7.5's Board UI, which does not exist until M5.
- **Whether it becomes a full §22 row** — target band, verdict tier, a column in `cmd/simulate`'s CSV — **or stays a lighter diagnostic is itself a choice.** Every other headless §22 row went through [D33](D33-telemetry-event-stream-coverage.md)'s audit and [D35](D35-simulation-sample-size-and-verdict-rule.md)'s sample-size and verdict treatment. This one has had neither.

## Options

**A — Define it as the share of nodes with zero player-round presence across the match, and add it as a new §22 row.** `#233` calls this "the most literal reading of 'going unused'" and the cheapest to compute: route steps, ending positions and posts are already in `telemetry.Match`'s existing inputs, so no new event is needed. Needs a target band, which nothing in the GDD or the decision log states — proposed here or deferred to a follow-up measurement task.

**B — Ship it as a diagnostic-only stat in `cmd/simulate`'s output, with no band.** No verdict rule, no D35 tier assignment; a number a maintainer reads by eye when re-running a node-count sweep. Risks becoming exactly the kind of number nobody checks — which is what happened to it as inline prose.

**C — Leave it unimplemented and note in §22 that the pointer has no computed row.** Describes today's state without inventing a metric under time pressure. Costs nothing, and leaves R9's own stated safeguard permanently unchecked.

**D — The pointer already names something, and it is not a new metric: make it resolve.** Not in `#233`'s list. "The Board" is GDD §7.5's deduction UI, and §22 already carries two rows that measure whether it is being used — row 15 (attribution queries) and row 16 (Heat Map opened per player per match), both classified by D33 as UI instrumentation and deferred to M5.5 and M5. Under this option nothing headless is created; §20's and §22's pointers are made to name those rows, and the two rows D37 actually read are recorded for the different question they answer.

## Decision

**D.**

R9's leading indicator is not missing. It is **deferred, on the same grounds and by the same audit as every other UI-dependent row in §22** — and no bot simulation can ever produce it, because a bot does not open a UI. What was missing is the cross-reference: R9's own text says "(§22)" and never says *which rows*, so D33's audit deferred them for UI reasons without anyone noticing they were R9's own safeguard, and D37 had to reach for the nearest headless substitutes.

Concretely:

- **§20's R9 sentence and §22's R9 row now name rows 15 and 16 explicitly**, and say that both are UI instrumentation which M2 cannot produce.
- **Rows 15 and 16 carry the back-reference** — each says it is R9's leading indicator, so the M5/M5.5 work that builds them knows it is building a safeguard and not only a usage stat.
- **Row 19 (Heat Map entries at low confidence) is named for what D37 actually used it as**: the headless read of the *converse* question — whether the remedy R9 prescribes has thinned the Board's own data past usefulness. Its existing "Fails if" text already says so in as many words: *"> 60% → observation coverage too thin for the tool to be usable."* "The tool" is the Heat Map, which is the Board.
- **Nothing is added.** No new §22 row, no new `MatchSummary` field, no new event, no `internal/rules` change, no new CSV column.

Option A is rejected on two independent grounds — it names the wrong object, and, measured, it moves in the wrong direction. Option B fails for the first of those reasons alone. Option C is right about the code and wrong about §22, which is a worse place to be wrong: it would write "unimplemented" against two rows that are already scheduled.

## Reasoning

### 1. "The Board" is a proper noun in this document, and both pointers use the capital

GDD §7.5 is titled **The Board** and defines it: *"the client-side intelligence wall, and P3 lives inside it"* — the Log, anchored Attribution, the Heat Map, and pins. The v1.1 changelog promoted it to a v1 requirement in exactly those terms: *"§7.5 The Board — the deduction UI is now a v1 requirement, not a nice-to-have."*

Where the GDD means the terrain it says "the map", or "the board" in lower case:

- §7.2: *"at an average degree of 3 that is 15 to 20 nodes of sight per round out of 25 — most of the board, every round, for free."*
- §7.5's saturation table: *"A four-step cone covers 94% of the board after a single round."*
- §11: *"you lose: half your mobility, your anonymity in the trail, your position on the board."*

Both places that name the leading indicator use the capital: §20's *"Watch for the Board going unused"* and §22's *"Board goes unused as leading indicator."* Under `#233`'s option A the sentence would have to be read as terrain, which is the one reading the capital rules out.

### 2. The roadmap already states which rows measure it

`docs/project/cinzal-implementation-plan.md`, risk register, **P6**:

> | **P6** | The Board (GDD §7.5) is underestimated | It is inside v1 scope, is four distinct tools, and is where P3 (the design pillar) actually lives — a thin Board means the deduction game does not land | Scoped explicitly in M5's deliverables; **§22's Heat Map and Attribution metrics are the check that it is being used.** |

That is rows 16 and 15, named as the Board-usage check, in the same document that scheduled them for M5. Nothing had to be invented to make R9's pointer resolve; it only had to be followed.

### 3. R9's own mechanism names the reading, not the terrain

> **R9 — Encounters may be far too frequent at 4–5 players.** If you collide with someone twenty times in fifteen rounds, **you never need to read the trail — you just walk.** That kills the deduction layer as thoroughly as sparsity would, but from the other direction…

What goes unused is the deduction layer. A player who walks into a rival every other round still visits nodes normally — they stop consulting the Log, stop running Attribution, and stop reading the Heat Map, because direct contact is telling them what deduction was for. Rows 15 and 16 count exactly those two acts. The map is as used as ever; the tool is not.

### 4. Which means the indicator was never available to M2 — and that is a finding, not a gap

D33's audit classified row 15 as *"a human-facing tool interaction over a UI that does not exist yet… not a headless fact at all"* and row 16 as *"UI instrumentation — a click count on a feature not yet built."* Both stand. The consequence nobody drew at the time is that **R9's leading indicator is structurally unmeasurable by bot simulation**, in a stronger sense than the rest of §22's deferred rows: every other row M2 owes is a fact about the *match*, and this one is a fact about the *player*. Ten thousand bot matches produce zero Heat Map opens, and always will.

So D37 could not have had this indicator regardless of what `#233` built — and a future revisit gets it from M5's instrumentation and M5.5's play sessions, not from a telemetry task. That is a real answer to the question `#233` asked, and it is not the answer `#233` expected.

### 5. Option A measures the map, and — measured — it moves in the wrong direction

Set the naming argument aside and take option A entirely on its own terms. Measured at [D35](D35-simulation-sample-size-and-verdict-rule.md)'s rigor: 10,000 matches per configuration, `mean ± 1.96·s/√n` over the per-match vector, two independently drawn root seeds, Drifter — R9's own tier, a map-geometry question. Git SHA `71eb1a0`, `game.DefaultConfig()` with `MapByPlayers` overridden in-process (no source edit is needed: it is a plain `map[int]MapSpec` field), edge ranges chosen to hold average degree inside the 2.8-3.2 band every shipped §6.1 row uses. Two definitions, because `#233`'s wording admits both:

- **U_end** — share of nodes no seat ever *ended a round on*.
- **U_touch** — `#233`'s own literal wording: share of nodes with no route pass-through, no ending position, and no post, at any point in the match. Setup positions count as presence.

Both were computed by a throwaway `cmd/simulate` test that re-runs `RunMatch`'s own fold while recording each seat's `Position` after every `Resolve`, then deleted — deliberately not merged, since this decision's conclusion is that neither belongs in the shipped metric set. It had to re-run the fold rather than read `telemetry.Match`'s inputs, and that is worth noting against option A's "cheapest to compute" claim: `Match`'s D34-fixed signature carries the *final* state, so per-round ending positions are not in it. A merged version of option A would have to take the declared route from the order log instead and accept that a halted or truncated route overstates where a seat actually stood.

Root seed 1 is `cinzal-simulate-default-root-seed-v1`, the same default D37 and `#229` swept under; root seed 2 is drawn independently for this decision and is not D37's second seed. Node counts stop at 36 — the generator's real ceiling ([#239](https://github.com/garnizeh/cinzal/issues/239)) — because going past it needs D10's lattice enlarged, and nothing here turns on the points beyond.

| Configuration | U_end, seed 1 | U_end, seed 2 | U_touch, seed 1 | U_touch, seed 2 | Confrontations/match, seed 1 |
|---|---|---|---|---|---|
| **4 players, 28 nodes** *(shipped; `#229`, accepted)* | 0.2291 [0.2276, 0.2305] | 0.2293 [0.2278, 0.2307] | 0.0489 [0.0479, 0.0499] | 0.0493 [0.0483, 0.0503] | 11.8121 [11.7370, 11.8872] |
| **5 players, 28 nodes** *(shipped; trips R9, deferred)* | **0.1763** [0.1749, 0.1777] | **0.1778** [0.1764, 0.1792] | **0.0308** [0.0301, 0.0316] | **0.0309** [0.0301, 0.0316] | 19.0986 [19.0109, 19.1863] |
| 5 players, 32 nodes | 0.2086 [0.2072, 0.2099] | 0.2088 [0.2074, 0.2102] | 0.0417 [0.0408, 0.0425] | 0.0414 [0.0406, 0.0423] | 16.5381 [16.4535, 16.6227] |
| 5 players, 36 nodes | 0.2446 [0.2432, 0.2459] | 0.2447 [0.2434, 0.2461] | 0.0569 [0.0560, 0.0579] | 0.0577 [0.0567, 0.0587] | 14.7754 [14.6903, 14.8605] |

The last column is the check that this is measuring the same matches D37 measured, not a re-implementation that drifted: on the shared root seed it reproduces D37's own 5-player figures **exactly** — 19.0986 [19.0109, 19.1863], 16.5381 [16.4535, 16.6227] and 14.7754 [14.6903, 14.8605], identical to four decimals at 28, 32 and 36 nodes — and the 4-player row lands inside `#229`'s published 11.75-11.81.

Three things follow, and each is sufficient on its own:

**It reads healthiest exactly when R9 is at its worst.** R9's failure is a map that is *too small*. Both definitions fall monotonically as node count falls — 0.2446 → 0.2086 → 0.1763 on U_end as the 5-player map goes 36 → 32 → 28 nodes, with no interval on either seed overlapping its neighbour. On a small map players cover it thoroughly, so "unused nodes" goes to its floor. A leading indicator that improves as the failure it guards worsens is not a leading indicator.

**At the two configurations this repository has already ruled on, it inverts the verdict.** The 4-player 28-node map is the configuration this repository has accepted as correct (`#229`, 11.81 confrontations per match, inside the band). The 5-player 28-node map trips R9 at 19.10 and is deferred to M5.5 as unmet (D37). Option A's metric ranks **the accepted map as more unused than the failing one** — 0.2291 against 0.1763 on U_end, 0.0489 against 0.0308 on U_touch, on both root seeds, every interval clear of every other. A §22 row scored that way, read against any single band, would have told `#229` to raise the *4-player* count and leave the 5-player row alone.

That inversion is not an artifact of one comparison: the metric is close to a plain function of **nodes per player-round**, which is map size divided by traffic. The 4-player map's 28 nodes over 4 × 15 = 60 player-rounds gives 0.467, and interpolating the 5-player curve to that same ratio predicts 0.236 against the 0.2291 measured — 0.007 apart, on intervals of ±0.0014. A single band spanning §22 row 10's own "4-5 players" would therefore be dominated by the player count, not by the map's shape, which is the thing R9 is asking about.

**U_touch has no room to hold a band.** `#233`'s literal wording sits between 0.031 and 0.058 — near its floor at every configuration measured, a span of 2.7 points across a node range over which the confrontation rate moves from 19.10 to 14.78. Almost every node on a Cinzal map gets stepped on by somebody; a band drawn in that strip would be a band on the third decimal.

None of this makes map utilization a bad thing to know — it is a real property, and the numbers above are a usable baseline. It makes it the wrong thing to put behind R9's pointer.

### 6. What D37 actually needed, and what it should be called

One phrase, two opposite failures:

| | The failure | The map moves | The indicator | Headless? |
|---|---|---|---|---|
| **R9's own** | Collisions are so frequent that deduction is unnecessary — the Board is not consulted | too **small** | §22 rows **15** and **16** — attribution queries, Heat Map opens | **No.** M5/M5.5 UI instrumentation |
| **R9's remedy overshooting** | Observation coverage is so thin that deduction is unreliable — the Board is not *usable* | too **large** | §22 rows **8** and **19** — share of map under sight, Heat Map entries at low confidence | **Yes.** Both computed since [#197](https://github.com/garnizeh/cinzal/issues/197) |

D37 needed the second — it was raising node count and asking how far it could go — and read rows 8 and 19, correctly, while calling them "the two nearest existing rows… proxies for it rather than the thing itself." They are not proxies for the second failure. They *are* the second failure's rows, and row 19's own "Fails if" text has said so since §22 was written. They were only ever proxies for the first one, which is not what D37 was measuring.

Recording this distinction in §22 is the substantive fix here: the next node-count change reads rows 8 and 19 as the named guardrail on its own remedy, and does not go looking for a row that will arrive with M5's instrumentation.

**One caveat travels with that, and it is D37's, not this decision's to settle.** Neither row has a failing line in the direction that matters. Row 8's stated action is `> 65%` ("post sight still too generous") with nothing defined on the low side at all; row 19's is `> 60%`, which the 0.445 measured at 46 nodes does not reach. Both left their *target bands* well before R9 cleared, and neither tripped a stated *action* — which is exactly why D37 had to write "this is not a second tripped exit criterion, and this decision does not claim one." Whether a low-side action line belongs on row 8, and whether row 19's `< 40%` target should carry one nearer to it, is a band question on bot evidence — the call [D36](D36-lease-rate-chokepoint-gate.md) and D37 both declined to make. It stays with M5.5, recorded in `#241` rather than decided here.

### 7. Why not B

Option B has option A's object error and adds a second one. A diagnostic printed under R9's name that measures something else is worse than the absence it replaces, because the next reader stops looking — which is `#233`'s own argument against B, applied to the metric B would actually print.

### 8. And no, a headless *proxy for usage* is not hiding in the event stream either

The obvious next idea is to approximate "you never need to read the trail" without a UI — some measure of how much of what the Board would have told a player was already known to them from direct contact. It is refused here for the same reason D33 refused row 18: **nothing in the ruleset labels an inference as needed or not needed.** Whether a player *would have* consulted the Log is a fact about that player's uncertainty, and the closest a match record gets to it is a chain of assumptions about what a hypothetical player would have done with information they were handed. D33's standing rule on exactly this shape — *"flagging it here rather than pretending a formula exists"* — applies unchanged, and a number built on that chain would be far more confident-looking than anything it could support. If someone does find a defensible formulation later, it is a new §22 row with its own decision, not a reinterpretation of R9's pointer.

### 9. Why this is not option C wearing different words

C's disposition is "no computed row exists; say so in §22." That is right about `internal/telemetry` and wrong about §22, where the rows exist, are numbered, and are scheduled. Writing "unimplemented" against a scheduled row is how a row gets built twice, or gets dropped as already-declined when M5 arrives. The difference between C and D is not tone: C leaves R9's safeguard unowned, and D hands it to the milestone that can actually build it.

## Consequences

- **GDD moves to v2.28.** §20's R9 sentence and §22's R9 row name rows 15 and 16 and say they are M5/M5.5 instrumentation; rows 15 and 16 carry the R9 back-reference; row 19 is named as the headless guardrail on R9's remedy, alongside row 8. No band, no threshold and no rule changes anywhere — this decision adds cross-references and changes no number.
- **§22's per-match table gains a row-number column**, which it needed the moment this decision started citing rows by number. D33's audit, `internal/telemetry`'s field comments and four decision documents all cite "§22 row N" against a table that never printed N — a reader had to count. The numbers are D33's own, in D33's own order, and §22 now states they are **append-only**: inserting a row silently moves every existing citation.
- **Companion RFC moves to r34, pointer only.** §17's "one computation, three sinks" and §16.4's harness description are unaffected: nothing about which rows are headless changes here.
- **No behaviour changes; three comments move.** `MatchSummary` gains no field, `internal/rules/gdd22_metrics_test.go`'s twenty-row table is untouched, `cmd/simulate`'s CSV gains no column, and no new event is emitted. What changes is where the distinction has to be legible from the code: `internal/game/config.go`'s `MapByPlayers` comment currently calls rows 8 and 19 "the 'Board goes unused' failure R9 names as the thing to watch", which is the conflation this decision resolves; `internal/telemetry`'s package comment now says why rows 15/16 will never have a field here (a bot opens no Heat Map); and `MatchSummary`'s row 8 and row 19 field comments name the pair as R9's remedy guardrail, row 8's carrying the asymmetry D37 ran into — its only stated action is on the high side, so a map raised far enough leaves the band without tripping anything.
- **D37's own §22 row numbers are corrected, in this decision's PR.** Its observation-coverage table and the two paragraphs reading it call Heat Map entries at low confidence "row 14" in four places; row 14 is the Convergence `[C]` metric and the Heat Map row is 19, per D33's numbering, `internal/rules/gdd22_metrics_test.go` and `MatchSummary`'s own field comments. D37's numbers, bands and conclusion are unaffected — it quotes row 19's real `< 40%` target and `> 60%` failing line throughout, so only the label was wrong. Found by review against the numbered table this PR adds, which is the table earning its keep on the first read.
- **D33's audit stands unamended.** Row 15 → M5.5 and row 16 → M5 were the right calls. What was missing was the R9 cross-reference, which §22 now carries.
- **D37 is not superseded and its verdict is unchanged.** Its measurement, its 46-node crossing, and its deferral all stand; rows 8 and 19 were the right rows to read for the question it was asking. Two of its sentences — that `#233` exists to fix the missing row, and that a future revisit "should have the real row" — get a forward pointer to this document, per the decision log's own "leave the original standing with a pointer forward" convention.
- **[#241](https://github.com/garnizeh/cinzal/issues/241) (the M5.5 hand-off) is updated.** Its caveat currently reads *"If #233 lands before M5.5, this row gets a better read than D37 could give it."* That is now known to be false: the better read arrives with M5's Heat Map instrumentation and M5.5's own attribution logging — rows 15 and 16 — and #241 carries them as the read to take when the 5-player R9 row is revisited.
- **What would reopen this.** A future node-count change wanting a direct map-utilization number should file it **under its own name**, with its own band and its own §22 row — the measurement in §5 above is its baseline and its warning that the band is not obvious. Separately, if M5's instrumentation turns out unable to collect rows 15/16 at all, R9 loses its indicator for real, and that is the point at which inventing a headless substitute becomes the least-bad option rather than the wrong object.
- **Reversible at no cost.** Documentation and one code comment. Nothing in `internal/rules`, `internal/game`'s behaviour, or `internal/telemetry` changes.
