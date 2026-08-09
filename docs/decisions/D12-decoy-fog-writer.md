# D12 — What does Decoy write at the fog boundary, and who does it name?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#50](https://github.com/garnizeh/cinzal/issues/50)

## The question

GDD §12 gives Decoy one line: *"Discard: plant a false 'Cargo left here' trace on any Known node."* §9.4 adds only that it resolves *"with the trail, at the end of the round."* Neither says whether the false trace carries a name, whose Infamy gates it if so, what the planter's own view shows, or which row of RFC §9.1's eleven-writer table it occupies. Every one of those is load-bearing: get the naming question wrong in the permissive direction and a Cr$ 5 item becomes the strongest disclosure primitive in the game.

## Why it is open

GDD §7.3 attaches a name to a real "Cargo taken" entry only at the actor's Infamy ≥ 3 (RFC §9.1 row 1). Decoy imitates that entry, so the same question recurs: whose Infamy gates the fake, and whose name does it carry?

- **Unnamed only** says "somebody took cargo here" — against a Known-and-above player's real, named traces, an unnamed entry is transparent as a fake, because nothing in the fog is normally unnamed once a player crosses Infamy 3. Cheap, but nearly useless at the table configuration where a deduction item should matter most.
- **Named, planter's choice of victim** forges a **named position fix on another player** — RFC §9.1 calls exactly this "the strongest disclosure primitive in the game." Selling it for Cr$ 5 with no gate would hand every player a way to manufacture evidence against a rival who was never there, with no Infamy cost and no risk. It also needs a wholly new gate (may you forge the name of a player below the Infamy that would make their real traces named?) that neither document states.
- **Does it feed the Heat Map and Attribution?** It must, or the item is inert: GDD §7.5's Board and RFC §9.2's `NodeStats` are exactly what a reader would use to weigh a trace, and an entry excluded from both is an entry nobody ever acts on.
- **Does the planter's own view distinguish it?** They know they planted it — the question is whether `Project` treats their seat differently from every other reader's, which is a fog-boundary decision even though the planter isn't being deceived about *their own* facts (GDD §7.1's `SelfState`-is-exact guarantee is about balance, cargo, contracts — not about other seats' derived views).
- **It is an un-enumerated position writer.** RFC §9.1 states the eleven-writer table is exhaustive and checked against GDD §7.3 row for row. Decoy writes a node/name pair and currently has no row. Either it gets one, or the fog suite's central negative assertion — no un-enumerated writer may name a player at a node — flags a legitimate item as a leak, and whoever hits that first is positioned to quietly weaken the assertion instead of fixing the table. [D4](D04-riot-trail-randomization.md) hit the same shape of problem for Riot and answered it by keeping the existing table's discipline intact rather than carving an exception into it; this decision follows the same instinct.

## Options

**A — Unnamed trace only.** "Cargo left here," no name, ever, regardless of the planter's Infamy. Cheap and safe: indistinguishable from a real Infamy 0–2 pickup, and no new gate is needed because nothing is ever named. Cost: against a table of Known-and-above players, whose real pickups are all named, an unnamed Decoy entry is transparently fake the moment anyone reads the log with prior context — the item stops doing anything once the match matures past the opening rounds.

**B — Named, planter's choice of victim.** The item forges another player's presence at a node they never visited. The strongest fit for the word "decoy" read as misdirection *about someone else*. Cost: it is the only mechanism in the game that writes a false named fix on a player who did nothing, it needs a new, carefully worded gate that doesn't exist in either source document, and it turns a Cr$ 5 Black Market item into the exact primitive RFC §9.1 singles out as the game's strongest disclosure — sold at a price point that assumes it's a minor tactical tool.

**C — Named as the planter, self-misdirection only.** The item plants a false trace of *the planter's own presence* at a node they were not at — a fabricated alibi, not a forgery of anyone else's identity. The gate is the ordinary one: the planter's own Infamy ≥ 3, exactly as GDD §7.3 already gates a real pickup. Below Infamy 3, it behaves exactly like Option A. Cost: narrower than B's "forge anyone" reading, but it needs no new gate, no new name-forgery rule, and no new leak surface — it reuses row 1's existing gate rather than inventing a twelfth one.

## Decision

**Option C.** Decoy plants a trail entry that is **structurally a real "Cargo taken" entry** (same `TrailEntry` shape, same kind — see Consequences), located at the Known node declared when the item is discarded, and it names **only the planter**, never another player.

**The name gate is row 1's own gate, applied to the planter.** The entry is named if and only if the planter's own Infamy is ≥ 3 at the moment the trace resolves. GDD §9.4 fixes that moment explicitly — "with the trail, at the end of the round" — so per §11.1's precedence rule ("evaluated at the moment they fire, using your Infamy at that moment"), the check uses the planter's Infamy as it stands at end-of-round resolution, after that round's other Infamy deltas (deliveries, confrontation results, stakes, Vanish, Debt) have already applied. This can differ from when a *real* pickup's own gate is evaluated earlier in the same round's resolution — that asymmetry is a direct consequence of GDD §9.4 pinning Decoy's own resolution point explicitly, not an oversight, and it never becomes visible to any reader (see below).

Below Infamy 3, the planted entry is unnamed — GDD §7.3's ordinary "Cargo taken" behaviour at that tier — which is Option A's behaviour exactly, arrived at for free rather than as a separate case.

**Distribution: sight-gated, same as row 1.** Only a reader with sight of the declared node in the round the entry resolves can see it — the planter included, with no exception. `Project` does not branch on "is this seat the planter" anywhere. If the planter lacks sight of their own declared node that round, their own view shows nothing there, exactly like any other reader without sight; they already know, from their own order log, that they discarded a Decoy there, and the fog boundary owes them no annotation beyond what it owes any other reader of any other trail entry.

**Feeds `SeatArchive` and `NodeStats` identically to a real Cargo Taken entry**, for every reader with sight — the Log lists it, the Heat Map's traffic-rate denominator and numerator both treat it exactly like a genuine pickup. This falls out automatically from the structural-identity requirement below; nothing new needs to be built in `writeTrail`'s archive path.

**Indistinguishable to readers, including the planter, as a structural guarantee, not a policy.** Decoy's `TrailEntry` must use the *same* `Kind` value as a real Cargo Taken entry — not a new `KindDecoy` that happens to render the same string. A distinct kind value would itself be the leak: any reader-side code branching on kind (now or in a future feature) could distinguish a Decoy from a real pickup even if the rendered text is identical. The fabricated/real distinction exists only inside `MatchState`'s internal record (for engine bookkeeping, determinism tests, and `//go:build debug` tooling per RFC §15.1) and is stripped at `Project` — it never becomes a field, an ordering artefact, or a serialisation difference in `PlayerView`.

## Reasoning

**C reuses an existing gate instead of inventing one.** B's cost is not just narrative — it requires a new authorisation rule (who may a player forge, at what Infamy, against what real Infamy the victim would need for a genuine trace to be named) that neither document states and that this decision would have to invent whole-cloth, on the specific axis RFC §9.1 already flags as the most dangerous kind of writer to get wrong. C's gate is "your own Infamy ≥ 3," already fully specified by GDD §7.3 for the entry Decoy imitates.

**Self-misdirection is the reading that survives contact with "Decoy" as a word.** A decoy is something that draws attention away from where you actually are — a false alibi is exactly that. Forging another named player's presence is a different mechanic (something closer to entrapment or frame-up), priced and gated like neither document does, and not what either card entry (GDD §9.4, §12) describes.

**Reusing row 1's `TrailEntry` kind, rather than adding a distinct one, is what makes "indistinguishable to readers" true by construction instead of by discipline.** A negative fog test (RFC §16.3's method: serialise the view to JSON, assert no distinguishing byte) can only pass reliably if there is structurally nothing to find — a separate kind value that happens to be filtered to the same string at render time is one refactor away from a leak. Making Decoy *be* a Cargo Taken entry, not merely *look like* one, removes the failure mode instead of testing around it.

**No special case for the planter's own seat, because `Project` special-casing "your own item's effects" is a precedent this codebase should not set once.** The planter already possesses the one piece of information that matters (they know they used the item) through their own order history, which is outside the fog boundary entirely. Anything `Project` adds on top — a flag, a differently-shaped entry, a note — is machinery that exists solely to serve one seat's convenience and is exactly the kind of per-seat branch RFC §9.3's fog suite is built to catch on every *other* writer; there is no reason Decoy should be the one exception.

## Consequences

- **RFC §9.1's table gains row 12**: Decoy (self-named false Cargo Taken), Distribution: Sight-gated, Names the player: Yes, self only, iff planter's Infamy ≥ 3 at end-of-round resolution, Fixes a node: Yes — the declared Known node. The "eleven authorised writers" sentence becomes twelve, and the cross-check against GDD §7.3 gains a line noting Decoy imitates row 1's gate rather than introducing a new one.
- **`writeTrail` gains one construction path**: on Decoy's end-of-round resolution, append a Cargo-Taken-kind `TrailEntry` at the declared node, actor = the planter, named computed by the same name-gate helper row 1 already uses, evaluated against the planter's Infamy at that point in resolution. No new `TrailEntry` variant, no new field.
- **`MatchState`'s internal record may carry a fabricated/synthetic marker for engine-internal use** (determinism tests, `//go:build debug` tooling), but `Project` must strip it unconditionally — this is the one thing the fog suite's new test for this item asserts.
- **The fog suite (RFC §16.3) gains a negative test specific to this item**: construct a match with one real Cargo Taken entry and one Decoy at different nodes a shared reader has sight of, serialise both readers' views to JSON, and assert no field, ordering, or byte sequence distinguishes the fabricated entry from the real one.
- **No RNG consumption.** The declared node and the planter's identity are both deterministic from player choice; [D3](D03-rng-consumption-table.md)'s table needs no new row for Decoy.
- **GDD §9.4 and §12 need no textual change.** Both already state the mechanics this decision assumed (resolution timing, target restricted to a Known node); this decision closes only the fog-boundary ambiguity RFC §9.1 left open around them.
- Reversible at low cost while `writeTrail` and `Project` are unwritten; expensive after, for the same golden-replay reason every entry in this log carries — a stored match with a Decoy in its order log bakes in whichever naming behaviour this decision fixes.
