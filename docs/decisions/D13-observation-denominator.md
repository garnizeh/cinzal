# D13 — Do Rain and Blackout rounds count toward the Heat Map's observation denominator?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#51](https://github.com/garnizeh/cinzal/issues/51)

## The question

RFC §9.2 builds `NodeStats.ObservedRounds` — the Heat Map rate's denominator — from `SeatArchive.Sight`, on the premise that *"sight with no traffic is itself an observation"*: watching a chokepoint for six rounds and seeing nothing cross is exactly the evidence the rate is built from. Two CITY event cards break that premise:

- **Rain** (GDD §14.2): *"No 'fresh tracks' entries are recorded anywhere this round. The weather washes the board clean."*
- **Blackout** (GDD §14.2): *"Next round nobody generates trail entries and nobody has sight beyond their own node."*

Under either, a node a seat had sight of can read as zero-traffic for a reason that has nothing to do with whether traffic actually happened. Do rounds affected by either card enter `Sight` — and the rate it drives — at all? And precisely which observations, not just which rounds, does the answer exclude?

## Why it is open

**Rain's own text is narrower than "the round doesn't count."** It suppresses exactly one trail-entry kind — "fresh tracks," GDD §7.3's entry for a node someone merely passed through. Every other kind (cargo taken, confrontation, delivery, post staked, lease expired, item purchased, Loitering) fires normally that round. So a node where a confrontation happened during a Rain round still carries a real, uncorrupted entry — Rain never touched it. The corruption is narrower and sharper than "Rain rounds are unreliable": it is specifically *a node whose only possible evidence this round was a pass-through, and that pass-through's entry was the one thing Rain erases.* A node with no traffic at all reads as zero with or without Rain — Rain has no effect on an honest zero, because nothing there would have generated an entry regardless.

**Blackout is broader but the same shape.** *"Nobody generates trail entries"* is a blanket suppression, not fresh-tracks-only — but it only matters where something real would otherwise have been logged. GDD §7.2 restricts sight itself to "your own node" that round, so the only place this can bite per seat is their own ending node. If nothing happened there, Blackout changes nothing about the reading. If something did (a pickup, a confrontation, anyone merely passing through), the entry that would have recorded it is gone, and the node looks silent for a reason the seat had no part in.

**RFC §6.7's pipeline puts `writeTrail()` before `globalEvent()`.** `writeTrail()` — where the archive accumulates — runs before Phase 6, where the round's global event card (Rain, if it's the draw) is popped and applied. Read literally, `writeTrail()` cannot know Rain is about to fire. This looked, on first pass, like it might force a retroactive correction after the fact — the same shape GDD §14.2's Curfew already uses ("−1 step for everyone this round, applied retroactively"). It doesn't: RFC §6.4 states the event deck is fully shuffled and ordered in `initial(seed, cfg)`, and *"Phase 1 **peeks** at the head of each deck to print the Headline... Neither [peek nor pop] consumes an index."* The card `writeTrail()` needs to know about is already fixed and non-secretly available — it is the identical peek Phase 1 already performed earlier in the same round, just re-read. No retroactive machinery is needed, only a read.

**A third case the issue doesn't name has the identical shape and needs the identical answer.** GDD §14.2's **Festival** ("Anyone ending there this round gains +1 Infamy and leaves **no trace**") and GDD §14.3's **Distracted Guard** ("+1 step next round and leaves **no trace** this round") join **Vanish** ("You leave no 'fresh tracks' trace this round," §9.1) as a third instance of an actor suppressing *their own* trace. All three are player-level: the world's trail-generation is not suppressed, only that one player's entry is, and a watcher with genuine sight of the node gets a genuine, informative zero — "nobody left a trace here" is exactly true, because the evasion worked. The issue names the boundary the rule needs to draw ("the world was unobservable" versus "somebody hid successfully") but only cites two of the three cards on the wrong side of it.

## Options

**A — Exclude suppressed rounds from the denominator, per round, uniformly.** Whenever Rain or Blackout is active, every node a seat has sight of that round is dropped from `Sight` entirely — one boolean gate in `writeTrail`, regardless of what actually happened at any given node.

- For: trivial to state, trivial to test, matches the issue's own Option A text exactly.
- Against: throws away real signal on both sides of the honesty question it's trying to fix. A node with a confrontation during a Rain round has a real, uncorrupted entry — Rain never touched it — and excluding the round drops it anyway. A node with genuinely no traffic during a Rain round was never going to log anything, Rain or not, and excluding it discards an honest zero that was never at risk. Both losses are unforced: the only case that actually needs fixing is the narrower one below.

**A′ — Exclude only the specific (node, round) pairs where suppression erased something real.** A node stays in `Sight` — as either a positive or an honest zero — whenever what a seat would see there this round is unaffected by Rain/Blackout. It moves to a separate bucket only when a real trail-entry event genuinely occurred there this round and Rain's or Blackout's suppression is the reason none survived to be recorded.

- For: matches RFC §9.2's own honesty standard exactly — a node is untrustworthy only when the reading is actually corrupted, not merely because a suppression card happened to be live somewhere in the match that round. Reuses a fact `writeTrail` already computes for ordinary entry generation (whether an entry would fire at a node this round); does not require inspecting anything not already in hand at that step.
- Against: needs `writeTrail` to retain a per-node "would this have logged something" fact even when the entry itself is suppressed, rather than a single per-round flag. More moving parts than A, though every one of them is state the step already touches.

**B — Count them, and surface them.** The denominator is literal; the Board shows *"4 of 6 rounds observed · 2 rounds obscured."* Honest in the raw sense, but the render-side work is M5's, so the M1 decision is only to store enough that B stays available later without a schema change.

**C — Count them and say nothing.** The default by omission, and the option this decision exists to rule out: it produces a number the game states is trustworthy and is not, which GDD §7.5 already singles out as the exact failure the rate was invented to avoid.

## Decision

**Option A′, plus B's storage (no display), plus explicit confirmation that Vanish, Distracted Guard and Festival need no special case at all.**

**The exclusion is per (seat, node, round), not per round.** For a node a seat had sight of this round:

- If nothing at that node was suppressed by Rain or Blackout — because nothing happened there, or because something happened and its entry kind isn't one either card touches — the round enters `Sight` for that node exactly as it always would: a positive if an entry is present, an honest zero if not.
- If a real trail-entry event occurred at that node this round and Rain's or Blackout's suppression is the reason no entry survives to be recorded, the round does **not** enter `Sight` for that node. It enters a new, parallel record instead (below), and neither `ObservedRounds` nor `TrafficRounds` counts it.

Concretely, for a node with sight this round: let *before* be the set of trail-entry events that genuinely occurred there, computed from movement, actions and deliveries exactly as `writeTrail` already computes it to decide what to emit; let *after* be the subset Rain/Blackout actually let through (Rain removes fresh-tracks only; Blackout removes everything). The node is excluded from `Sight` this round iff *before* is non-empty and *after* is empty. If *before* is empty, or anything in *after* survives, the round enters `Sight` unmodified — including the case where *after* is empty because *before* was already empty, which is an honest zero and not this decision's concern at all.

**Vanish, Distracted Guard and Festival never populate the exclusion, regardless of outcome.** These suppress the *acting player's own* entry as the deliberate, designed result of a successful evasion — not a world-level event erasing something that was going to be recorded. From a watcher's `Sight`, "nothing here" is exactly true and exactly the intended reading (see Reasoning). `writeTrail`'s *before*/*after* computation for archive purposes only ever runs for Rain's and Blackout's suppression; these three cards are invisible to it.

**New storage, parallel to `Sight`, not a modification of it:**

```go
type SeatArchive struct {
    Sight     map[NodeID]RoundSet    // unchanged: rounds this node gave an honest reading
    Obscured  map[NodeID]RoundSet    // NEW: rounds a real entry at this node was erased
                                      // by Rain or Blackout specifically — never by
                                      // Vanish, Distracted Guard or Festival
    Trail     []StampedTrailEntry    // unchanged
}

type NodeStats struct {
    ObservedRounds int   // |Sight[node]|                — unchanged formula
    TrafficRounds  int   // rounds in Sight carrying a tracks entry — unchanged formula
    ObscuredRounds int   // NEW: |Obscured[node]|
}
```

`Obscured` is fog-sensitive in exactly the same way `Sight` is — which nodes a seat could even be eligible to have something erased *from* depends on that seat's own sight that round (sharpest under Blackout, where only one node per seat is ever eligible) — so it belongs inside the per-seat `SeatArchive`, not as match-wide `PlayerView` state. `Project` ships only the requesting seat's `Obscured`, exactly as it already does for `Sight`.

**No RNG consumption change.** Determining whether this round is Rain is a re-read of Phase 1's already-performed, non-consuming peek at the event deck's head (RFC §6.4); determining Blackout is a read of the existing next-round-modifier flag ([D5](D05-upkeep-phase.md)). Neither needs a new draw or a new index in [D3](D03-rng-consumption-table.md)'s table.

**M5's display (Option B) is not decided here.** This decision only guarantees `ObscuredRounds` exists per node, so a future "4 of 6 rounds observed · 1 obscured" render needs no new field when M5 gets there — the same posture D11 and D12 both took toward their own downstream consumers.

## Reasoning

**A′ costs one more field than A, and A costs real signal on both sides of the line it draws.** A round-level flag is the option a first read of the issue's own text suggests, and it is defensible on simplicity alone — but tracing the cards' actual text shows it throws away exactly the data RFC §9.2 exists to protect: an honest zero at a node Rain never touched, and a real positive that survived Rain's narrower suppression. Both losses are avoidable at the cost of computing, per node, a fact `writeTrail` already needs for an unrelated reason (deciding what to emit) — reusing it for the archive decision is not new work, it is a second read of an existing answer.

**Vanish/Distracted Guard/Festival are excluded from the mechanism by construction, not by a special-case check.** GDD's own reasoning for Vanish states the point directly: *"Vanish suppresses 'fresh tracks' entries, which are about movement... the exposure cost of the Infamy ladder finally paying off in the fiction"* — successful evasion is designed to be indistinguishable from absence, and a watcher correctly reading "nothing here" off a node where a Legend just Vanished is the mechanic working, not a leak the archive needs to correct for. Rain and Blackout have no equivalent design intent — nobody chose for the board to go dark, it is simply an event that happened to them — which is the actual axis this decision draws the line on: *whose choice erased the entry*, not merely whether one was erased. Because the *before*/*after* computation this decision defines only ever runs against Rain's and Blackout's suppression sources, these three cards need no check to exclude them — there is nothing in their resolution path that could ever populate `Obscured`.

**Riot was checked and does not raise the same problem.** GDD §14.3's Riot (D4) permutes which node a sight-gated entry is attached to within the flagged sector; it never deletes an entry's existence, so it never turns a real positive into an apparent zero the way Rain and Blackout do. A node under Riot still gets a real entry — possibly a different one than the node that actually generated it — and `Sight` reads it exactly as any other trail entry. No exclusion applies.

**The peek resolves what looked like a pipeline ordering bug.** Read narrowly, RFC §6.7's `writeTrail() → globalEvent()` ordering makes it look like `writeTrail` cannot possibly know Rain is coming — the natural fallback would have been retroactive correction after `globalEvent()` runs, the same shape Curfew already uses for its own "applied retroactively" step penalty. That fallback is unnecessary here: RFC §6.4 already establishes that the whole event deck is fixed at Setup and that Phase 1's Headline peek is non-consuming — the same peek is available, for free, at any later point in the same round, including inside `writeTrail`. This is worth stating plainly because the alternative (inventing a second retroactive-correction path alongside Curfew's) would have been real, avoidable complexity for a problem the architecture had already solved.

**`Obscured` lives beside `Sight`, not folded into it, because they answer different questions a reader needs kept separate.** `Sight` is "did this seat get an honest reading of this node this round" — zero or positive, both trustworthy. `Obscured` is "did this seat lose a reading it would otherwise have earned, to a cause outside their control." Merging them would force a choice between under-counting (treat obscured as excluded from everything, A's cost) or over-counting (treat it as an honest zero, exactly the corruption this decision exists to prevent) — keeping them as parallel sets costs one more map and answers both questions correctly.

## Consequences

- **RFC §9.2's `SeatArchive` gains `Obscured map[NodeID]RoundSet`; `NodeStats` gains `ObscuredRounds int`.** Both are the same shape and cost as the fields they sit beside — RFC §9.2's own "size is a non-issue" arithmetic (25 nodes × 15 rounds, a 15-bit set per node per seat) applies unchanged to the new map.
- **`writeTrail`'s archive step gains one per-node check**, reusing the entry-generation computation it already performs: for each node in a seat's sight this round, if a real entry existed pre-suppression and none survives Rain's or Blackout's suppression specifically, record the round in `Obscured[node]` instead of `Sight[node]`. No change to entry generation itself, no change to `Trail`/the Log, no new writer row in RFC §9.1's twelve-row table — this decision touches only the archive's bookkeeping, not what crosses the fog boundary as a trail entry.
- **RFC §16.1's Archive test row** (currently: *"A node watched 6 rounds with traffic in 4 reports 4/6; a node watched with no traffic reports 0/N, not absence"*) gains three cases: a node with a real, non-fresh-tracks entry during a Rain round reports the entry normally (Rain's narrower suppression doesn't touch it); a node with genuinely no traffic during a Rain or Blackout round reports an honest zero, not an exclusion; and a node whose only would-be entry (fresh tracks under Rain, anything under Blackout) is erased reports neither a zero nor a positive — it is absent from `Sight` and present in `Obscured`, and `ObservedRounds` for that node does not include that round at all. A fourth, negative case: a node where Vanish, Distracted Guard or Festival suppressed the *acting* player's entry reports an ordinary honest zero to every other watching seat — never `Obscured`.
- **No change to RFC §9.1's authorised-writer table.** This decision governs the observation archive's bookkeeping (§9.2), not which entries cross the fog boundary as trail entries (§9.1) — the two are already independent, and Rain/Blackout neither add nor remove a writer row.
- **No RNG consumption change**, and no new row in [D3](D03-rng-consumption-table.md)'s table — see Decision.
- **GDD §14.2 and §14.3 need no textual change.** Rain's, Blackout's, Vanish's, Distracted Guard's and Festival's card text already state the mechanics this decision assumes; this decision closes only the archive-bookkeeping ambiguity RFC §9.2 left open around them.
- Reversible at low cost while `writeTrail` and `Project` are unwritten; expensive after, for the same golden-replay reason every entry in this log carries — a stored match's Heat Map bakes in whichever exclusion rule ran when its rounds were folded.
