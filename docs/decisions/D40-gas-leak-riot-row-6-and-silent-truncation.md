# D40 — Gas Leak and Riot never reach §22 row 6 — is row 6 measuring the incident load it claims to?

**Status:** decided
**Blocks:** [#274](https://github.com/garnizeh/cinzal/issues/274) — the implementing task (events, golden fixtures, #205's R6 re-run)
**Decided:** 2026-08-24
**Issue:** [#246](https://github.com/garnizeh/cinzal/issues/246)

## The question

[#205](https://github.com/garnizeh/cinzal/issues/205)'s exit demonstration split GDD §22 row 6 by incident card, across 20,000 matches per player count (`docs/exit-demos/205-r1-r6-r7.md`). Seven of sixteen cards register zero hits at every player count. Five are correct and uncontroversial — Sinkhole, Informant Ring, Spilled Load, Open Doors and Word of Work resolve into a node effect, a public reveal, a crate, a discounted purchase, and an extra offer, none of them a seat being hit. **Gas Leak and Riot are the other two, and they are not the same case**: both genuinely affect a player, and both are zero only because of where they resolve in the pipeline.

- **Gas Leak** (`internal/rules/movement.go`, `advance()`) cancels an eligible seat's move and clears its `Route`, `PushingOn` and `Action` for the round — the single most complete negation of a turn any card in either deck performs.
- **Riot** (`internal/rules/trail.go`, `applyRiotPermutation`) permutes the flagged sector's eligible trail entries, falsifying what a seat's own trace tells the map this round.

Both resolve in an earlier phase than `incident()` (movement, and `writeTrail`, respectively), so `incident()`'s own switch is a documented no-op for both, and neither ever emits `EventIncidentHit`. [D33](D33-telemetry-event-stream-coverage.md)'s row-6 resolver list names only the nine resolvers living inside `incidents.go` — the implementation matches that decision exactly; the decision is what never considered these two.

Two questions, both raised by [#246](https://github.com/garnizeh/cinzal/issues/246):

1. **Does row 6 get corrected to count them?** At the shipped default, Operator, pooled, row 6 reads 20.61% / 26.36% / 31.73% / 35.93% at 2/3/4/5 players against a `< 20%` failing line — the 2-player figure clears it by only 0.47 percentage points, and two more hit-capable cards moves the aggregate by materially more than that.
2. **Gas Leak also clears a seat's route and action while emitting no event at all — not a telemetry event, not a player-facing one.** GDD §15.0's *"Orders never silently fail"* already produces a specific kind for every Step 0 degradation, and [D30](D30-contended-action-loss-notification.md) already generalized that guarantee past Step 0 to four more resolution-time categories. Is Gas Leak's silence the same gap D30 already closed, just at a call site D30 never audited, or a separate decision?

## Why it is open

**The row's own wording says "hitting," and Gas Leak is the paradigm case of a hit.** §22 row 6 is *"Sector incidents actually hitting a player,"* answering R6's *"routes feel unplannable"* (GDD §20). A card that deletes a seat's entire turn — route, push-on, and action, all three — is exactly what an unplannable route looks like, and it currently contributes nothing to the row built to measure that.

**D30's own reasoning already extends past Step 0, and never reached Gas Leak.** D30's audit covered `resolveDeal`, `resolvePickup`, `resolveStakePost`'s cap branch, and `resolveLedger` — four categories, six causes, all firing at Step N+1 or Step N+3, all "a legal order that can no longer complete by the time it resolves." D30's decision text states the guarantee generally: *"wherever a legally-submitted order's declared action fails to do what it said... the player gets a specific reason."* Gas Leak's truncation is exactly that shape — a route legal at submission, invalidated mid-movement by a card whose sector wasn't known until the round it lands — but it fires from a different pipeline phase (movement itself) than any of D30's four categories, so D30's own audit never reached it.

**Torched is not the same case as the other zeros, and is already understood.** It hits lease owners, and Operator almost never holds a lease ([#204](https://github.com/garnizeh/cinzal/issues/204), [D36](D36-lease-rate-chokepoint-gate.md)); under Drifter it hits 11.8%–21.9%, confirming the mechanism rather than the card. It travels with the lease-rate finding, not with this decision.

**#205's own R6 verdict is already closed and does not reopen here.** NO ACTION at every player count, with this exact under-count recorded as a stated caveat and deferred to M5.5 alongside a separate band question (§241's carrier). Nothing in M2 is blocked by D40 either way — correcting the instrumentation and re-reading the verdict against the corrected number is the implementing task's job, not this document's.

## Options — row 6's undercount

**A — Emit `EventIncidentHit` from both cards' real call sites.** Row 6 then measures what its own wording says. Costs two emissions from functions that currently produce no incident events, in phases that are not the incident phase, plus a golden-fixture re-record.

**B — Narrow row 6's wording to what it actually measures** ("sector incidents resolved in Phase 7 that hit a player"). Cheapest, and defensible only if "hitting" is genuinely meant to exclude a cancelled turn — hard to argue, given the row exists specifically to answer whether routes feel unplannable, and a cancelled route is the least plannable outcome in either deck.

**C — Leave both alone; record the under-count in §22.** Only tenable if the 2-player margin is re-examined and found robust anyway — but establishing that requires computing the corrected number, which is Option A's own output. There is no cheaper way to check C than to do A.

## Options — Gas Leak's silent notification

**1 — Leave it silent.** Rejected outright: it contradicts GDD §15.0's text directly, and D30 already established that "Orders never silently fail" is a standing pipeline guarantee, not a sentence scoped to the paragraph it sits in.

**2 — Reuse `EventRouteTruncated`.** Rejected on D30's own precedent: `EventCurfewTruncated` exists as a distinct kind from `EventRouteTruncated` specifically *because* the cause is a different "specific reason" a player needs named — a step-allowance change is not a destroyed edge, even though both truncate a route identically. A sector incident is exactly as distinct a cause as Curfew was.

**3 — A new, dedicated `EventKind`, fired at the same branch as the row-6 fix.** Same shape as the existing four Step 0 kinds (`Round`, `Node`, `Seat`), fired the moment `advance()` truncates the seat's route for Gas Leak.

## Decision

**Option A for row 6, Option 3 for the silent notification, decided together as one decision.**

**Gas Leak** (`movement.go`, `advance()`) — at the existing truncation branch, the moment a step's destination reverts to `from` because of Gas Leak: emit one `EventIncidentHit{Round, Node: from, Seat: seat}` (row 6's fix), and one new `EventGasLeakTruncated{Round, Node: from, Seat: seat}` (the notification fix) — both from the same branch, same commit. `advance()` gains an `[]game.Event` return; `Resolve`'s movement loop (`resolve.go:83-89`) appends it alongside the existing transition/crossing/collision handling.

**Riot** (`trail.go`, `applyRiotPermutation`) — for every `riotEligible` entry actually collected before the permutation, emit one `EventIncidentHit{Round, Node: entry.Node (the real origin, before the permutation moves it), Seat}` for each seat the entry names: `*entry.Actor` when non-nil, and — because GDD's confrontation trail row always names both parties — `*entry.Target` as well for an `EventConfrontation` entry. A Fresh Tracks entry, or a cargo-taken/item-purchased entry below its own naming gate, contributes nothing: there is no seat identity to attach a hit to, matching GDD §14.3's own text that Riot only touches sight-gated, *named* information. `applyRiotPermutation` gains an `[]game.Event` return; `writeTrail` appends it into `out` before `distributeTrail` runs.

**This is one decision, not two.** Both parts share Gas Leak's exact call site and land in the exact same commit; splitting them would produce two documents whose "why it is open" section each point at the other. D30 already supplies the governing principle for the notification half — this decision applies it to a call site D30's own audit never reached, it doesn't re-derive it.

**Not decided here, filed separately as an adjacent finding:** GDD §14.3 also requires a player whose own trace Riot moved to be told so (not where) — a distinct, unconditional disclosure requirement with no telemetry angle and no shared call site with Gas Leak's fix. `grep -rn "trace was moved\|trace moved" internal/` finds no such implementation. Filed as [#275](https://github.com/garnizeh/cinzal/issues/275) rather than folded in here, since it needs its own design call (which entry kinds qualify, what the notification may not disclose) that this decision's scope doesn't require settling.

## Reasoning

**Why not Option B (narrow row 6's wording).** The row's own text is *"actually hitting,"* answering §20's *"routes feel unplannable"* — a cancelled route, action and all, is the least plannable outcome the game has. Redefining the row to exclude it would be redefining it around the instrumentation gap rather than closing the gap.

**Why not Option C (record and leave).** The 2-player cell's 0.47-point margin over the failing line is not comfortably inside noise once two more hit-capable cards are added back — the issue's own estimate is that the correction moves the aggregate by materially more than that margin. And there's no way to *check* that C is safe without computing what A produces; C is not actually a cheaper option, it's A's output treated as unnecessary before it's known.

**Why Option A is cheap enough to take outright, matching D33's own standard.** D33 drew the line between "small, well-scoped addition" and "genuinely expensive row" by what each row's fix actually costs — two emissions from two already-identified call sites is the same shape as the four additions D33 approved for rows 1/6/9/12, not a new audit across the deck.

**Why Riot's `Seat` comes from `entry.Actor`/`entry.Target` directly, not `riotParticipant`.** `riotParticipant`'s own doc comment states its `0` return is deliberately ambiguous between "seat 0" and "no identifiable seat," safe only because its one existing caller uses it purely as a sort key. Reusing it to populate an event's `Seat` field would silently mint a false hit against seat 0 on every anonymous Fresh Tracks or below-gate entry Riot happens to touch — a bug this decision would be introducing, not one it would be closing.

**Why the notification gets its own `EventKind` rather than reusing `EventRouteTruncated`.** D30 already decided this exact question once, for Curfew versus a destroyed edge: same truncation shape, different cause, different kind, because the cause is the specific reason GDD §15.0 requires naming. A sector incident is not a Step-0 world-state check either — it is discovered mid-movement, the one phase D30's own four-category audit never covered — so it earns its own kind on the same reasoning that already gave Curfew one.

**Why #205's R6 verdict is not re-litigated by this decision.** It is already closed NO ACTION, carrying this exact under-count as a named caveat, deferred to M5.5 alongside a separate band question the per-card table also surfaced. This decision fixes the instrumentation the verdict was read against; whether the corrected number moves any cell across a line is a measurement the implementing task performs and reports, not something this document predicts.

## Consequences

- `internal/game/event.go` gains one new `EventKind` (Gas Leak's truncation notification), sized and documented like the existing four Step 0 kinds — `Round`, `Node`, `Seat` — and excluded from RFC §9.1's writer table for the same reason those four are.
- `internal/rules/movement.go`'s `advance()` and `internal/rules/trail.go`'s `applyRiotPermutation` both change signature to return `[]game.Event`; `resolve.go`'s movement loop and `writeTrail` each gain one `append`.
- M1's golden replay fixtures (#77) need a re-record for every fixture whose match draws Gas Leak or Riot at least once — `Event` construction consumes no RNG indices (D30/D33's own precedent), so this is a fixture-content change, not a determinism risk.
- §22 row 6's measured numbers change at every player count once the fix lands. [#205](https://github.com/garnizeh/cinzal/issues/205)'s exit-demo doc needs its R6 section re-run and re-read against the corrected stream — not re-litigated, since R6's verdict already stands as NO ACTION and already names this under-count as the reason to expect the number to move.
- Implementing task: [#274](https://github.com/garnizeh/cinzal/issues/274) — events, golden fixtures, and the R6 re-run, in one PR.
- Adjacent, out-of-scope finding filed separately: [#275](https://github.com/garnizeh/cinzal/issues/275) — Riot's own missing "your trace moved" player notification.
- **Reversible at low cost.** Nothing here changes game mechanics, RP, Balance, or any resolution outcome — only which events the pipeline emits for occurrences that already happen. Superseding this decision later costs a fixture re-record and an event-shape change, not a rules rewrite.
