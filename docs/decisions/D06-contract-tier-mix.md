# D6 — What determines the tier mix of a three-contract offer?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#44](https://github.com/garnizeh/cinzal/issues/44)

## The question

GDD §8.1 says a player is shown three contracts and takes one, or declines all. §8.3 gates tiers by Infamy — II at ≥ 3, III at ≥ 6, IV at ≥ 9. Neither section, nor anywhere else, says what determines the **tier** of each of the three. Read literally, a Legend eligible for I–IV could be shown three Tier I contracts, and the Contact Cooldown means they wait a round to try again.

## Why it is open

That is not a hypothetical corner case. With a uniform draw over the eligible set, an all-Tier-I offer to a Legend (4 independent tiers, 3 draws) is roughly 1 in 64 per offer, and the Legend cooldown is 1 round — near-certain across a season of matches, not a rare unlucky seed.

It also carries the balance case. §24.2's ladder arithmetic — the argument that a Nobody's contract ceiling is 8 RP while a Legend converts two or three Tier IV scores — is not just directionally consistent with a tier guarantee, it requires one exactly. Reworking its own table by RP-per-completed-job:

| Tier band | Offers | Jobs completed | Contract RP | RP ÷ jobs | Matches |
|---|---|---|---|---|---|
| Nobody (0–2) | 4 | 4 | 8 | **2** | Tier I RP |
| Known (3–5) | 5 | 5 | 15 | **3** | Tier II RP |
| Feared (6–8) | 8 | 3 | 15 | **5** | Tier III RP |
| Legend (9–10) | 15 | 2 | 16 | **8** | Tier IV RP |

Every completed job in §24.2's own numbers lands at exactly the RP value of the tier that Infamy band just unlocked — never a mix, never a job at a lower tier dragging the average down. For Known that means all 5 completed jobs are Tier II despite Tier I also being eligible; for Feared, all 3 are Tier III despite I and II being eligible too. A uniform draw over eligible tiers cannot produce that: with 3 independent uniform picks over {I, II}, a Known player's offer contains **zero** Tier II contracts about 1 time in 8, and §24.2's arithmetic silently assumes that never happens. The table was written as if some rule already guaranteed the top eligible tier every offer; this decision is the first place that rule gets written down.

Two more things GDD/RFC leave silent, both needed to write §8.1's rule precisely, both raised in the issue itself:

- Whether the three offered contracts must be distinct — same tier is presumably fine (two Tier I jobs are still two different jobs), same origin/destination pair presumably is not.
- Which Infamy the tier check uses — GDD §11.1 states general precedence for tier effects, and its own worked answer for this case is explicit: *"Contract tier gating and the Contact Cooldown use your Infamy at Phase 2, when the offer is generated."* This is settled already and is cited, not re-decided. (The issue text attributes this quote to "RFC §11.1" — that section number belongs to the GDD; the RFC's own §11.1 is the order form, an unrelated section. Noted here so the citation doesn't propagate.)

## Options

**A — Uniform over eligible tiers.** One line of code. Leaves the all-low-tier offer in — see above — and is directly contradicted by §24.2's own arithmetic, which assumes a guarantee that a uniform draw cannot produce.

**B — Guarantee at least one contract at the player's highest eligible tier**, the other two drawn uniformly over the eligible set. Matches §24.2 exactly (table above). The choice stays real — a Legend may still prefer a fast Tier II job to a slow Tier IV one — and it is one constraint to state and to test.

**C — A per-tier weight table in `Config`.** Most tunable, and the roadmap's M1 exit criterion is explicit that *"every number the GDD calls tunable is a `Config` field, never a constant"* (implementation-plan.md, Config as data). Cost, on its own: four numbers per Infamy tier that nobody has data to set, with no M2 measurement yet pointed at them, and — critically — a weight table alone still doesn't reproduce §24.2's arithmetic unless the weights happen to be tuned to guarantee the top tier, which is Option B wearing a configuration file.

## Decision

**B for the guaranteed slot, with C's tunability folded into the other two — not a fourth option, the synthesis the issue itself recommended.**

**One slot is deterministic.** Of the three offered contracts, the first is always drawn from the player's **single highest currently eligible tier**, using their Infamy at Phase 2 (GDD §11.1 — cited, not re-decided here). No draw decides *which* tier this slot targets; it is a pure function of Infamy.

**The other two slots' target tier is a weighted, independent draw over the full eligible tier set.** The weight per tier is a `Config` field — `ContractTier.OfferWeight`, one integer per tier — defaulting to **1 for every tier**, i.e. an even split, until M2's measurement gives a reason to move it. This is Option C's tunability bought at zero cost today: nothing about the default behaviour differs from "uniform," and nothing about §24.2's arithmetic depends on how these two slots land, because the guarantee alone is what the ladder's numbers require.

**Distinctness is already settled, by D7, not re-decided here.** [D7](D07-contract-pool-fallback.md)'s cascade is without-replacement across every tier's pool by construction — once a slot fills, its pair is removed from every other tier's candidate pool before the next slot's cascade runs. Three (or fewer) filled slots are always three distinct origin/destination pairs; same-tier repeats are permitted (they are different pairs), same-pair repeats are structurally impossible. D7's own text anticipated this: *"D6's own issue already presumes distinct origin/destination pairs; this decision makes that a mechanical property of the cascade rather than leaving it to depend on how D6's separate same-tier-repeat question resolves."*

**Slot resolution order — fixed and stated, because two independent draws plus a shared without-replacement pool need one to name which runs first:**

1. **Assign all three target tiers before any pool is touched.** Slot 1's target is the player's highest eligible tier (no draw). Slot 2's and slot 3's targets are each an independent weighted draw over the eligible tier set, purpose `contract.offer.tier`, one RNG index each — drawn every time, even when only one tier is eligible (a Nobody's two weighted draws are degenerate, always resolving to Tier I, but they still consume an index; see Reasoning).
2. **Run [D7](D07-contract-pool-fallback.md)'s per-slot cascade in slot order — 1, then 2, then 3** — purpose `contract.offer.pick`, zero or one index per slot depending on whether that slot's cascade finds a pair. Slot 1 runs first so the guarantee gets first claim on the shared candidate pool: if slot 2 or 3 happened to target the same tier and ran first, it could consume the only pair the guarantee needed, forcing the guarantee itself down D7's cascade for a reason that has nothing to do with an actually short pool. Running the guaranteed slot first means it only ever cascades downward because the *pool* was short — D7's own trigger condition — never because a same-tier sibling slot beat it to the only candidate.

**What this changes in RFC §6.4's `contract.offer` row.** The row's current "3 per offering seat" becomes, per offering seat: **2 draws under `contract.offer.tier`** (always, regardless of how many tiers are eligible) **plus `filled` draws under `contract.offer.pick`** (`filled` ∈ 0–3, per [D7](D07-contract-pool-fallback.md)'s own already-established per-slot cost) — **2 to 5 total**, up from a flat 3. Per D7's own precedent (and D3's and D01's before it — decisions produce a document, not a code or table edit), this table edit is not made in this pull request; it lands with whoever implements the RNG accounting (#56) and the contract subsystem (#63), using this document as the source of truth. The same implementer adds `OfferWeight int` to the `ContractTier` element of `Config.Contracts`, defaulting every tier to `1`.

## Reasoning

**The guarantee is not a design preference, it is what §24.2's own numbers require.** This is worth stating precisely because it changes the shape of the argument: Option A isn't merely worse than Option B on the merits, it's inconsistent with a table already in the document. A uniform draw over 2 eligible tiers puts *zero* copies of the higher one in a 3-pick offer about 12.5% of the time (`0.5³`), and over 3 eligible tiers about the same order of magnitude. §24.2's table has no room for that noise — every one of its four rows converts at exactly the top eligible tier's RP, every time. The rule this decision writes down is the one already assumed.

**Why the non-guaranteed slots get a weight dial instead of staying flatly uniform.** It costs nothing now — default weight 1 across all four tiers is uniform, byte-for-byte the same offer distribution Option B alone would produce — and it satisfies the roadmap's own M1 exit criterion ("Config as data") without inventing numbers nobody has evidence for yet, which is exactly the trap Option C alone falls into. The alternative, hard-coding uniform and adding a weight table later, is a second decision waiting to happen the moment M2's simulation harness (RFC §16.4) has something to say about offer composition — cheaper to leave the knob in place now than to reopen this file for it.

**Why the guaranteed slot needs no configuration and no draw.** It is not a balance dial, it's the mechanism that makes the *existing* Infamy-tier dials (cooldown table, step table) mean what §11's "shape of the curve" section already claims they mean — climbing the ladder buys visibly better contracts, not a better *chance* at them. Turning it into a weighted pick (even weighted heavily toward the top) reopens the 1-in-64 scenario at a smaller but still real probability, for a design property the rest of the document treats as certain.

**Why slot 1's cascade runs before slots 2 and 3's.** D7 already established that pool selection is without replacement *across* tiers, precisely because the tier bands overlap (a distance-6 pair is a valid candidate for II, III, and IV at once). That means which slot's cascade runs first can change which slot ends up dropping to a lower tier when two slots want the same scarce pair — and only one assignment of "goes first" is consistent with calling slot 1's tier a guarantee rather than a preference. Running it first is the only ordering under which "the offer always includes your highest eligible tier, pool permitting" is true as often as the pool allows; any other order would occasionally sacrifice the guarantee to a weighted slot that had no claim to priority.

**Why the tier-assignment draws happen even when there's only one eligible tier to choose from.** A Nobody's two weighted draws over `{I}` are degenerate — `Next(purpose, 1)` always returns index 0 — but they still advance the RNG sequence. This is the same shape [D3](D03-rng-consumption-table.md) and [D7](D07-contract-pool-fallback.md) already established for edge cases that only ever produce one outcome (Scaffolding's one-sector draw, D7's `min(k, n)` cascades): a call that is deterministic in a given match state is still a call, and skipping it when the outcome happens to be forced is exactly the kind of implementation-dependent shortcut that desynchronises two otherwise-correct implementations. Always drawing, regardless of how many real choices exist, keeps the index count a function of the *rule*, not of the *state* it happened to run against.

**Why the tier-assignment draw and the pool-pick draw are separate purpose strings.** Same reasoning [D3](D03-rng-consumption-table.md) already used to split `deck.event` into `.select` and `.order`: a divergent replay should be able to say *which stage* went wrong. "Which tier did slot 2 target" and "which specific pair filled it" are mechanically different operations with different candidate sets, and collapsing them into one `contract.offer` purpose string (as RFC §6.4 currently has it) throws away exactly the debugging property `purpose` exists for.

**On the citation in the issue.** The issue text quotes GDD §11.1 verbatim but labels it "RFC §11.1." The RFC does have a section numbered 11.1, but it is *The order form* (HTTP surface), unrelated to Infamy timing. This doesn't change the outcome — the quoted rule is correct and GDD §11.1 does say it — so it isn't a D15-style factual error worth its own task, only a mislabelled cross-reference worth not repeating.

## Consequences

- **GDD §8.1 gains the tier-mix rule as player-facing prose**, with a changelog entry, this PR. It states the guarantee and the even-split default for the other two slots in plain terms, without naming `Config` or RNG mechanics — consistent with how the GDD treats every other tunable dial (§16's "corrective dials in order of preference," §11's cooldown and step tables).
- **RFC §6.4's `contract.offer` row is not edited in this PR.** Per [D7](D07-contract-pool-fallback.md)'s own precedent, the split into `contract.offer.tier` (2 draws) and `contract.offer.pick` (0–3 draws, [D7](D07-contract-pool-fallback.md)'s own cost) lands with whoever implements the RNG accounting ([#56](https://github.com/garnizeh/cinzal/issues/56)) and the contract subsystem ([#63](https://github.com/garnizeh/cinzal/issues/63)), using this document as the source of truth — the same deferral D3, D7, D8 and D9 each made for their own table or struct edits.
- **`Config.Contracts[i].ContractTier` gains an `OfferWeight int` field**, default `1`, also an implementation-time edit rather than one made in this PR — flagged here so it isn't independently rediscovered.
- **[#63](https://github.com/garnizeh/cinzal/issues/63) is now unblocked on both of its named decisions.** It already cited D7's fallback by name; it can now also cite this document for the tier-mix rule that D7's cascade consumes as an input.
- Reversible at low cost while `internal/rules`' contract subsystem is unwritten; expensive after, for the same golden-replay reason every entry in this log carries — a fixed test seed bakes in whichever tier landed in whichever slot.
