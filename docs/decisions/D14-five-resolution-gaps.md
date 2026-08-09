# D14 — Five small resolution gaps: `Torched` at zero lease, `Muscle` in a melee, buying at the hand limit, `Open Doors` stock, `Bounty`'s tie-break

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#52](https://github.com/garnizeh/cinzal/issues/52)

## The question

Five small resolution rules the GDD and RFC leave unstated. Each is cheap alone; each is a silent bug if missed, which RFC §6.6 names directly — *"a rule quietly stops firing and nothing crashes"* — and which is why they are filed as one decision rather than five deferrals nobody circles back to.

1. Does a `Torched` post that goes to zero-or-below lease expire immediately in Phase 7, or in Upkeep — and does the public "corner went quiet" trace fire either way?
2. Does every non-winner of a 3-or-more-way melee lose `Muscle`, and does a tie change that?
3. Can a player Deal at hand limit 3, and does a same-round Field 4 discard let it compose?
4. Which market's stock does `Open Doors` draw from, and does half price round up or down?
5. What is `Bounty`'s tie-break when RFC §6.5's selection table doesn't carry one?

## Why it is open

### 1 · Torched at zero lease

GDD §14.3: *"Every post in the sector loses 3 rounds of lease, present or not."* A post with 2 rounds left goes to −1 under a literal reading. The issue frames this as two options that "differ by one round of sight" — expire the moment Torched fires (Phase 7), or floor at 0 and let Upkeep's ordinary decrement discover it.

Tracing RFC §6.7's pipeline shows that framing doesn't survive contact with the actual phase order:

```text
… writeTrail() → globalEvent() · incident() · pressure() → upkeep() …
```

`incident()` — where Torched fires — and `upkeep()` — where the ordinary per-round lease decrement and its "at zero" check live (GDD's Upkeep step 2, RFC §6.7) — are **both strictly after `writeTrail()`**, in that order, **within the same round**. Whatever the post's status was when `writeTrail()` ran is what that round's sight and trail already reflect, before either Torched or Upkeep gets a chance to touch it. So "expire in Phase 7" and "expire in Upkeep" are not actually a round apart — they are, at most, two phases apart in the *same* round, and every mechanism that could react to the post's absence (Stake Post, Pickup, any Field-2 action) has already resolved earlier still, in the Actions step. Neither ordering changes what any player can *do* about it this round.

What the framing does get right is that there are genuinely two different **mechanisms** on the table: a second, Torched-specific expiry check that fires and closes the lease the instant Phase 7 applies the −3, versus reusing Upkeep's existing per-round decrement-and-check, generalized to handle a decrement larger than 1. Two independent expiry checks is the real risk here — not lost sight, but a lease that could be closed twice, or a trace that fires from two different code paths and has to agree with itself.

Separately, and not raised by the framing at all: GDD's Upkeep step 2 text says *"decrement... at zero, the lease expires"* — that check has only ever needed to handle `1 → 0`, because until now nothing has ever subtracted more than 1 in a single pass. Torched is the first thing that can, and `== 0` silently stops catching a lease that Torched has already driven negative.

### 2 · Muscle in a melee

GDD §12: *"Muscle: Permanent: +1 in all confrontations. Lost when you lose a confrontation."* GDD §15: *"With three or more players in a node it's a free-for-all: highest total wins, everyone else loses."* Muscle costs Cr$ 7 — a third of a Tier I contract's payout — so whether "everyone else loses" triggers Muscle's loss condition for every one of them, and whether a tie (where §15 says "nobody wins... nobody loses cargo, stakes are returned") is a loss at all, is worth pinning down explicitly rather than trusting a reader to chain two sections together correctly at implementation time.

### 3 · Buying at the hand limit

GDD §12 caps the hand at 3 and says nothing about what happens when a Deal would exceed it. §15.0's order-legality table has no row for it. §9.4 notes that Field 4 discards resolve *before movement*, while Deal (a Field 2 action) resolves at Step N+1 — *after* movement — so a same-round discard genuinely could free the slot in time, but nothing states whether that's how the game actually handles it, versus rejecting the order outright or silently degrading the Deal to Nothing.

### 4 · Open Doors' stock

GDD §14.3: *"Every player ending here may immediately buy one item at half price, without being at a Black Market."* Every other purchase in the game reads a specific market's 3 rolled items (§12), refreshed at Phase 3. Open Doors has no market to read from — and, unlike Torn Map (§12: reveals 4 random Hidden nodes, no choice involved) or the other Field 4 items (target declared from what the player already knows), a Deal-shaped purchase requires the player to *choose which item*, and that choice has nowhere to attach: `Resolve` is a pure function with no I/O (RFC §6.1, §6.3), orders are fully submitted before any of Phase 6–7 runs, and Open Doors' own trigger — will the flagged sector's incident even be Open Doors, and will the player's route survive degradation to actually end there — isn't known until Phase 7, well after submission. Bolt Hole hit this identical shape (GDD §9.4: *"there is no moment at which to choose, because resolution does not pause for input... you declare the node when you declare the item"*) and solved it by moving the choice earlier. Open Doors has no discard declaration to attach a pre-choice to, because the player doesn't yet hold the item.

### 5 · Bounty's tie-break

RFC §6.5's selection table gives a primary key and a tie-break for five selections. `Bounty` (GDD §14.2: *"The highest-RP player has a price on their head. Whoever defeats them in a confrontation this round takes Cr$ 10 from the bank"*) isn't in the table. The neighbouring `Raid` row needs no tie-break because the GDD explicitly hits every tied player — but Bounty can't copy that: marking every tied highest-RP player as a live bounty target means each one who separately loses a confrontation this round pays out Cr$ 10 from the bank independently, so three players tied at the top turns a Cr$ 10 card into a potential Cr$ 30 one. `New Boss` sits one row up in the same table, selecting on the same stat (RP) for the same reason (a single named target), and already resolves its tie with the fairness key.

## Options

### 1 · Torched

- **A — Two independent expiry checks.** Torched clamps the lease to 0 and closes it (fires the trace) the instant Phase 7 applies the −3; Upkeep's decrement never sees an already-closed lease. *Against:* a second, parallel path to the same "lease expires, fire the trace" outcome that Upkeep step 2 already owns — two implementations of one rule that now have to be kept in sync forever, for zero behavioural gain given the phase-ordering finding above.
- **B — One expiry check, generalized.** Torched only ever decrements the lease counter (allowed to go negative); Upkeep step 2's existing "decrement, then check" logic is the sole place that detects and closes an expired lease, with its check widened from `== 0` to `<= 0`. *For:* one authoritative expiry path, matching D5's own reasoning for why Upkeep's steps are ordered the way they are; costs a one-comparator change to an existing check, not a new one.

### 2 · Muscle

- **A — Confirm the plain reading.** Every non-winner of a 3+-way melee is a Loser under §15's own framing, and Muscle's "lost when you lose a confrontation" applies to each of them identically to a 1v1 loss. A tie produces no Loser at all (§15: *"nobody wins... nobody loses"*), so Muscle survives a tie untouched. *For:* introduces no new rule; it is what §12 and §15 already say once read together, and the value of deciding it is purely in writing it down so nobody has to re-derive the chain under implementation pressure.
- **B — Muscle survives one loss per melee, win or lose depends on placement.** E.g. only the single lowest-total player loses Muscle, treating "everyone else loses" more narrowly. *Against:* invents a distinction the GDD's own confrontation math doesn't draw — there is no second-place; §15's TOTAL formula and Winner/Loser sections know only "highest" and "everyone else."

### 3 · Buying at the hand limit

- **A — Reject at submission (`Legal`), checked after same-order discards.** A new `Legal` row: if `(current hand count − Field 4 discards declared in this order) + 1 > 3` when Deal is the chosen action, reject. *For:* matches the shape of the existing balance-check row (§15.0) exactly — hand size, like balance, is fully known to the player and fully determined by their own prior actions at submission time, so it belongs in the "illegal payload" category (§15.0: *"never the result of the world changing"*), not the degradation category. Composability with a same-round discard falls out for free, because the check is computed against the whole order, not field-by-field.
- **B — Degrade the Deal to Nothing at resolution.** *Against:* Deal's target (the market, the item) doesn't change between submission and Step N+1 the way a contested node does — nothing about hand size is racing against another player's order. Silently downgrading a well-formed, checkable-in-advance order to Nothing is exactly the case §15.0 carves degradation out to *not* cover.
- **C — Permit unconditionally, auto-discard something.** *Against:* the GDD never grants the game authority to choose what a player gives up; every existing discard is player-declared.

### 4 · Open Doors

- **A — Fresh, private roll of 3 items, resolved in Phase 7.** *Against:* fatal on inspection — the player cannot pre-declare a choice among items that haven't been rolled yet, since the roll would happen after order submission and `Resolve` has no way to pause for input mid-pipeline (§6.1, §9.4's own Bolt Hole reasoning). Forces either removing player choice entirely (auto-select) or inventing a new interaction model resolution doesn't have.
- **B — The nearest or any Black Market in the flagged sector, sight required.** *Against:* not every sector is guaranteed a Black Market (§6.2's allocation is match-wide, not per-sector, and a 3-node sector could roll zero); "nearest" needs its own tie-break for equidistant markets, adding a determinism hazard for no clear benefit; and it doesn't solve the pre-declaration problem any better than A unless sight is already established, at which point sector-restriction adds nothing.
- **C — Any Black Market the player currently has sight of, declared up front, no new roll.** The player pre-declares, as an order add-on, a Black Market node and an item from its stock — both already visible to them, since Phase 3's stock refresh and Phase 1's Headline both happen *before* order submission (§14.1, §12). If Open Doors doesn't trigger for them, or the declared item is no longer on offer, or they declared nothing, the boon simply doesn't fire — no purchase, no error. *For:* solves the pre-declaration problem completely (nothing about the choice depends on anything not already known at submission time); costs zero new RNG, reusing whatever the existing `market.stock` roll already produced; needs no new fog surface, since "sight reveals stock" is already the rule (§12) and this reads a real, already-computed stock rather than inventing a hypothetical one. *Against:* a player with no Black Market in sight this round simply can't use the boon that round — narrower than A or B, but honestly so: they can't meaningfully choose among items they've never seen either way.
- **D — Full item catalog at half price, no roll.** *Against:* strictly better than a real Deal (choice of all 8 vs. a market's rolled 3), which undercuts the market's own scarcity design (§12: *"Each Black Market shows 3 rolled items"* is load-bearing, not incidental), and needs no randomness at all for a card the GDD frames as a boon alongside four others that all still gate on where you end your route.

### 5 · Bounty

- **A — Mark every tied highest-RP player, pay out per defeat.** *For:* the literal reading of "whoever defeats them," and the issue's own alternative phrasing. *Against:* the issue's own arithmetic — a 3-way RP tie turns a Cr$ 10 card into a potential Cr$ 30 one, unpredictably, based purely on how many players happen to be tied at the top this round. That's a real balance swing riding on a coincidence, not a decision either targeted player made.
- **B — Single target, fairness-key tie-break, matching New Boss's row.** Highest RP selects the target; ties resolve through the identical chain RFC §6.5 already defines (ascending Infamy → lower balance → lower RP → seeded coin) — the same key `New Boss` uses one row up in the same table, for the same reason (one stat, one named target). *For:* introduces no new tie-break machinery, caps the card at its printed value regardless of how many players share the RP lead, and treats Bounty as the RP-analogue of New Boss rather than as its own special case.

## Decision

### 1 · Torched — Option B

**One expiry check.** Torched only decrements; it never floors, clamps, or closes a lease itself. GDD's Upkeep step 2 (and RFC §6.7's `upkeep()`) is the sole place a lease transitions to expired, and its check widens from *"at zero"* to **at zero or below** — the only change this requires, since the mechanism (decrement, then check, then fire the anchor) is otherwise exactly what Upkeep step 2 already does for ordinary expiry. Whether a given round's expiry was caused by Torched, by ordinary time passing, or (per §15's Upkeep note) by a Debt-forced surrender is not distinguished anywhere downstream — matching the precedent already set for Debt-forced expiry: *"the same trace whether the lease expired on its own or was surrendered for debt. The district never learns which."* Torched-forced expiry is a third cause producing the identical trace.

**"The corner went quiet" fires, unconditionally, exactly as for any other expiry.** It is RFC §9.1 row 4 — global, named (owner), fixes the node — with no gate beyond "did the lease reach zero-or-below this round." The issue's question of "can an incident trigger it" is answered by pointing out there is no separate trigger to gate: row 4 fires off the lease's own state, not off *why* the state changed, so Torched needs no new conditional path into it.

**No new phase ordering, no new writer row.** Torched's −3 happens in `incident()` (Phase 7); the widened check lives entirely inside `upkeep()`'s existing step 2 (Phase 8), which already runs after it in the same round, every round (RFC §6.7). This round's own `writeTrail()` — and therefore this round's sight and trail — already ran before either phase, so the post's owner gets this round's sight of that node exactly as if Torched hadn't fired; the earliest observable consequence is next round. The lease-expiry Event itself is not deferred, though: it fires this round, appended to this round's Event stream from inside `upkeep()` exactly as RFC §6.7 already describes for every step in the pipeline, so recap, email and the public row-4 anchor all consume it on the normal schedule. What waits for next round is only the owner's own private sight of that specific node, since `writeTrail()` had already run before `upkeep()` got the chance to touch it.

### 2 · Muscle — Option A

**Every non-winner of a 3+-way melee loses Muscle if they hold it; a tie loses nobody's.** §15's free-for-all framing — *"highest total wins, everyone else loses"* — is not a separate outcome category from the ordinary Winner/Loser split; it is that split applied to more than two participants, and Muscle's trigger (*"lost when you lose a confrontation"*) reads it the same way a 1v1 loss does. A tie produces no Loser at all under §15's own text (*"nobody wins... nobody loses cargo, stakes are returned"*), so nothing in a tie ever reaches Muscle's trigger condition — Muscle survives untouched, for every participant, tied or not.

This ruling changes no rule text; it records that the chain composes the way a careful reading already implies, so nobody has to re-derive it at implementation time under the pressure of writing the melee resolver.

### 3 · Buying at the hand limit — Option A

**New order-legality row.** An order proposing Deal is illegal, and rejected at submission (falling back to the absence default per §18, exactly like every other row in §15.0's table) when:

```text
(current hand count − items discarded via Field 4 in this same order) + 1 > 3
```

The check is computed against the whole submitted order, not against Field 2 in isolation, so a same-round Field 4 discard **does** free the slot for a same-round Deal — the issue's own observation that immediate discards resolve before movement, and Deal resolves after, composes exactly as it looked like it should. This holds for armed discards too: *"armed discards are spent whether or not they fire"* (§9.4) means the item leaves the hand at declaration, before movement, identically to an immediate discard — only the item's *effect* timing differs between the two categories, not when the hand slot itself frees up.

**"Items discarded" counts held instances, not declared names.** §9.4 already limits Field 4 to items the player actually holds, so an order cannot legally declare more copies of a given item discarded than the hand contains — a player holding one Shiv cannot free two slots by naming it twice. The formula's subtracted term is bounded by that existing constraint, not a new one this decision has to add.

**Muscle counts toward the hand of 3.** §12 states the cap directly under a table that lists all eight items, Muscle included, with no carve-out; Muscle is simply the one item on that list that Field 4 cannot discard (§9.4: *"not a discard... permanent while held"*). A player holding Muscle plus two other items is at the cap like anyone else — they just can't clear it by discarding Muscle itself.

This is an order-legality table addition (GDD §15.0), landing beside the existing balance-check row, and a one-line addition to RFC §6.1's `Legal` signature's implied rule set — no change to `Resolve`'s degradation step, and no new RNG.

### 4 · Open Doors — Option C

**Draws from a Black Market the player has current sight of, declared up front.** A new, no-cost order add-on (Field 5-shaped, alongside the Ledger and lease renewal) lets a player declare a Black Market node and an item from its currently-visible stock, conditional on Open Doors triggering for them this round. Because Phase 3's stock refresh and Phase 1's Headline both precede order submission, every input to this declaration is real, current, and already known to the player when they build the order — nothing about it depends on a roll that hasn't happened yet. The declared market does **not** need to be inside the flagged sector; the card already discards the "must be at a market" requirement entirely (*"without being at a Black Market"*), and nothing in its text ties the stock's source to the node the player ends on.

**Degrades silently, never rejects.** If the player declared nothing, if Open Doors doesn't trigger for them this round, if the declared item is no longer on offer (bought by someone else earlier in the same round's Actions step), or if the declared node has stopped being a Black Market they have sight of, the boon simply does not fire — no purchase, no penalty, no error. This is the existing Step-0 degradation shape (§15.0), not a new one: a legal declaration whose target has moved under it. **`Legal` accepts the declaration unconditionally at submission**, including a market or item that is already invalid the moment it's declared — there is nothing to check against at submission time, since (unlike case 3's hand-limit row) every failure mode here is a resolution-time fact, not one knowable when the order is built.

**Half price rounds down.** RFC §6.3 states plainly that *"rounding is specified as 'down' everywhere it appears"* in the GDD, with no exception carved for this card. Muscle at Cr$ 7 costs Cr$ 3 under Open Doors.

**The hand limit from case 3 applies, checked at resolution rather than at submission.** Unlike an ordinary Deal, eligibility for Open Doors isn't known until Phase 7, so the check that Legal performs for case 3 cannot run at submission time here — there is nothing yet to check against. Instead, at the moment Open Doors would resolve the purchase, if the player's hand (as it stands after that round's own Field 4 discards, which have already resolved by then) is already at 3, the purchase silently doesn't happen — the same non-firing outcome as any other ineligible case above, not a special one.

**No new RNG, no new writer row.** The purchase reuses whichever `market.stock` roll already produced the declared market's current stock (Phase 3) — no new entry in D3's table. The resulting trace is an ordinary Item purchased entry, RFC §9.1 row 8, sight-gated, named only at the buyer's Infamy ≥ 6 exactly as any other Deal — Open Doors changes the price and the trigger, not the trail entry it produces.

### 5 · Bounty — Option B

**Resolved in Phase 6, identically to New Boss.** Bounty is a Global Event deck card (GDD §14.2), so like every card in that deck it resolves in `globalEvent()` — after Phase 5's confrontations and deliveries have already applied their effects, before Phase 7. "Highest RP" is read at that moment, against whatever state Phase 5 has already produced, exactly as New Boss's "lowest RP" already is. Neither card freezes its target earlier in the round — Bounty adds no new mechanism here, just the same un-frozen read New Boss already uses.

**Single target: highest RP, tied by the fairness key.** Selection is highest RP, exactly as printed; on a tie, resolve through the identical chain RFC §6.5 already defines for contended selections — ascending Infamy → lower balance → lower RP → seeded coin — the same key `New Boss` uses one row up in the same table for the same reason (one stat, one named target). Because all tied candidates share the same RP by construction, the tie-break chain's own "lower RP" level is a guaranteed no-op here and falls through to Infamy, exactly as it does whenever `New Boss`'s tie-break chain is invoked against players tied on its own primary key.

**No new anchor is needed to identify the target.** RFC §9.1's `PlayerView.Others` already carries every opponent's RP unconditionally (*"band, infamy, RP, posts — never position"*) — every player can already compute "who is highest-RP" themselves, identically, from information they already have, the same reasoning RFC §9.1 uses to explain why row 8 (Item purchased at Infamy ≥ 6) never discloses a node its reader didn't already have.

**The payout is per confrontation the target actually loses this round, not capped to one.** This is not a new rule invented for Bounty — it is what "whoever defeats them" already says, applied to the one selected target rather than to a whole tied set. A target who loses two separate confrontations in the same round (rare, since a lost confrontation ordinarily ends a player's route for the round, but reachable if a later step's collision catches them after a pushback) pays the bounty to each separate winner, exactly as their stake and cargo are separately at risk in each fight — Bounty attaches Cr$ 10 from the bank to *being defeated*, once each time it happens, the same way every other confrontation consequence already scales with however many times it occurs. What Option B forecloses is not repeat payouts to repeat winners; it's the version where the *card itself* multiplies with the number of players tied for the target slot, which was never earned by any decision either tied player made.

## Reasoning

**Every one of these five resolves to reusing an existing mechanism rather than inventing a new one, and that is not a coincidence — it is the same failure mode RFC §6.6 and D13 both name.** A gap like this is never a missing *feature*; it's a missing sentence connecting two rules that are each already fully specified on their own. Torched needn't invent an expiry path because Upkeep already has one; Muscle needn't invent a melee-loss rule because the Winner/Loser split already is one; the hand limit needn't invent a new resolution-time behaviour because the balance-check row already shows what "checkable at submission" looks like; Open Doors needn't invent a new purchase mechanism because Deal and Bolt Hole's up-front declaration already cover both halves of the problem between them; Bounty needn't invent a tie-break because New Boss's row already is one. In every case, tracing the actual pipeline order or the actual struct contents first — rather than pattern-matching the issue's own suggested options — either eliminated an option outright (Open Doors' fresh-roll option A, dead on the pre-declaration constraint) or showed that two proposed options weren't actually distinguishable in practice (Torched's "differ by one round of sight," which the phase order shows they don't).

**The one genuinely new piece of state this decision adds is Open Doors' pre-declaration add-on**, and it is new only in the sense that no *existing* order field fit it — Field 4 assumes the player already holds the item being declared against, which isn't true here. It is not new in kind: it is the same up-front-declaration shape Bolt Hole and Decoy already use, for the identical reason (resolution cannot pause for input), applied to the one boon in GDD §14.3 that requires a choice at trigger time when none of the other four do.

**Muscle and Bounty are the two rulings that add no mechanism at all** — both are read directly off text that was already fully determinate once two sections are held together at once. They are worth deciding anyway, not because the answer was hard, but because RFC §6.6's whole point is that a correct-but-undocumented chain of reasoning is exactly as fragile as an incorrect one the moment someone implements only one of the two sections it depends on.

## Consequences

- **RFC §6.7's `upkeep()` pseudocode and GDD's Upkeep step 2 text** both need their lease-expiry check restated as "at zero or below" rather than "at zero." No new step, no new event kind — the same `Event` that already fires row 4's anchor fires it here too.
- **GDD §15.0's order-legality table gains one row**: a Deal that would exceed hand limit 3 after this order's own Field 4 discards, in the "reject, absence default" category — beside the existing balance-check row, not the degradation table. RFC §6.1's `Legal(v PlayerView, o Order, cfg Config) error` implements it as one more computed condition, no signature change.
- **A new order add-on** (Field 5-shaped: no action cost, always legal to submit, may simply never trigger) carries Open Doors' pre-declared Black Market node and item choice. This is the one net-new piece of `Order` shape this decision introduces; everything else is a check against state the order and `Resolve` already carry.
- **No RNG consumption change anywhere in this decision.** Torched is deterministic (hits every post in the sector, no draw). Muscle's loss is a deduction from the melee's own resolution, not a draw. The hand-limit check is arithmetic. Open Doors reuses an existing `market.stock` roll rather than drawing a new one. Bounty's tie-break reuses the existing fairness-key chain, which only spends its seeded-coin draw at the fourth level exactly as it always does. **D3's table gains no new row from this decision** — the issue anticipated cases 4 and 5 might need one; tracing both shows they don't.
- **RFC §9.1's twelve-row writer table is unchanged.** Open Doors' purchase is row 8 wearing a different trigger and price, not a new row; Bounty's target needs no writer at all, since RP is already unconditional per-opponent state.
- **GDD §12, §14.2, §14.3 and §15's substantive rules text need no change beyond §15.0's new row noted above.** This decision closes gaps between sections that were each individually already correct; none of the five rulings contradicts a stated rule, and none required rewriting a card's printed effect.
- Reversible at low cost while `Resolve`, `Legal` and `Project` are unwritten. After: cases 1, 2 and 5 are pure resolution logic and cheap to revisit even post-implementation, since none of them touches stored schema. Case 3's `Legal` row and case 4's new order field are the two with real reversal cost — the latter because it is the one genuinely new piece of wire format this decision adds, and changing an `Order` field shape after matches have been played against it is the same golden-replay cost every entry in this log carries.
