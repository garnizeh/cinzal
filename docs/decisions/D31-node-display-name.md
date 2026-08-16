# D31 — How does a node acquire its display name?

**Status:** decided
**Blocks:** nothing in M1's implementation; blocks any fog test that asserts on `NodeView.Name`, and blocks M5's map rendering
**Decided:** 2026-08-16
**Issue:** [#154](https://github.com/garnizeh/cinzal/issues/154)

## The question

`Node.Name` (`internal/rules/state.go:17`) and `NodeView.Name` (`internal/game/view.go:233`) both exist, and `NodeView.Name`'s own field comment says it is "disclosed from Rumoured onward" — matching GDD §7.1's fog table, which lists a node's name as one of exactly three things a Rumoured node reveals, alongside type and sector. **Nothing ever assigns either field.** `newGraph` (`internal/rules/initial.go:83-96`) converts `gen.Graph` into `rules.Graph` and leaves `Name` at its Go zero value, with the gap flagged in its own doc comment rather than papered over. `projectNodes` (`internal/rules/fog.go:107`) faithfully copies that empty string into every visible `NodeView`. So today every node in every `PlayerView` discloses `Name: ""` — GDD §7.1's disclosure table promises a name and the game currently reveals an empty one.

How does a node acquire the display name both structs already assume it has?

## Why it is open

Both ends of the pipeline assume the field is populated, but nothing in the GDD or the RFC specifies the mechanism, and inventing one inside `newGraph` would be a rules decision smuggled into setup plumbing — exactly the kind of thing D03 exists to stop people answering independently. The setting material exists and is unused: GDD §3 names four districts with distinct character and a "predominant nodes" column, and §6.2 fixes the node types and their per-map counts. A name is presumably some function of (type, sector, ordinal), or a draw from a per-district pool — but which, and whether it costs an RNG index, was undecided.

## Options

**A — Deterministic function of (`Sector`, `NodeType`, ordinal), computed at setup.** E.g. `"Old Docks Warehouse 2"`. Zero RNG cost, reproducible without any new state, and needs no new vocabulary: `game.Sector.String()` and `game.NodeType.String()` (`internal/game/enums.go:33-46`, `:64-76`) already return exactly GDD §3's and §6.2's own names. Cost: names repeat in shape across matches — flavourless but honest, and the issue's own framing of this option.

**B — Drawn from per-district name pools at generation.** Richer flavour, closer to a real place name. Cost: a new `Purpose`, a new row in RFC §6.4's consumption table (and in `ConsumptionTable`, `internal/rules/rng_purpose.go`), a determinism story for draw order, and — the real cost — a pool of authored names large enough for the biggest supported map (§6.2's table tops out at 28 nodes) that does not currently exist anywhere in the project. GDD §21's randomness inventory, which itemises all fifteen current sources of randomness by name, lists nothing resembling node naming; that silence is itself evidence nothing currently expects a draw here.

**C — Not a rules concern — `internal/render` derives a label from type and sector.** Cost: contradicts `NodeView.Name` existing at all, and contradicts GDD §7.1 treating the name as disclosed *information* on equal footing with type and sector, not decoration layered on afterward. Removing it would mean correcting GDD §7.1's table and removing two struct fields already written against the "disclosed" reading.

## Decision

**Option A.** A node's name is `"{Sector} {NodeType} {ordinal}"` — the district it sits in, its node type, and its 1-based rank among nodes sharing that same (Sector, NodeType) pair, assigned by scanning `Graph.Nodes` in ascending `NodeID` order (the order the struct's own invariant, `Nodes[i].ID == game.NodeID(i)`, already fixes). E.g. the second Warehouse encountered in Old Docks is `"Old Docks Warehouse 2"`. Computed once, in `newGraph`, from facts `rules` already owns at that point (`n.Sector`, `n.Type`, and iteration order) — no RNG draw, no new `Purpose`, no RFC §6.4 row.

## Reasoning

**The GDD's own prose already talks about nodes this way, not as authored place names.** §2 and §7.1 both refer to "the Docks Warehouse" when illustrating a scenario (`docs/project/cinzal-gdd.md:296`, `:401`) — sector-plus-type phrasing, not an invented proper noun. Nothing in either spec ever names a specific node the way it names factions ("The Ravens") or items — the one passage that reads like it might (§7.4's worked example, "Fifteen Overpass," "Water Tank Alley") is illustrative fiction dramatising a player's internal monologue, not a literal in-game string the rules commit to reproducing. Option A is the more literal reading of what the specs actually assume a node is called.

**The vocabulary already exists and is already correct.** `Sector.String()` and `NodeType.String()` return GDD §3's and §6.2's exact names today. A naming scheme built on them can't drift from the spec's own terms — if a sector or type is ever renamed, the fix is in one place, not in a second, parallel name table.

**Cost is asymmetric, and it isn't close.** Option A is a pure function of data `rules` already computes, costing nothing in the consumption table — matching D03's own standing principle that a pure function of already-determined facts is free, and only a genuine draw needs a `Purpose` and a row. Option B needs all of: a new `Purpose`, a new RFC §6.4 row, a determinism story for draw order, and — unlike every other line item in this decision — content that does not exist yet. Authoring a per-district name pool deep enough for the largest map (28 nodes; §6.2's allocation table puts as many as 9 of one type in play at once) is real creative work with no current owner, disproportionate to what is fundamentally a "populate a struct field" plumbing gap in `internal/rules`, the package this project keeps deliberately thin.

**A name isn't hiding new information relative to what's already disclosed alongside it.** GDD §7.1 discloses name, type, and sector together, starting at Rumoured. Type and sector are already committed, RNG-free facts about the node by the time a name would be assigned; a name built from exactly those two facts plus a stable ordinal reveals nothing a Rumoured viewer doesn't already have from the other two fields in the same row of the same table. Option B's richer, independently-drawn name would be the only thing in that triad carrying its own randomness — an inconsistency the spec gives no reason to introduce.

**Rejecting C:** GDD §7.1's disclosure table is explicit that a name is *information*, on the same row as type and sector, not a rendering choice made downstream of the rules. Treating it as render-only would require correcting that table and deleting `Node.Name`/`NodeView.Name`, a bigger and harder-to-reverse move than Option A, for no benefit Option A doesn't already deliver — `rules` computing the name from facts it already owns keeps `internal/render` exactly as thin as D01 wants it, without touching the fog contract.

## Consequences

- **GDD §6.2 gains one paragraph** (this decision's own PR) stating the naming scheme and cross-referencing D31 — the changelog entry documents the derivation, not a new player-facing rule beyond "nodes have names, disclosed with type and sector."
- **`internal/rules` gains one small pure helper**, called from `newGraph` (`internal/rules/initial.go`), that assigns every `Node.Name` from `(Sector, Type, ordinal)` before `MatchState` is returned from `initial`. No RNG consumption changes; `ConsumptionTable` (`internal/rules/rng_purpose.go`) needs no new row. This is a task, landed separately from this decision, matching how D28's `blockedBordersThisRound` and D11's `Config.Suppress` behaviours landed in their own PRs after their decisions.
- **`projectNodes` (`internal/rules/fog.go:107`) needs no change** — it already copies `n.Name` into every visible `NodeView`; once `newGraph` stops leaving `Name` at zero, that existing copy starts carrying a real value. No new row in RFC §9.1's authorised-writer table, since `NodeView.Name`'s own field comment already documented it as disclosed.
- **The fog test #154 was filed to unblock** (feeding #78) should assert absence below Rumoured and a non-empty, well-formed name from Rumoured onward — not a fixed string, since the exact value is seed- and map-dependent by construction.
- Reversible at low cost: like D27, moving to a drawn name later (Option B) is a superseding decision, not a rewrite — `newGraph`'s naming call site is the only thing that would change, and nothing downstream (`projectNodes`, `NodeView`, any renderer) cares how the string was produced.
