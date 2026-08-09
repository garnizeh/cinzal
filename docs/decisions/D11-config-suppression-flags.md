# D11 — Which subsystems can `Config` switch off, and what does "off" mean for each?

**Status:** decided
**Blocks:** M1 — Rules core, M7 — Onboarding
**Decided:** 2026-08-08
**Issue:** [#49](https://github.com/garnizeh/cinzal/issues/49)

## The question

RFC §14.4 requires solo scenarios to disable subsystems "as `Config` flags, not as branches in `rules`." The `Config` sketch in RFC §6.2 has none of these fields. Two things have to be answered together, because the flag names are the easy half: **which subsystems get a flag**, and **what "off" means for each one** — and several of those have a wrong answer that looks reasonable.

## Why it is open

GDD §19.1's scenario ladder is explicit about what each stage omits:

| Scenario | Deliberately absent |
|---|---|
| 1 · First Run | Leases, incidents, Infamy tiers, items |
| 2 · The Trail | Infamy tiers, events |
| 3 · The Ladder | Incidents |
| 4 · Pressure, 5 · Full Match | — |

`rules` is the deepest package in the dependency graph (roadmap risk **P4**). Retrofitting a suppression flag after M7's bots, the simulation harness, and every M1 task already depend on `Config`'s shape means reopening a pure package everything imports — so this is decided in M1, even though M7 is the first consumer, the same pull-forward the roadmap already applies to [D8](D08-sector-size-constraint.md)'s scenario node counts.

The hard part is semantics, not naming. Each subsystem raises a specific question that a plausible wrong answer gets wrong:

| Subsystem | The question "off" raises |
|---|---|
| **Leases** | Is Stake Post illegal (rejected by `Legal`), or legal-but-free? Does the post cap become 0? Do posts still score 2 RP and sector majority? |
| **Incidents** | Is the Unstable Sector still announced in the Headline? Scenario 3 says incidents are absent — a Headline promising "something is going to happen there" that never happens teaches the player a wrong lesson. |
| **Events** | Same question for the event-category line, and: are rounds 4–15 silent, or is the deck simply not built? The latter changes `initial()`'s RNG consumption, which [D3](D03-rng-consumption-table.md)'s table must cover. |
| **Infamy tiers** | The sharpest one. Does Infamy stop being tracked, or does it track and stop having *effects*? Steps, cooldown, combat bonus, name gating in the trail, and position reveals all key off the tier. Scenario 1 teaches "route, action, stance, one delivery" — so a flat 4 steps, a flat cooldown, no reveals. |
| **Items** | Do Black Markets still exist as node types, with Deal illegal? Or does generation stop placing them — which changes [D9](D09-node-type-rounding.md)'s type shares and therefore the map? |
| **Pressure** | Follows Infamy tiers, but is separately suppressible in principle. |

## Options

**A — One boolean per subsystem**, grouped in a nested struct so `Config` stays readable. Simple, greppable, and each flag is one guard at one edge of the subsystem — the same "guard placed at the boundary, never sprinkled through the rules" discipline [D5](D05-upkeep-phase.md) already used for Upkeep's dependency-ordered steps.

- For: a reviewer can read the struct and know exactly which subsystems a given scenario touches, with no indirection.
- Against: none of substance for a fixed, small set of five subsystems — this isn't the shape where a combinatorial explosion of booleans becomes unreadable.

**B — A single `ScenarioProfile` enum.** Fewer fields, but it hides which subsystem is actually off behind a name, and every new scenario needs a new enum value defined inside `rules` — the exact branch-wearing-a-different-hat RFC §14.4 forbids. It also can't express free play's "any combination", since free play (§19.1) isn't one of the five named rows.

## Decision

**Option A.** `Config` gains one nested field:

```go
type Config struct {
    // … existing fields (RFC §6.2) …
    Suppress SubsystemSuppression
}

type SubsystemSuppression struct {
    Leases      bool
    Incidents   bool
    Events      bool
    InfamyTiers bool
    Items       bool
}
```

Zero value is `false` for every field — an ordinary match (and scenarios 4–5) never sets any of them, matching GDD §19.1's own "—" for those two rows. **No `Pressure` field.** Pressure's precondition is being Legend tier, and Legend tier cannot be reached while `InfamyTiers` is suppressed (below) — Pressure is already fully suppressed as a consequence, with no code path that would ever check an independent flag. Adding one would be a knob nothing turns, the same objection [D9](D09-node-type-rounding.md)'s reasoning raises against inventing unneeded structure.

Semantics, one guard per subsystem, each placed where the phase or action already runs — never a condition threaded through the rest of `rules`:

**Leases.** `Suppress.Leases` makes **Stake Post illegal** — rejected by `Legal()` exactly like any other illegal order component, not legal-but-free. Nothing else changes: `PostCapByPlayers`, the sector-majority computation, and final scoring's post-RP line are all untouched code, because with staking illegal no post is ever created and every one of those computations simply operates over an empty set. This is the cheapest possible guard — one new rejection rule in `Legal()` — and it needs no companion changes anywhere else in `Resolve`.

**Incidents.** `Suppress.Incidents` does two things, both at Setup or announcement boundaries: the incident deck (`deck.incident.select` / `deck.incident.order`, [D3](D03-rng-consumption-table.md)) is **never built** — not drawn and discarded, simply skipped, so it consumes **zero** RNG indices rather than 25 — and the Headline (GDD §14.1) never prints an Unstable Sector line, in any round. Phase 7's incident resolution step becomes a no-op by construction: there is no flagged sector to check, because none was ever chosen. This is what stops the "something is going to happen there, and then nothing does" problem the issue names — the flag that would trigger the promise is the same flag that gets suppressed.

**Events.** `Suppress.Events` is the same shape as Incidents: the event deck (`deck.event.select` / `deck.event.order`) is **never built** (zero RNG indices, not 23), and the Headline's global-event-category line never prints, in any round including 4–15. Rounds 4–15 are silent because there is no deck to pop from, not because a populated deck is being hidden.

**Infamy tiers.** Infamy **keeps being tracked as a plain integer, exactly as GDD §11's Gains and Losses table already specifies** — deliveries, confrontation wins, Stake Post, Vanish, Debt all still move the number. It has to: GDD §16's tiebreaker order reads "highest Infamy" unconditionally, in every scenario, so the value must exist and be meaningful even where the ladder is suppressed. What `Suppress.InfamyTiers` gates is a single function — the lookup that turns an Infamy value into the ladder row (combat bonus, contract-tier ceiling, Contact Cooldown, step base, exposure/reveal behaviour, GDD §11's table). When suppressed, that lookup always returns the **Nobody** row (0–2), regardless of the player's actual Infamy: flat 4 steps (subject to the same §9.1a additive modifiers everyone gets), flat 4-round cooldown, +0 combat, no name in the trail, no position reveal, contract offers capped at tier I. One player could sit at numeric Infamy 9 in a suppressed scenario and still be treated as Nobody by every tier-gated rule — that is the intended behaviour, not a bug, and it is why Pressure needs no flag of its own: Pressure's precondition (Legend tier) is unreachable through this same lookup.

**Items.** `Suppress.Items` leaves **map generation untouched** — Black Market nodes generate exactly as [D9](D09-node-type-rounding.md)'s table specifies, at every supported node count, so this decision does not reopen D9's shares or touch `rules/gen`. What changes is downstream of generation, at the same two boundaries Leases uses: a Black Market's stock is **never rolled** (the "3 rolled items, refreshed every 2 rounds" step is skipped, so the node always shows zero items to see or buy), and **Deal is illegal**, rejected by `Legal()` the same way Stake Post is under `Suppress.Leases`. A Black Market under this flag is inert map furniture — present, walkable, contributing to the type mix D9 already fixed, but never a source or sink of items.

## Reasoning

**Every guard sits at the point the subsystem is already invoked, never inside shared resolution logic.** `Legal()` gains two new rejection cases (Stake Post, Deal); the Headline's print step gains two early-outs; the tier lookup gains one early-return; the deck-build step at Setup gains two skips. None of this is a condition inside the confrontation math, the step formula's modifier sum, or Upkeep's five ordered steps — [D5](D05-upkeep-phase.md)'s own reasoning already established that a disabled subsystem should make a step a no-op *by empty input*, not by a branch, and every guard here follows that same shape: the branch lives at the edge where state is created or announced, and everything downstream is unmodified code operating on an empty or flat result.

**Infamy tracks unconditionally because GDD §16 needs it to, in every scenario, not just the ones with tiers on.** Splitting "track" from "have effects" costs exactly one function boundary (the tier lookup) and avoids inventing a second representation of Infamy for suppressed matches, or a special case in the tiebreaker rule for scenarios where tiers are off. It is also the cheaper of the two readings the issue poses: the alternative (Infamy stops being tracked) would need `Resolve` to special-case every one of GDD §11's Gains and Losses rows for suppressed matches, where this reading needs zero changes to any of them.

**Items keeps map generation untouched for the same reason Leases doesn't touch the post cap.** Both flags are deliberately shallow: they remove the *action* that would create economic activity around a piece of map furniture, not the furniture itself. Making Items reach back into `rules/gen` and renegotiate D9's shares would be the single most expensive option on the table for a flag whose only named consumer (Scenario 1, 12 nodes) needs nothing more than "there is nothing to buy" — and it would make Items the only suppression flag that composes with map generation instead of with `Legal()` or the Headline, breaking the pattern the other four all share.

**No `ScenarioProfile` enum, because option B's cost compounds exactly where this decision needs it not to.** Every new named scenario — and M7 adds three (GDD §19.1's own five, of which three are new node counts already resolved by D8) — would need a new `rules`-internal enum value under option B, which is the branch RFC §14.4 explicitly rules out. Option A's cost is flat: a sixth scenario, or free play's arbitrary combination, needs zero new code in `rules`, only a new data row wherever M7 stores scenario definitions (RFC §14.4's "scenarios are data" already assumes this).

## Consequences

- **`internal/game` gains `SubsystemSuppression`, and `Config` gains a `Suppress` field of that type.** Both are plain data — no behaviour, no imports — consistent with `internal/game`'s leaf status under [D01](D01-package-layout.md).
- **`rules.Legal()` gains two rejection cases** (Stake Post under `Suppress.Leases`, Deal under `Suppress.Items`), each a single early check, not a change to the confrontation, movement, or step-allowance logic.
- **The tier-lookup function gains one early return**, pinning every ladder-derived value to the Nobody row when `Suppress.InfamyTiers` is set. This is the only place Infamy's *effects* are gated; the Gains and Losses table (GDD §11) is unmodified.
- **Setup's deck-build step gains two skips.** `deck.event.select`/`deck.event.order` (23 draws) are skipped entirely under `Suppress.Events`; `deck.incident.select`/`deck.incident.order` (25 draws) are skipped entirely under `Suppress.Incidents`. [D3](D03-rng-consumption-table.md)'s table gains a note that these four rows' costs are conditional on the corresponding `Suppress` flag being unset — zero draws, not a drawn-and-discarded deck — the same "lazy, never a fixed cost paid unconditionally" discipline RFC §6.4 already states for early-terminated draws elsewhere in the pipeline, applied here at Setup instead of mid-round.
- **The Headline gains two early-outs**: no Unstable Sector line under `Suppress.Incidents`, no event-category line under `Suppress.Events`, in every round including the ones that would otherwise carry one.
- **RFC §6.2's `Config` sketch gains the `Suppress` field**; §14.4 gains a cross-reference to this decision in place of its own ad hoc "disables leases, incidents, items and Infamy tiers" sentence.
- **M7's scenario data rows (RFC §14.4, "scenarios are data") can now cite concrete field names** — `Suppress.Leases`, `.Incidents`, `.Events`, `.InfamyTiers`, `.Items` — instead of an unspecified suppression mechanism. Which flags each of the five named scenarios sets is scenario data, not this decision: this document fixes the mechanism and the semantics, GDD §19.1's table already fixes the intent, and M7 is where the two meet.
- **The M2 simulation harness must honour these flags** and continue tagging suppressed-subsystem runs the same way solo runs are already tagged (`opponents=bots`, RFC §14.4) — a match with tiers or incidents off is exploring a different rule set, and mixing its telemetry into the human-balance set would corrupt it invisibly, the same failure shape RFC §14.4 already calls out for solo-vs-human data.
- Reversible at low cost while `rules` is unwritten; expensive after, for the same reason every structural decision in this log carries that caveat — once `Config` round-trips through stored matches, a field rename or semantics change touches every replay that used it.
