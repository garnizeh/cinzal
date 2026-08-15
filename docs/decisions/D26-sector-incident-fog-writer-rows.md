# D26 — Do `EventInformantRing` and `EventSpilledLoadCrate` get their own §9.1 fog-writer rows, or fold into 11 and 13?

**Status:** decided
**Blocks:** Nothing outstanding for merge — [#75](https://github.com/garnizeh/cinzal/issues/75) already landed (`internal/rules: Project and the fourteen authorised position writers`, merged as #144) with both kinds provisionally folded into rows 11 and 13 pending this decision (`internal/rules/anchors.go:19-25`, `internal/rules/fog.go:193-198`). This decision removes that hedge and finalizes RFC §9.1's table.
**Decided:** 2026-08-14
**Issue:** [#142](https://github.com/garnizeh/cinzal/issues/142)

## The question

RFC §9.1 (`docs/project/cinzal-architecture-rfc.md:820-833`) lists fourteen authorised opponent-position writers, each citing its GDD/RFC source. Two already-implemented `game.EventKind` values fit the position-writer shape but appear in neither the table nor its citing prose:

- **`EventInformantRing`** — Informant Ring, GDD §14.3 sector incident (`docs/project/cinzal-gdd.md:1080`): "every player ending in the flagged sector has their position revealed publicly." Same columns as row 11 (Informants, GDD §14.2): global, named, fixes a node.
- **`EventSpilledLoadCrate`** — Spilled Load, GDD §14.3 sector incident (`docs/project/cinzal-gdd.md:1087`): "a crate appears at a random node in the sector... announced publicly." Same columns as row 13 (Dead Runner): global, unnamed, fixes a node.

Both landed with issue #73 (sector incidents), after rows 13-14 were added by #72 and after §9.1's table was last edited — the table's changelog never picked up either. `internal/rules/anchors.go`'s `buildRoundAnchors` already routes both through their partner row's exact case branch, but its own doc comment (`:19-25`) flags this as provisional: "D26 (#142) is open on whether they should be."

Two readings are both defensible from the code as it stands (the issue's own framing): fold into rows 11/13 on the strength of matching distribution/named/node-only columns, or add rows 15/16 because Informant Ring's reveal is conditioned on sector membership rather than unconditional.

## Options

**1 — Fold into rows 11 and 13.** No new rows. `writeAnchors`'s switch and the fog suite's row-per-case structure treat both kinds as their partner row's alternate source; a citation footnote on rows 11 and 13 covers them.

**2 — Two new rows, 15 (Informant Ring) and 16 (Spilled Load).** RFC §9.1 gains both rows, citing GDD §14.3 directly, matching precedent #72 set for rows 13-14. "Fourteen" becomes "sixteen" throughout §9.1 and §16.1.

## Decision

**Option 2.** Both kinds get their own rows — 15 (Informant Ring) and 16 (Spilled Load) — rather than folding into 11 and 13.

| # | Source | Distribution | Names the player? | Fixes a node? |
|---|---|---|---|---|
| 15 | **Informant Ring** incident (GDD §14.3) | Global, once, per eligible seat | Yes | Yes |
| 16 | **Spilled Load** incident, crate placed (GDD §14.3) | Global, once | **No** | Yes — the drawn node |

No functional code change is required. `buildRoundAnchors` (`internal/rules/anchors.go:32-35`) already places `EventInformantRing` in the named/global case alongside `EventInformants` and `EventSpilledLoadCrate` in the node-only/global case alongside `EventDeadRunnerCrate` and `EventFenceWindfallAnnounced` — that grouping is correct and unchanged by this decision, since it groups by disclosure *shape*, not by RFC row number (see Reasoning). What changes is documentation only: the RFC table gains two rows, and the "pending D26" hedges in `anchors.go` and `fog.go` are corrected to cite the resolved rows.

## Reasoning

**A row is defined by its authorised writer — one named GDD source that independently justifies a position disclosure — not by matching distribution/named/node-only columns.** Shape identity between rows is already the norm, not the exception: rows 5 and 6 are column-for-column identical (Global, No, Yes) yet cite different GDD sections (§9.1's own Loitering rule vs §8.4's loose-crate rule) and were never folded. Rows 13 and 14, added together by #72, are also column-for-column identical (Global, No, Yes) — Dead Runner and Fence's Windfall are two distinct GDD §14.2 cards, and #72 gave them two distinct rows specifically because each is its own "announced publicly" card text, not because their disclosure shape differs. If matching columns were sufficient grounds to fold, #72 would have folded Fence's Windfall into row 13 instead of adding row 14 — it didn't, and its own changelog entry (RFC r20→r21) says why: each is a separate "GDD §14.2... card text" citation.

**Row 12 (Decoy, D12) makes the same point more starkly.** Decoy doesn't just match row 1's shape — by D12's own decision, it reuses row 1's *literal* `TrailEntry` `Kind`, byte-identical on the wire, indistinguishable to any reader including the planter. If shape (or even wire-identity) were what defined a row, row 12 would not exist; D12 gave it its own row precisely because it is a distinct writer — a different mechanic with its own gate — that needs its own citable authorization, regardless of how identical its output looks.

**Row 8 is the one real precedent for folding, and it doesn't generalize to this case.** Row 8 ("Item purchased, Infamy ≥ 6") covers every item "regardless of item" because item choice is a *data attribute* of one GDD §7.3 trail-table entry and one `game.EventKind` — there was never more than one EventKind to begin with. `EventInformantRing` and `EventSpilledLoadCrate` are not a data attribute of `EventInformants`/`EventDeadRunnerCrate` — they are separate `game.EventKind` values, kept separate by explicit design. Their own doc comments say so directly: `resolveInformantRing` (`internal/rules/incidents.go:405-407`) is "a distinct Kind so recap/telemetry can attribute the reveal to the incident rather than the global event card," and `anchors.go:24-25` keeps both "as their own `game.EventKind`... not renumbered to their partner row's Kind, so a recap/telemetry consumer can still tell them apart." A row-8-style fold would contradict the exact design intent the implementers already wrote down.

**The code already proves "one RFC row" and "one code branch" are decoupled concepts here, which removes the only real cost of Option 2.** `buildRoundAnchors`'s switch already puts rows 13 and 14 — two RFC rows — in the identical case (`internal/rules/anchors.go:35`: `case game.EventDeadRunnerCrate, game.EventFenceWindfallAnnounced, game.EventSpilledLoadCrate:`). Adding rows 15 and 16 costs nothing at the implementation layer: `EventInformantRing` and `EventSpilledLoadCrate` are already in the correct case branches for their shape. The RFC table is a citation/documentation granularity — it is what the Anchor-parity fog-suite check (RFC §16.1) asserts against per source — not an implementation-grouping granularity, and the two need not move together.

**Informant Ring's sector-conditioned eligibility does not need its own column or justification to get a row.** Rows 9 and 10 already establish that a row can encapsulate an eligibility condition (Infamy tier) internally; the table's Distribution column describes the disclosure's global/sight-gated character and duration, not the population of seats a round's trigger happens to include. Informant Ring qualifying for its own row rests on the same ground rows 13/14 and row 12 already stand on — a distinct named source — not on the sector-conditioning detail the issue's Option 2 reasoning raised as an alternative argument.

**This generalises the way the issue asks for.** Future incident/event cards with a shape matching an existing row fold into it only when the "many" is a data attribute of one already-authorised writer (row 8's shape). A card with its own name, its own GDD citation, and its own `game.EventKind` — kept distinct specifically for attribution, as both of these already are — gets its own row, however identical its columns look next to an existing one.

## Consequences

- RFC §9.1's table gains rows 15 (Informant Ring, GDD §14.3) and 16 (Spilled Load, GDD §14.3). "Fourteen authorised writers" becomes "sixteen" throughout §9.1 (prose and the `writeAnchors` pseudocode's case count) and §16.1 (the Fog test row count and the Anchor-parity test's per-row citation list), matching the precedent #72 set for rows 13-14.
- `internal/rules/anchors.go`'s `buildRoundAnchors` doc comment is corrected: no longer "not literal rows... D26 is open," now cites rows 15 and 16 directly. The switch statement itself is unchanged.
- `internal/rules/fog.go`'s `writeAnchors` doc comment is corrected the same way — "folded... pending D26/#142" becomes a direct row-15/row-16 citation.
- `internal/rules/incidents.go`'s `resolveInformantRing` and `resolveSpilledLoad` doc comments gain an explicit row citation, matching `resolveInformants`' existing "RFC §9.1 row 11" style.
- No GDD change: both cards' printed effect (GDD §14.3) is unchanged and already fully specifies the mechanics this decision cites — the same posture #72 took ("No GDD change... this is the resolution-shape... decision the deck's own text left to whoever built" the fog boundary).
- No RNG, ordering, or gameplay change. This is a documentation/citation resolution only.
