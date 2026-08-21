# D39 — R1 trips at every player count: how does GDD §15's confrontation rule get softened?

**Status:** decided
**Blocks:** the GDD §15/§22 and `internal/rules`/`internal/telemetry` tasks this hands forward
**Decided:** 2026-08-21
**Issue:** [#245](https://github.com/garnizeh/cinzal/issues/245)

## The question

GDD §20's R1 carries the only threshold in M2's exit criteria that both tripped and has a written remedy behind it:

> **R1 — Cancelled routes may frustrate.** You plan four steps, take a confrontation on step one, and lose everything after it. If that's common, the game reads as a lottery. v1 ships the simple rule; §22 measures it. **Threshold: if more than 15% of submitted routes are cancelled mid-route, the confrontation rule gets softened** — most likely by letting the loser continue their route from the fallback node with remaining steps intact.

[#205](https://github.com/garnizeh/cinzal/issues/205) measured it and it tripped everywhere: 30.06% / 40.51% / 45.70% / 56.36% at 2/3/4/5 players (Operator, 20,000 matches per cell), against a 15% line, on both tiers, in every round from 3 to 15, in 83.6%–100.0% of individual matches.

[#245](https://github.com/garnizeh/cinzal/issues/245) filed the design work that verdict hands forward, and named three things that have to be settled together: **the remedy's exact form** (§20 says *"most likely by"*, not *"by"*), **what the numerator actually contains** (`EventRouteHalted` fires for a tie participant, a decisive loser, and the rare crossing-corrected winner, and carries no cause field), and **how the remedy interacts with §22 row 9's Evasive band** and rows 17 and 20.

## Why it is open

- **§20's remedy is hedged, and it is a §15 rule change.** Letting the loser continue changes what a confrontation is worth, which is the same lever R11 (endgame camping) and §9.3 (Evasive as default insurance) sit on. Whether it applies whole, partially, or only to decisive losses is not something §20 decides.
- **The remedy is written for the loser, and the numerator is not only losers.** §20's sentence leaves a tie participant's halted route untouched, and nothing measured before this decision could say how large that share is.
- **R9's own remedy has already been spent here.** [#234](https://github.com/garnizeh/cinzal/pull/234) raised the 4-player map from 25 to 28 nodes; R1 at 4 players moved 49.24% → 45.70% against a line 30 points below. §20 already points R1 at the confrontation rule rather than at the map, and `#205` confirmed it.

## Method

D35's rigor throughout: **10,000 matches per configuration**, `mean ± 1.96·s/√n` over the per-match vector, **two independently drawn root seeds** — `#205`'s own two — pooled into one 20,000-match interval before any verdict is read. `game.DefaultConfig()`, unmodified, at 2/3/4/5 players. R1's verdict tier is **Operator**; Drifter is reported alongside, per D35.

**112 configurations in total**: every option below against Operator (8 × 4 player counts × 2 seeds), and the shipped rule plus A, A′, D, E and E+ against Drifter (6 × 4 × 2). B, B′ and C are Operator-only — they are rejected on Operator's own numbers before Drifter could break the tie, and neither is the adopted rule; the adopted one and the two it is measured against are reported on both tiers.

Git SHA `1d3948c`, plus a temporary probe that is **not** part of this decision's diff: a cause field and a remaining-steps pair on `EventRouteHalted`, the movement step index threaded into `resolveConfrontations`, and each candidate rule behind a switch — built, run, and reverted, the shape [D37](D37-five-player-confrontation-load.md) and [D38](D38-board-going-unused-indicator.md) established for a decision that has to measure a rule it has not yet decided to ship.

**The probe reproduces `#205` exactly.** At 4 players, Operator, root seed 1, with every candidate rule switched off, it returns `RoutesCancelledMidRoute` = `0.455350 ± 0.002572`, `ConfrontationsPerMatch` = `17.952400`, and `ConfrontationsWonAgainstEvasiveLoser` = `0.455799` — the same figures to six decimal places as `docs/exit-demos/205/p4-operator-seed1.csv`. That is the check that this decision measured the same matches the demonstration measured.

## What the sweep found

### 1. The split: §20's remedy is written for about seven tenths of the numerator

`EventRouteHalted` fires at three call sites — `resolveTie`, `resolveLoser`, and `resolveDecisive`'s crossing-corrected winner — and carries no cause, which is why `#205` could not split it. Tagged and re-run:

| Players | Decisive loser | Tie participant | Crossing-corrected winner |
|---|---|---|---|
| 2 | 72.18% [71.88%, 72.47%] | 21.00% [20.71%, 21.29%] | 6.82% [6.69%, 6.95%] |
| 3 | 70.37% [70.17%, 70.57%] | 22.83% [22.62%, 23.03%] | 6.81% [6.71%, 6.90%] |
| 4 | 70.15% [69.98%, 70.31%] | 23.36% [23.20%, 23.53%] | 6.49% [6.42%, 6.57%] |
| 5 | 70.00% [69.87%, 70.13%] | 23.87% [23.73%, 24.00%] | 6.14% [6.08%, 6.19%] |

Two things follow. §20's sentence names **the loser**, and the loser is the large majority of the numerator, so the remedy is aimed at the right place. But the tie share is not a rounding error, and **GDD §15 never says a tie loses its round**: its tie paragraph is *"everyone falls back to the node they came from, nobody loses cargo, stakes are returned. Ties are anticlimactic by design — it makes speculative aggression a worse bet."* The halt is `internal/rules`' own addition, and `resolveTie`'s doc comment says so in as many words — *"the GDD does not restate that for a tie the way it does for a Loser, but it is an implementation necessity."* It is a necessity of the movement loop, not a design decision anybody took, and forfeiting the round is the least anticlimactic outcome the game has.

**A smaller correction on the way past.** [D33](D33-telemetry-event-stream-coverage.md)'s row 1 audit describes the halt as firing on *"every confrontation loser and on a winner whose own position was corrected"* — two of the three call sites. The tie participants are missing from that line, and they are the share above. `EventRouteHalted`'s own doc comment in `internal/game/event.go` lists all three correctly, so the code is right and the audit line is incomplete.

### 2. Row 1 as instrumented is not a share of submitted routes

D33 settled row 1's numerator as `len(EventRouteHalted)` and its denominator as an order-log count of submitted non-empty routes. Those are different units, and the gap is not theoretical:

| Players | halts | on a seat that submitted no route | repeat halt, same round+seat | first halt, no step left | a submitted route cut short |
|---|---|---|---|---|---|
| 2 | 164014 | 9166 (5.6%) | 10075 (6.1%) | 49933 (30.4%) | 94840 (57.8%) |
| 3 | 344944 | 11527 (3.3%) | 39421 (11.4%) | 100809 (29.2%) | 193187 (56.0%) |
| 4 | 519680 | 16537 (3.2%) | 72731 (14.0%) | 146507 (28.2%) | 283905 (54.6%) |
| 5 | 796396 | 27107 (3.4%) | 132839 (16.7%) | 224314 (28.2%) | 412136 (51.8%) |

- **A halt can land on a seat that submitted no route at all.** A stationary seat is still a seat at a node, and GDD §15's collision rule evaluates every player's position after each step *"whether or not they moved."* Nothing was cancelled; the numerator counts it anyway, and the denominator by construction excludes it.
- **One route can be halted more than once in a round.** A seat that has stopped moving can be caught again by somebody else's later movement step — RFC §6.5's own Bounty worked case says exactly that: *"a pushed loser is caught again by someone else's later movement step."* Each catch fires another event against a route that was already gone.
- **A halt on the last step cancels no remaining step.** GDD §15 is deliberate that this still costs the same — *"the 2-node pushback and −1 step next round are position-independent, so being caught on your last step now costs exactly as much as being caught on your first"* — and that is a statement about the **penalty**, not about the word "mid-route". Row 1 counts it as a cancelled route; R1's own text (*"take a confrontation on step one, and lose everything after it"*) is about the steps that were lost.

**The three together mean the quantity is not a proportion, and it does not stay under 1 in practice.** The largest per-match value in this sweep is **1.231884** at 5 players and **1.105263** at 3 (Operator, over 20,000 matches per cell) — matches in which row 1 reports that 123% of the submitted routes were cancelled. That is not a rounding artefact or a pathological seed; it is what counting one thing against a denominator of a different thing does. A number that can exceed its own denominator cannot be read against a *"< 15% of submitted routes"* threshold, whatever value it happens to take.

**D33 already applied the correct discipline one row over.** Its row 10 audit refuses raw event counting for exactly this reason: *"Count distinct `(Round, Node)` pairs among `EventConfrontation` entries, not raw event count: a K-way tie emits K−1 events for one confrontation, and a multi-loser decisive result emits one event per loser — raw counting overcounts either shape."* Row 1 needed the same sentence and did not get it.

**The reading this decision adopts**, and how the shipped rule scores under both:

> Row 1's numerator is the number of **distinct (round, seat) pairs that submitted a non-empty route and whose first halt that round left at least one step of their declared plan — route or Pushing On — unspent.** The denominator is unchanged: every order-log entry whose `Route` has at least one step.

That is three corrections in one sentence, and each is the row's own words rather than a new idea: *routes*, not events; *submitted* routes, so a seat that declared none is out of the numerator as it is already out of the denominator; and *mid*-route, so a plan that had already run out has not been cut short by being stopped.

| Players | as instrumented (Operator) | routes cut mid-route (Operator) | as instrumented (Drifter) | routes cut mid-route (Drifter) |
|---|---|---|---|---|
| 2 | 30.06% [29.85%, 30.26%] | 17.32% [17.18%, 17.46%] | 18.95% [18.80%, 19.09%] | 12.51% [12.40%, 12.62%] |
| 3 | 40.51% [40.32%, 40.70%] | 22.64% [22.52%, 22.76%] | 24.32% [24.18%, 24.46%] | 15.66% [15.56%, 15.76%] |
| 4 | 45.70% [45.52%, 45.88%] | 24.93% [24.82%, 25.03%] | 26.74% [26.61%, 26.87%] | 16.88% [16.79%, 16.97%] |
| 5 | 56.36% [56.19%, 56.54%] | 29.13% [29.03%, 29.23%] | 36.18% [36.05%, 36.31%] | 22.30% [22.22%, 22.39%] |

Two things to read off that table beyond the verdict. **The left-hand column reproduces `#205`'s published pooled figures exactly** — 30.06 / 40.51 / 45.70 / 56.36 for Operator and 18.95 / 24.32 / 26.74 / 36.18 for Drifter — which is what says the corrected column was computed over the same matches and not a different sweep. And **Drifter's 2-player cell flips**, from 18.95% to 12.51%, the one cell in the table where the two readings disagree about the verdict. R1 is read against Operator ([D35](D35-simulation-sample-size-and-verdict-rule.md) §3.3, roadmap exit criteria), so it does not disturb anything, but it is the honest illustration of how much the unit was carrying.

**The verdict `#205` recorded does not move.** Under the corrected reading the shipped rule still fails the 15% line at every player count, on both tiers, with the whole interval on the failing side. Fixing the numerator changes the magnitude of the problem, not its existence — which is the only reason it is safe to fix it inside the same decision that chooses the remedy.

### 3. §20's remedy, made mechanical, walks the loser back into the fight

Under option A the loser's resumed plan starts by retracing the hops they were just pushed back over, because that is the only path from the fallback node to the declared tail that walks real edges. That path goes through the confrontation node.

| Option | Players | continued | not continued | re-entered the fight node |
|---|---|---|---|---|
| A — §20 as written | 2 | 128,772 (45.4%) | 154,834 | 115,346 (**89.6%** of continued) |
| A — §20 as written | 3 | 280,762 (46.0%) | 329,738 | 251,174 (**89.5%**) |
| A — §20 as written | 4 | 424,366 (45.9%) | 499,944 | 378,834 (**89.3%**) |
| A — §20 as written | 5 | 632,174 (44.4%) | 791,666 | 568,780 (**90.0%**) |
| B — A, ties too | 4 | 292,563 (43.0%) | 388,529 | 269,516 (**92.1%**) |
| D/E/E+ — blind continuation | 4 | 177,402–255,289 (51.7%–52.7%) | — | **0** |

The "not continued" column is not a defect: it is mostly seats whose plan had already run out, the same population as the *"first halt, no step left"* column above. Roughly 45% of the seats §20's remedy is written for have something left to continue with; the rest are the halts that were never cancelling anything.

**GDD §15 has already ruled on this shape once, in the paragraph directly above §20's remedy.** The two-hop pushback for a stationary Evasive loser carries a clause whose whole purpose is to stop the loser coming back:

> The two-hop rule for a stationary Evasive loser needs the "must not re-enter the confrontation node" clause specifically, or the walk can go C → D → C and deposit the camper back on the contested chokepoint — turning a penalty into a free round of holding position. **Without it the rule is worse than no rule.**

The analogy is not exact and this decision does not lean on it: under option A the return is paid for in steps, and a pushback hop is free. What the measurement adds is that paying for it does not stop the outcome §15 was worried about from happening — it happens to the large majority of continued losers, and the arithmetic that made option A look sufficient does not survive it:

| Players | as instrumented: arithmetic | measured | routes cut mid-route: arithmetic | measured |
|---|---|---|---|---|
| 2 | 9.51% | **26.35%** | 5.57% | **8.38%** |
| 3 | 12.61% | **35.73%** | 7.13% | **11.14%** |
| 4 | 14.06% | **40.36%** | 7.68% | **12.13%** |
| 5 | 17.24% | **50.58%** | 8.87% | **14.21%** |

The "arithmetic" columns are `#245`'s own reasoning carried out exactly — subtract the decisive-loser share from the numerator and leave the rest of the match alone. It is out by a factor of **2.9** on the instrumented reading and by **1.5**–**1.6** on the corrected one, in the same direction every time: **a loser who keeps walking gets caught again.** That is the second-order effect the split alone cannot show, and it is why `#245`'s step 1 was necessary but not sufficient.

### 4. Every option, measured

**Row 1 as currently instrumented.** No candidate clears 15% above 2 players, and only the two most generous clear it there:

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 30.06% [29.85%, 30.26%] | 40.51% [40.32%, 40.70%] | 45.70% [45.52%, 45.88%] | 56.36% [56.19%, 56.54%] |
| A — loser resumes, keeps the action | 26.35% [26.15%, 26.55%] | 35.73% [35.53%, 35.92%] | 40.36% [40.18%, 40.55%] | 50.58% [50.39%, 50.76%] |
| A′ — A, action still forfeit | 26.36% [26.16%, 26.56%] | 35.75% [35.56%, 35.94%] | 40.38% [40.20%, 40.56%] | 50.50% [50.32%, 50.68%] |
| B — A, ties too | 25.11% [24.92%, 25.30%] | 34.12% [33.93%, 34.31%] | 38.63% [38.45%, 38.81%] | 48.65% [48.47%, 48.84%] |
| B′ — A′, ties too | 25.13% [24.94%, 25.32%] | 34.12% [33.94%, 34.31%] | 38.55% [38.37%, 38.73%] | 48.57% [48.39%, 48.76%] |
| C — A on half the remaining steps | 27.12% [26.93%, 27.32%] | 36.80% [36.61%, 36.99%] | 41.26% [41.08%, 41.43%] | 51.38% [51.20%, 51.56%] |
| D — blind continuation, no re-entry | 17.06% [16.91%, 17.21%] | 23.42% [23.27%, 23.57%] | 26.97% [26.82%, 27.11%] | 34.82% [34.67%, 34.97%] |
| E — D, ties too | 13.66% [13.53%, 13.80%] | 18.88% [18.75%, 19.01%] | 21.90% [21.77%, 22.02%] | 28.49% [28.36%, 28.62%] |
| E+ — E, crossing-corrected winner too | 12.48% [12.35%, 12.60%] | 17.29% [17.16%, 17.41%] | 20.07% [19.95%, 20.19%] | 26.48% [26.35%, 26.61%] |

**Row 1 counting routes**, the reading this decision adopts:

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 17.32% [17.18%, 17.46%] | 22.64% [22.52%, 22.76%] | 24.93% [24.82%, 25.03%] | 29.13% [29.03%, 29.23%] |
| A — loser resumes, keeps the action | 8.38% [8.28%, 8.48%] | 11.14% [11.05%, 11.22%] | 12.13% [12.05%, 12.21%] | 14.21% [14.14%, 14.28%] |
| A′ — A, action still forfeit | 8.37% [8.28%, 8.47%] | 11.09% [11.01%, 11.18%] | 12.11% [12.04%, 12.19%] | 14.17% [14.10%, 14.24%] |
| B — A, ties too | 4.73% [4.67%, 4.80%] | 6.15% [6.09%, 6.20%] | 6.51% [6.46%, 6.56%] | 7.54% [7.49%, 7.58%] |
| B′ — A′, ties too | 4.73% [4.67%, 4.80%] | 6.12% [6.06%, 6.18%] | 6.48% [6.43%, 6.53%] | 7.50% [7.45%, 7.54%] |
| C — A on half the remaining steps | 11.23% [11.12%, 11.34%] | 14.98% [14.88%, 15.08%] | 16.32% [16.23%, 16.41%] | 19.20% [19.12%, 19.29%] |
| D — blind continuation, no re-entry | 6.04% [5.96%, 6.13%] | 8.64% [8.56%, 8.71%] | 9.75% [9.68%, 9.82%] | 12.01% [11.94%, 12.08%] |
| E — D, ties too | 2.73% [2.69%, 2.78%] | 4.25% [4.21%, 4.30%] | 4.97% [4.93%, 5.01%] | 6.41% [6.37%, 6.46%] |
| E+ — E, crossing-corrected winner too | 1.65% [1.62%, 1.69%] | 2.85% [2.81%, 2.88%] | 3.51% [3.47%, 3.54%] | 4.75% [4.71%, 4.78%] |

**Drifter**, reported per D35 — not the verdict tier, and every option lands in the same order it does under Operator:

| Option | metric | 2p | 3p | 4p | 5p |
|---|---|---|---|---|---|
| shipped GDD §15 | as instrumented | 18.95% [18.80%, 19.09%] | 24.32% [24.18%, 24.46%] | 26.74% [26.61%, 26.87%] | 36.18% [36.05%, 36.31%] |
| shipped GDD §15 | cut mid-route | 12.51% [12.40%, 12.62%] | 15.66% [15.56%, 15.76%] | 16.88% [16.79%, 16.97%] | 22.30% [22.22%, 22.39%] |
| A — loser resumes, keeps the action | as instrumented | 15.19% [15.06%, 15.33%] | 20.23% [20.10%, 20.36%] | 22.40% [22.27%, 22.52%] | 30.51% [30.39%, 30.64%] |
| A — loser resumes, keeps the action | cut mid-route | 5.38% [5.31%, 5.46%] | 6.96% [6.89%, 7.03%] | 7.64% [7.58%, 7.70%] | 10.03% [9.97%, 10.09%] |
| A′ — A, action still forfeit | as instrumented | 15.23% [15.09%, 15.37%] | 20.18% [20.05%, 20.31%] | 22.36% [22.23%, 22.48%] | 30.47% [30.34%, 30.59%] |
| A′ — A, action still forfeit | cut mid-route | 5.39% [5.32%, 5.46%] | 6.95% [6.88%, 7.01%] | 7.61% [7.55%, 7.67%] | 10.04% [9.98%, 10.10%] |
| D — blind continuation, no re-entry | as instrumented | 9.63% [9.53%, 9.73%] | 12.77% [12.67%, 12.86%] | 14.46% [14.37%, 14.55%] | 20.03% [19.94%, 20.13%] |
| D — blind continuation, no re-entry | cut mid-route | 4.35% [4.29%, 4.42%] | 5.86% [5.80%, 5.92%] | 6.65% [6.59%, 6.71%] | 9.28% [9.22%, 9.34%] |
| E — D, ties too | as instrumented | 7.38% [7.30%, 7.46%] | 9.84% [9.76%, 9.92%] | 11.27% [11.20%, 11.35%] | 15.83% [15.75%, 15.91%] |
| E — D, ties too | cut mid-route | 2.10% [2.06%, 2.14%] | 3.09% [3.05%, 3.13%] | 3.64% [3.61%, 3.68%] | 5.39% [5.35%, 5.43%] |
| E+ — E, crossing-corrected winner too | as instrumented | 6.28% [6.21%, 6.36%] | 8.42% [8.35%, 8.49%] | 9.67% [9.61%, 9.74%] | 13.68% [13.61%, 13.75%] |
| E+ — E, crossing-corrected winner too | cut mid-route | 0.99% [0.96%, 1.02%] | 1.71% [1.68%, 1.74%] | 2.16% [2.14%, 2.19%] | 3.46% [3.43%, 3.49%] |

Drifter's own verdicts differ from Operator's in exactly one place, and it is worth stating rather than burying: on the corrected count the **shipped** rule already passes at 2 players under Drifter (12.51%) and fails at 3, 4 and 5. R1 is read against Operator ([D35](D35-simulation-sample-size-and-verdict-rule.md) §3.3), so this changes no verdict — but it is the tier difference D35 asks to be reported, and it is the expected direction: a bot with no plan has less plan to lose.

**C is closed outright.** At half the remaining steps the corrected rate is 16.32% [16.23%, 16.41%] at 4 players and 19.20% [19.12%, 19.29%] at 5 — both intervals entirely above the line — and at 3 players it straddles it, 14.98% [14.88%, 15.08%], which under D35 is not even a pass. Halving the steps roughly halves the benefit, and the benefit was not large enough to halve.

### 5. Nothing else in §22 moves against any of them

Every band, every option, pooled. The rows that could plausibly have moved against a softened loss are 9 (Evasive as default insurance, §9.3), 17 (Loitering, R11's own instrument) and 20 (endgame farming, R11):

**row 9 — confrontations won against an Evasive loser (20-40%, fails > 55%)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 43.41% [43.04%, 43.78%] | 44.11% [43.84%, 44.38%] | 45.50% [45.27%, 45.72%] | 44.67% [44.49%, 44.85%] |
| A — loser resumes, keeps the action | 45.83% [45.45%, 46.20%] | 45.16% [44.88%, 45.43%] | 46.28% [46.05%, 46.50%] | 44.84% [44.66%, 45.03%] |
| A′ — A, action still forfeit | 46.24% [45.86%, 46.62%] | 45.67% [45.39%, 45.94%] | 46.55% [46.33%, 46.78%] | 45.06% [44.87%, 45.24%] |
| B — A, ties too | 50.73% [50.34%, 51.11%] | 50.20% [49.92%, 50.48%] | 51.05% [50.82%, 51.28%] | 49.24% [49.05%, 49.43%] |
| B′ — A′, ties too | 51.21% [50.82%, 51.60%] | 50.74% [50.46%, 51.02%] | 51.47% [51.24%, 51.70%] | 49.55% [49.36%, 49.73%] |
| C — A on half the remaining steps | 45.00% [44.62%, 45.37%] | 45.02% [44.75%, 45.29%] | 46.10% [45.87%, 46.32%] | 44.98% [44.80%, 45.17%] |
| D — blind continuation, no re-entry | 43.53% [43.15%, 43.90%] | 44.00% [43.73%, 44.26%] | 45.69% [45.47%, 45.91%] | 44.68% [44.50%, 44.87%] |
| E — D, ties too | 43.52% [43.15%, 43.90%] | 44.01% [43.74%, 44.28%] | 45.57% [45.34%, 45.79%] | 44.86% [44.68%, 45.04%] |
| E+ — E, crossing-corrected winner too | 43.58% [43.21%, 43.96%] | 43.94% [43.67%, 44.21%] | 45.73% [45.50%, 45.95%] | 44.84% [44.66%, 45.02%] |

**row 10/11 — confrontations per match (4-12, fails > 12)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 6.5629 [6.5214, 6.6044] | 12.4927 [12.4445, 12.5410] | 18.0297 [17.9744, 18.0851] | 26.3099 [26.2502, 26.3696] |
| A — loser resumes, keeps the action | 6.3292 [6.2901, 6.3682] | 12.0183 [11.9723, 12.0644] | 17.3398 [17.2869, 17.3928] | 25.2523 [25.1950, 25.3097] |
| A′ — A, action still forfeit | 6.3228 [6.2839, 6.3618] | 12.0133 [11.9674, 12.0592] | 17.3278 [17.2749, 17.3806] | 25.2014 [25.1440, 25.2587] |
| B — A, ties too | 6.2152 [6.1766, 6.2537] | 11.8351 [11.7894, 11.8807] | 17.0983 [17.0458, 17.1507] | 24.9697 [24.9120, 25.0273] |
| B′ — A′, ties too | 6.2102 [6.1718, 6.2486] | 11.8241 [11.7785, 11.8698] | 17.0689 [17.0165, 17.1212] | 24.9253 [24.8677, 24.9829] |
| C — A on half the remaining steps | 6.3591 [6.3196, 6.3987] | 12.0647 [12.0182, 12.1111] | 17.3383 [17.2850, 17.3917] | 25.2658 [25.2080, 25.3235] |
| D — blind continuation, no re-entry | 5.8167 [5.7810, 5.8525] | 11.5772 [11.5319, 11.6226] | 17.0939 [17.0402, 17.1477] | 25.5958 [25.5360, 25.6557] |
| E — D, ties too | 5.7284 [5.6931, 5.7638] | 11.5451 [11.4993, 11.5909] | 17.1889 [17.1342, 17.2436] | 25.8286 [25.7674, 25.8898] |
| E+ — E, crossing-corrected winner too | 5.7465 [5.7110, 5.7819] | 11.6594 [11.6130, 11.7059] | 17.3502 [17.2947, 17.4058] | 26.1498 [26.0878, 26.2118] |

**row 17 — rounds flagged Loitering (< 8%, fails > 15%)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 6.06% [5.97%, 6.15%] | 7.22% [7.14%, 7.30%] | 7.20% [7.13%, 7.27%] | 8.46% [8.40%, 8.53%] |
| A — loser resumes, keeps the action | 5.33% [5.25%, 5.41%] | 6.56% [6.48%, 6.64%] | 6.44% [6.37%, 6.50%] | 7.67% [7.61%, 7.73%] |
| A′ — A, action still forfeit | 5.39% [5.30%, 5.47%] | 6.62% [6.54%, 6.70%] | 6.50% [6.43%, 6.56%] | 7.72% [7.65%, 7.78%] |
| B — A, ties too | 4.96% [4.88%, 5.04%] | 5.95% [5.88%, 6.02%] | 5.85% [5.79%, 5.91%] | 7.02% [6.95%, 7.08%] |
| B′ — A′, ties too | 5.01% [4.93%, 5.09%] | 6.04% [5.97%, 6.11%] | 5.92% [5.85%, 5.98%] | 7.07% [7.01%, 7.13%] |
| C — A on half the remaining steps | 5.89% [5.80%, 5.98%] | 7.32% [7.24%, 7.40%] | 7.18% [7.11%, 7.25%] | 8.46% [8.39%, 8.53%] |
| D — blind continuation, no re-entry | 4.42% [4.34%, 4.49%] | 4.93% [4.87%, 5.00%] | 4.88% [4.82%, 4.93%] | 5.80% [5.75%, 5.86%] |
| E — D, ties too | 4.03% [3.96%, 4.10%] | 4.39% [4.33%, 4.45%] | 4.31% [4.26%, 4.36%] | 5.16% [5.11%, 5.21%] |
| E+ — E, crossing-corrected winner too | 3.92% [3.85%, 3.98%] | 4.24% [4.18%, 4.30%] | 4.17% [4.12%, 4.22%] | 4.98% [4.93%, 5.02%] |

**row 20 — confrontations in the final 3 rounds (< 30%, fails > 45%)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 20.96% [20.68%, 21.23%] | 21.05% [20.90%, 21.21%] | 21.55% [21.43%, 21.66%] | 20.47% [20.39%, 20.55%] |
| A — loser resumes, keeps the action | 20.51% [20.24%, 20.79%] | 20.68% [20.53%, 20.84%] | 21.15% [21.04%, 21.27%] | 20.40% [20.31%, 20.48%] |
| A′ — A, action still forfeit | 20.54% [20.27%, 20.81%] | 20.72% [20.56%, 20.87%] | 21.15% [21.03%, 21.26%] | 20.42% [20.33%, 20.50%] |
| B — A, ties too | 20.10% [19.83%, 20.37%] | 20.29% [20.14%, 20.45%] | 20.91% [20.79%, 21.02%] | 20.18% [20.09%, 20.26%] |
| B′ — A′, ties too | 20.11% [19.84%, 20.38%] | 20.32% [20.16%, 20.47%] | 20.88% [20.76%, 21.00%] | 20.16% [20.07%, 20.24%] |
| C — A on half the remaining steps | 20.83% [20.56%, 21.11%] | 20.96% [20.81%, 21.11%] | 21.26% [21.15%, 21.38%] | 20.58% [20.50%, 20.66%] |
| D — blind continuation, no re-entry | 20.47% [20.19%, 20.74%] | 20.74% [20.59%, 20.90%] | 21.32% [21.19%, 21.44%] | 20.54% [20.45%, 20.63%] |
| E — D, ties too | 20.28% [20.01%, 20.56%] | 20.45% [20.29%, 20.61%] | 21.22% [21.10%, 21.35%] | 20.30% [20.21%, 20.39%] |
| E+ — E, crossing-corrected winner too | 20.16% [19.89%, 20.44%] | 20.46% [20.30%, 20.63%] | 21.12% [20.99%, 21.24%] | 20.26% [20.17%, 20.35%] |

**row 2 — deliveries per player (4-6, fails < 3)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 1.3156 [1.3049, 1.3263] | 1.0581 [1.0497, 1.0666] | 0.8138 [0.8075, 0.8201] | 0.6488 [0.6437, 0.6539] |
| A — loser resumes, keeps the action | 1.3453 [1.3345, 1.3561] | 1.0808 [1.0721, 1.0894] | 0.8336 [0.8272, 0.8401] | 0.6683 [0.6632, 0.6734] |
| A′ — A, action still forfeit | 1.3111 [1.3005, 1.3216] | 1.0466 [1.0382, 1.0550] | 0.8080 [0.8017, 0.8143] | 0.6432 [0.6383, 0.6482] |
| B — A, ties too | 1.3783 [1.3675, 1.3891] | 1.1039 [1.0952, 1.1126] | 0.8544 [0.8478, 0.8609] | 0.6872 [0.6820, 0.6924] |
| B′ — A′, ties too | 1.3391 [1.3285, 1.3497] | 1.0649 [1.0564, 1.0733] | 0.8247 [0.8183, 0.8310] | 0.6581 [0.6531, 0.6631] |
| C — A on half the remaining steps | 1.3283 [1.3176, 1.3390] | 1.0550 [1.0466, 1.0634] | 0.8205 [0.8141, 0.8269] | 0.6530 [0.6479, 0.6581] |
| D — blind continuation, no re-entry | 1.4607 [1.4502, 1.4713] | 1.2045 [1.1959, 1.2130] | 0.9404 [0.9339, 0.9470] | 0.7601 [0.7548, 0.7653] |
| E — D, ties too | 1.4973 [1.4868, 1.5079] | 1.2437 [1.2350, 1.2523] | 0.9658 [0.9592, 0.9724] | 0.7860 [0.7807, 0.7914] |
| E+ — E, crossing-corrected winner too | 1.5072 [1.4968, 1.5177] | 1.2546 [1.2460, 1.2631] | 0.9769 [0.9703, 0.9836] | 0.7938 [0.7885, 0.7992] |

**row 3 — winner's RP lead over last place (< 40%)**

| Option | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| shipped GDD §15 | 83.63% [82.80%, 84.47%] | 118.06% [117.23%, 118.90%] | 137.35% [136.50%, 138.20%] | 155.02% [154.03%, 156.02%] |
| A — loser resumes, keeps the action | 83.98% [83.13%, 84.83%] | 119.14% [118.31%, 119.97%] | 138.54% [137.66%, 139.42%] | 154.39% [153.41%, 155.37%] |
| A′ — A, action still forfeit | 85.41% [84.56%, 86.26%] | 120.70% [119.87%, 121.54%] | 140.20% [139.30%, 141.09%] | 156.73% [155.73%, 157.74%] |
| B — A, ties too | 83.28% [82.44%, 84.11%] | 119.49% [118.65%, 120.33%] | 138.94% [138.06%, 139.82%] | 155.13% [154.13%, 156.13%] |
| B′ — A′, ties too | 84.78% [83.94%, 85.61%] | 121.19% [120.33%, 122.05%] | 140.59% [139.69%, 141.49%] | 157.42% [156.39%, 158.45%] |
| C — A on half the remaining steps | 84.20% [83.36%, 85.05%] | 120.46% [119.60%, 121.31%] | 138.42% [137.55%, 139.29%] | 155.19% [154.21%, 156.17%] |
| D — blind continuation, no re-entry | 76.09% [75.33%, 76.86%] | 108.80% [108.08%, 109.52%] | 127.16% [126.45%, 127.88%] | 143.11% [142.30%, 143.92%] |
| E — D, ties too | 74.43% [73.69%, 75.17%] | 106.94% [106.24%, 107.65%] | 125.29% [124.60%, 125.98%] | 140.79% [140.03%, 141.55%] |
| E+ — E, crossing-corrected winner too | 73.86% [73.14%, 74.59%] | 106.10% [105.41%, 106.79%] | 124.21% [123.53%, 124.90%] | 140.19% [139.42%, 140.96%] |

## Options

Every option below except F and G was implemented and run at full rigor, not reasoned about. §20's sentence does not survive contact with the movement loop unamended, so "implemented" means a specific mechanical reading, stated with each.

**A — §20 as written: the decisive loser continues their declared route from the fallback node, with their remaining steps and their action.** The declared tail is not adjacent to the fallback node, so "continue" has to be given a mechanism: the loser retraces the one or two hops they were just pushed back over, then resumes the declared tail, the whole thing truncated to the step budget they had left. Total steps for the round are unchanged — the ground is made back up out of them, which is what *"remaining steps intact"* costs in practice. (A crossing resolves at the Aggressive party's origin, GDD §15a, which for one of the two is not the node their own step was heading for; their own destination is re-inserted between the retrace and the tail, or the plan would walk an edge that does not exist.)

**A′ — A, but the action stays forfeit.** The minimal reading of §20: §15's Loser bullet couples two losses in one clause — *"loses the remainder of their route and their action"* — and §20's sentence replaces only the route half.

**B — A, extended to tie participants.** GDD §15 never says a tie loses its round; `resolveTie`'s own doc comment records the halt as an implementation necessity rather than a rule.

**B′ — A′, extended to tie participants.**

**C — A, on half the remaining steps** (rounded down; a loser with one step left is not continued). `#245`'s "applied partially" option.

**D — The loser continues from the fallback node under Pushing On (§9.1), for the steps they had left, treating the confrontation node as the node they came from.** This is not §20's sentence: the loser gets the movement back, not the plan. §9.1's own ladder then supplies both edge cases §20's version has to invent — its level 5 *"exclude the node you just came from"* is what keeps the walk out of the fight node, and *"You cannot take an action at the end of a Pushing On route — you have no idea where you'll be"* keeps §15's action forfeit standing rather than making it a second thing to decide.

**E — D, extended to tie participants.**

**E+ — E, extended to the crossing-corrected winner.** The third call site, and the one GDD §15 contradicts most directly: the Winner bullet says outright *"Holds the node; their route continues"*, and `resolveDecisive` halts them anyway when the crossing correction moves them. They stand on the confrontation node rather than back from it, so there is no fallback to walk from and nothing to exclude — the remaining allowance simply becomes blind steps from where they are.

**F — Change nothing in §15; re-specify row 1's numerator only.** Not a remedy; included because the numerator turns out to be wrong independently of the rule, and this option asks whether fixing it is sufficient on its own.

**G — Record the row unmet and defer to M5.5**, the shape [D36](D36-lease-rate-chokepoint-gate.md) and [D37](D37-five-player-confrontation-load.md) both took.

## Decision

**Two parts, and neither works without the other.**

**1. Row 1's numerator is re-specified to count routes.** One per (round, seat) that submitted a non-empty route and whose first halt that round left at least one step of its declared plan unspent — not one per `EventRouteHalted`. The threshold stays at 15% and the verdict tier stays Operator. This is not a concession to the remedy: under the corrected reading the shipped rule **still fails at every player count**, which is why it is safe to fix it here rather than in a decision of its own.

**2. Every confrontation participant whose declared route can no longer be walked keeps the round's remaining *steps*, and spends them as GDD §9.1 blind steps from wherever they actually stand.** The declared route and the action stay forfeit. That is one rule covering all three of §1's call sites — the decisive loser, every tie participant, and the crossing-corrected winner — which is option **E+**, not §20's sentence, which is option A.

Concretely, replacing the second half of §15's Loser bullet:

> Falls back **one node** (**two** if Evasive) per the pushback rule below, and **loses the remainder of their declared route and their action** — but not the round's remaining **steps**. From the fallback node they **Push On** (§9.1) for as many steps as their allowance had left, under the sector bias they declared, or none; the confrontation node counts as the node they just came from, so the first of those steps may not re-enter it where an alternative exists.

in the tie paragraph, after *"stakes are returned"*:

> Their declared route and their action end there, and their remaining steps carry on from that node as blind steps, exactly as a loser's do.

and, for the winner whose position the crossing correction moved — the one case where §15's *"their route continues"* cannot be honoured literally — the same continuation from the node they hold, which is strictly better than the halt they get today and asks nothing new of them.

**§20's own remedy, option A, is rejected — on measurement, not on principle**, and §20's *"most likely by"* is the hedge that makes that a decision rather than a contradiction. A clears the corrected threshold at every player count, so this is not a case of the spec's remedy failing outright; it is a case of a better one already existing inside the GDD.

C is closed outright: at half the remaining steps the corrected rate is 16.32% at 4 players and 19.20% at 5, both intervals entirely above the line, and it straddles the line at 3. F is insufficient for the same reason the numerator correction is not a concession — the corrected shipped rule still fails everywhere. G is available and not needed: unlike [D36](D36-lease-rate-chokepoint-gate.md)'s and [D37](D37-five-player-confrontation-load.md)'s deferrals, this threshold has a remedy that clears it with margin at bot rigor, and deferring would leave M5.5 confirming a rule nobody had chosen.

## Reasoning

### 1. §20's sentence has exactly two mechanical readings, and the movement loop forces both

*"Continue their route from the fallback node"* cannot be executed as written. `advance` indexes `Order.Route` by the round's absolute step number and trusts each entry to be adjacent to the seat's live position; the fallback node is one or two nodes back along the traversed path, and the declared tail is adjacent to the node the fight happened at, not to it. So either:

- **the loser walks back** — the retrace, option A — which puts the confrontation node in their path; or
- **the loser is not pushed back at all** when they still have steps, and simply carries on from the fight node — which deletes the displacement penalty for exactly the losers §15 aimed it at, and is not something §20 proposes.

There is no third reading that keeps both the pushback and the declared route without inventing a path the player never declared, which is a re-plan, not a continuation. **The re-entry is not an implementation choice; it is what §20's sentence costs.** Measured, it happens to 89.3%–89.9% of continued losers under A, and 92.1%–92.7% under B.

GDD §15 has already ruled on this shape, in the paragraph immediately above the pushback table:

> The two-hop rule for a stationary Evasive loser needs the "must not re-enter the confrontation node" clause specifically, or the walk can go C → D → C and deposit the camper back on the contested chokepoint — turning a penalty into a free round of holding position. **Without it the rule is worse than no rule.**

This decision does not rest on that analogy — under A the return is paid for in steps and a pushback hop is free — but it is the reason §9.1's own ladder already carries the exclusion that option D needs, and it is why the exclusion is a rule the GDD owns rather than one this decision invents.

### 2. The no-re-entry form is not a trade-off against §20's — it is better on every row measured

The comparison that decides this is not R1 alone. Against the shipped rule, at 4 players, Operator:

| | R1 (routes cut mid-route), 5p | row 9 (Evasive), 4p | row 17 (Loitering), 5p | row 2 (deliveries), 4p | row 3 (RP lead), 4p |
|---|---|---|---|---|---|
| **shipped §15** | 29.13% | 45.50% | 8.46% | 0.8138 | 137.35% |
| **A** — §20 as written | 14.21% | 46.28% | 7.67% | 0.8336 | 138.54% |
| **A′** — A, action forfeit | 14.17% | 46.55% | 7.72% | 0.8080 | 140.20% |
| **B** — A, ties too | 7.54% | **51.05%** | 7.02% | 0.8544 | 138.94% |
| **B′** — A′, ties too | 7.50% | **51.47%** | 7.07% | 0.8247 | 140.59% |
| **D** — blind, no re-entry | 12.01% | 45.69% | 5.80% | 0.9404 | 127.16% |
| **E** — D, ties too | 6.41% | 45.57% | 5.16% | 0.9658 | 125.29% |
| **E+** — E, corrected winner too | **4.75%** | **45.73%** | **4.98%** | **0.9769** | **124.21%** |

Three things in that table decide it.

- **A does not have the margin the threshold needs.** 14.21% [14.14%, 14.28%] at 5 players clears 15% by 0.79 percentage points. The interval is entirely on the passing side, so under D35's rule A *passes* — but R9 is already unmet at 5 players ([D37](D37-five-player-confrontation-load.md)) and any future map or bot change moves this row. A threshold met by two thirds of a point is met on paper.
- **The obvious way to give A that margin is the one that costs row 9.** Extending A to ties (option B) takes 5 players to 7.54% — and moves row 9 from 45.50% to 51.05% at 4 players, spending more than half the remaining distance to its own `> 55%` failing line, on the row `#245` predicted would move. **E+ reaches better R1 numbers than B at every player count and leaves row 9 where it found it** — 45.73% against 45.50%, a difference smaller than the shipped rule's own interval. Whether the action survives (A vs A′, B vs B′) changes R1 by less than 0.05 pp and row 9 by less than 0.5 pp, so it is not the lever either; the re-entry is.
- **Everything else moves the right way under D, E and E+, and barely moves under A and B.** Under E+, Loitering (row 17, R11's own instrument) falls from 7.20% to 4.17% at 4 players and from 8.46% to 4.98% at 5; deliveries per player rise 20%; and the winner's RP lead over last place — GDD §22's comeback-mechanism row — falls 13 percentage points at 4 players, the only intervention measured here that moves it at all. Confrontations per match (row 10) land closer to the shipped rule under E+ than under any other option (17.35 against 18.03 at 4 players, 26.15 against 26.31 at 5), so none of this is bought by draining the confrontations out of the game — the fights still happen; they stop freezing the board.

Why A and B move so little on the rows that are not R1 is the same fact as §1: a continued loser who walks back through the fight node is walking back into the traffic that produced it. That shows up as more halts (row 1 as instrumented barely falls under A — 45.70% → 40.36% at 4 players, against E+'s 20.07%), and as a player who spends the rest of the match a step behind.

### 3. §9.1 supplies both edge cases §20's version would have to invent

Options D, E and E+ need two rules that option A would have to write from scratch, and GDD §9.1 already has both:

- **Where the blind steps may go.** §9.1's ladder, level 5: *"At every level, exclude the node you just came from unless it is the only option."* Pointing that at the confrontation node is the whole no-re-entry rule; no new clause, no new table.
- **Whether the action survives.** §9.1: *"You cannot take an action at the end of a Pushing On route — you have no idea where you'll be."* §15.0 already rejects *"Pushing On combined with an action"* at submission. So §15's action forfeit stands unchanged, and this decision never has to weigh it — where option A does, because a resumed declared route means the loser knows where they will be, and their action's legality was checked at submission against a node they will now not reach.

That last point is not decorative. Under A the loser ends **short** of their declared end node, so the action §15.0 validated against that node's type is now pointed at a different node. Keeping it (option A) opens a legality question at resolution time that the shipped rule never has to answer, and it is D30's territory: an action that can no longer complete has to fail with a named event, not silently. D, E and E+ do not create that question — the loser has no declared end node left to have been validated against.

### 4. The winner's halt was never a rule either, and it is the same rule that fixes it

GDD §15's Winner bullet ends *"Holds the node; their route continues."* `resolveDecisive` halts them anyway whenever the crossing correction moved their position, and its own doc comment concedes the point: *"the one case where 'route continues' doesn't literally hold."* It is 6.14%–6.82% of row 1's numerator, and — unlike the tie — it is a penalty applied to the participant the rules say won.

It cannot be fixed by honouring §15 literally, for §1's reason: the winner's declared tail is adjacent to the node they were carried to, not to the node the fight resolved at. But the continuation this decision adopts needs no special case for them at all. They already stand on a node; there is no fallback to walk from and nothing to exclude; the remaining allowance becomes blind steps exactly as it does for everyone else. Extending it costs one branch and takes row 1 from 6.41% to 4.75% at 5 players while leaving row 9 (45.57% → 45.73%), row 20 and confrontations per match where they were.

So the rule this decision states is not "the loser continues" with two exceptions bolted on. It is **one condition — the declared route can no longer be walked from where the participant now stands — with one consequence**, and the three call sites §1 enumerated are the three places that condition arises.

### 5. The tie halt was never a rule, and this is the decision that should say so

GDD §15's tie paragraph is *"everyone falls back to the node they came from, nobody loses cargo, stakes are returned. Ties are anticlimactic by design — it makes speculative aggression a worse bet."* Nothing there forfeits a round. `resolveTie`'s own doc comment is explicit that the halt is a mechanical necessity: *"the GDD does not restate that for a tie the way it does for a Loser, but it is an implementation necessity — `advance()` trusts `Route[step-1]` to be adjacent to the seat's actual current position, and a reverted position generally invalidates whatever the rest of a multi-step route assumed."*

That necessity is exactly what a continuation rule dissolves: once the remaining steps are blind steps from wherever the seat actually stands, no adjacency assumption survives to be invalidated. Ties are 21.2%–23.8% of row 1's numerator, and forfeiting the round is the least anticlimactic outcome the rules contain — so the extension is not a lever pulled to reach a number; it is the tie paragraph finally getting the mechanism it always implied.

## Consequences

### What changes in the specs

- **GDD §15** — the Loser bullet, the tie paragraph and the crossing-corrected winner, as quoted under *Decision*. The Winner bullet's *"Holds the node; their route continues"* stands as written; what changes is that the one case where it cannot hold literally now continues rather than halting.
- **GDD §20, R1** — the open item records the measurement, the verdict, and the remedy actually adopted, including that it is not the one the paragraph guessed at.
- **GDD §22 row 1** — the metric's unit is pinned in the row itself: one per submitted route cut short before its last step, never one per halt event.
- **RFC §6.4** — no new RNG purpose. `pushon.edge` and `scavenge.d6` are already *"1 per blind step"* and *"1 per newly explored node"*, and both stay lazily drawn, so the table's rows are unchanged and only the number of steps they cover moves. §6.4's worked examples gain the case, and its *"a loser who moved consumes 0, because their fallback walks a known route rather than drawing"* line stops being true and needs rewriting.

### What changes in the code, as tasks this hands forward

1. **`EventRouteHalted` gains a cause field** — `#245`'s own first step. Needed now for the re-specified row 1 (a halt with no step left must not count) and for any future audit of this numerator; the probe's version carried the cause plus the unspent-step count, and the real one needs at least enough to answer both.
2. **`internal/telemetry` row 1 is recomputed against the new definition**, per (round, seat) rather than per event, with `internal/telemetry/summary.go`'s `RoutesCancelledMidRoute` doc comment rewritten — it currently states the superseded definition and defends it.
3. **`internal/rules` implements the §15 change** — all three of `haltMovement`'s confrontation call sites (`resolveLoser`, `resolveTie`, and `resolveDecisive`'s corrected winner) clear the declared route and the action but convert the round's remaining allowance into `PushingOn` steps from wherever the seat now stands, with `seatWalk.Previous` pointed at the confrontation node so §9.1's ladder does the exclusion. The corrected winner needs no exclusion — they are standing on that node. `haltMovement` itself keeps its other callers unchanged.
4. **Golden replays and RNG index accounting regenerate.** The rule changes what a match is, so every golden fixture moves; `internal/rules/determinism_test.go` and the two bot golden fixtures are regenerated with this decision as the stated PR reason, which their own comments require.
5. **`#205`'s R1 sweep is re-run against the changed rule.** The harness invocation is recorded verbatim in `docs/exit-demos/205-r1-r6-r7.md`, so this is a re-run rather than a re-derivation, and it is what turns the numbers below from a probe's into the demonstration's.

### What this costs if it turns out wrong

The rule is a `resolveLoser`/`resolveTie` change behind no configuration flag, so reversing it is a revert plus another golden-fixture regeneration — cheap in code and expensive only in that the fixtures move twice. The numerator change is cheaper still and independent: it can stand whatever happens to §15, and it should, since it is a defect either way.

The two things worth watching, neither of which bot simulation can settle:

- **A blind walk is movement the player did not choose.** §9.1's Pushing On is opt-in, declared with a step count and a sector bias; this imposes it. The measured effect is good on every row, but "the loser is not frozen" and "the loser is satisfied" are different claims, and only the second is R1's actual worry. M5.5 confirms it or does not — the roadmap's own *"a sweep narrows the range, M5.5 confirms it"* caveat applies here more directly than to D36, because this one changes a rule rather than declining to.
- **Row 9 is outside its target band before and after.** 45.50% against a 20–40% band, `> 55%` to fail. E+ leaves it where it found it — 45.73% — which is the reason it was chosen over B, but it does not fix it. That is §9.3's question, not R1's, and it stays open.

### What does not change

The 15% threshold, R1's verdict tier (Operator), D35's verdict rule, §22 row 1's denominator, the pushback table, the stake/cargo/Infamy/Deadline-Pause consequences of losing, and every other §22 band. Confrontations per match (rows 10 and 11) stay inside the same verdict as before — still above R9's `> 12` line at 4 and 5 players, still inside the 4–12 band at 2 and 3 — and E+ lands closer to the shipped rule than any other option measured (5.7465 against 6.5629 at 2 players, 11.6594 against 12.4927 at 3, 17.3502 against 18.0297 at 4, 26.1498 against 26.3099 at 5). The metric does move; no band or threshold verdict does, so nothing here is bought by having fewer fights.
