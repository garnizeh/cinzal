# D48 — Does D44's completeness check reach `Config`'s arrays and map keys?

**Status:** decided
**Blocks:** the `Config` codec in `internal/store` (#315, #318), and #328's golden fixture, which is only as trustworthy as the completeness check behind it
**Decided:** 2026-08-26
**Issue:** [#345](https://github.com/garnizeh/cinzal/issues/345)

## The question

[D44](D44-config-order-jsonb-encoding.md) answers "does an incomplete `Config` fail or default" with **it fails**, built on one mechanism: an exact key-set comparison against a frozen key list, applied recursively into every nested object the frozen shape names. `game.Config` has two field shapes that are not objects, and D44's mechanism as written does not reach either:

**Fixed-size arrays** (`StepsByTier [4]int`, `CooldownByTier [4]int`, `Contracts [4]ContractTier`) marshal as JSON arrays. Decoding `[4,4,3]` into a `[4]int` fills the first three elements and leaves the fourth at zero, no error — `DisallowUnknownFields` says nothing about array length, and a JSON array *longer* than the Go array is silently truncated with equally no error. D44 names `Contracts[i]`'s key sets but never the array's length, so a three-element `contracts` array passes both halves of D44's check while `Contracts[3]` — Tier IV — reads as an all-zero `ContractTier`.

**Int-keyed maps** (`PostCapByPlayers map[int]int`, `MapByPlayers map[int]MapSpec`) marshal as objects whose keys are player counts. D44 checks each *value* against `MapSpec`'s frozen key list, never the map's own key set. A payload whose `map_by_players` is missing its `"5"` entry decodes into a three-entry map with no error at any level.

## Why it is open

**This is the `LeaseCostPerBlock` scenario D44 exists to close, at array and map grain.** D44's own Reasoning makes the argument and stops one shape short: *"checking only the outer object leaves the identical hole one level down: `Contracts[0]` missing `payment` decodes exactly as silently as top-level `Config` missing `LeaseCostPerBlock`."* The same sentence is true again with "`Contracts` missing its fourth element" or "`map_by_players` missing its 5-player entry" substituted in, and the recursive key-set mechanism as specified catches neither — it only ever loops over elements/keys that are *present*, so an absent trailing array element or an absent map key is invisible to it by construction, not by oversight in an individual check.

**`Config.Validate` is a partial mitigation and cannot be the answer.** `Validate(players)` rejects a `Config` with no `MapByPlayers`/`PostCapByPlayers` entry, but only for the one player count it is called with — a 3-player match reloads happily from a config whose 5-player map entry was lost, and nothing notices until a different match is created from a copied config. `Validate` checks nothing about `StepsByTier`, `CooldownByTier`, or `Contracts` length. D44's whole posture is that the codec must prove it read exactly what version N wrote, not that the result happens to be servable for whichever player count is asked first.

**Roadmap §5 guarantees this recurs.** "Config as data" promises the struct keeps growing; a future tunable that is an array or a map inherits the same hole unless the rule is stated by shape rather than by example.

## Options

**Q1 — bolt array/map checks on beside D44's object check, or extend the same walk to be shape-aware?**

- *A separate pass*, dedicated to arrays and maps, run alongside D44's existing object key-set walk. Two passes over the same decoded tree, and two places a future field addition has to remember to update — the exact failure mode ("forgot to mirror it") D44's own Reasoning already rejects for the codec-in-`store` DTO option.
- *One recursive, shape-aware walk*: given a field's declared kind (object / fixed array / int-keyed map) plus its frozen key list, length, or key set, the same function checks whichever completeness property applies, and recurses into element/value types that are themselves objects using the identical object check D44 already specifies. One function, one frozen-shape declaration per version. Chosen — this is D44's own "recursive, not outer-object-only" framing, completed rather than duplicated.

**Q2 — where does the frozen legal player-count set (`{2,3,4,5}`) come from?**

- *Derive it from wherever the game's player-count bounds live today* (e.g. `game`'s own min/max-player constants). Ties a frozen wire fact for version N to a live value that can change for a reason having nothing to do with any stored `Config` — the identical "not that list; it is today's tuning" problem D44 already named and rejected for `DefaultConfig()` as a source.
- *A literal, hand-written key set per version*, declared alongside the frozen struct type and object key lists `store` already maintains for that version. No dependency on anything that can move out from under an already-shipped version. Chosen.

**Q3 — does tightening the check change what an already-decided `Config` version means?**

- *Treat it as a new version*, bumping the version int and adding a migration. Wrong tool: nothing about what version 1 *means* has changed — a three-element `contracts` array or a `map_by_players` missing `"5"` was never a legal version-1 encoding under D44's stated intent, only under its under-specified implementation.
- *Treat it as a correction to the decode mechanism*, with no version change. A version-1 payload that already carried all four contracts and all four map entries decodes exactly as before; one that didn't now fails where it previously silently succeeded, which is what D44 already promised it would do. Chosen.

## Decision

**D44's completeness check is restated over every JSON shape a frozen `Config` version can produce, not over nested objects alone**, as one recursive, shape-aware walk:

1. **Object fields** (`Config` itself, `ContractTier`, `MapSpec`, `ScavengingTable`, `PressureConfig`, `SubsystemSuppression`): exact key-set match against the frozen key list for that type and version — unchanged from D44.

2. **Fixed-size array fields** (`StepsByTier`, `CooldownByTier`, `Contracts`): the field's `json.RawMessage` is unmarshalled into `[]json.RawMessage`, and its length must equal the frozen length for that field **exactly** — not decoded directly into the Go `[4]int`/`[4]ContractTier` target, since that decode is precisely the operation that silently zero-fills a short array and silently truncates a long one. If the element type is itself an object (`Contracts`' `ContractTier` elements), each element additionally gets the object key-set check from (1), recursively. Array length and per-element completeness are two different checks; both are required, and neither substitutes for the other — a `Contracts` array with the right length but a malformed `Contracts[2]` needs (1), and a `Contracts` array with four well-formed-looking elements masking a missing fourth needs this clause.

3. **Int-keyed map fields** (`PostCapByPlayers`, `MapByPlayers`): the field's `json.RawMessage` is unmarshalled into `map[string]json.RawMessage` — JSON object keys are always strings, and `encoding/json` already renders a Go `int` map key as its decimal string on encode, so no numeric parsing is needed to compare key sets — and that key set must equal the frozen legal key set for that field and version **exactly**, the same not-a-superset-not-a-subset rule D44 already applies to objects. If the value type is itself an object (`MapByPlayers`' `MapSpec` values), each value additionally gets the object key-set check from (1), recursively.

4. **The frozen length (arrays) and frozen key set (maps) are literal, hand-written data**, declared per version alongside the frozen object key lists D44 already requires `store` to maintain — never computed from `len(DefaultConfig().Contracts)`, `maps.Keys(DefaultConfig().MapByPlayers)`, or any other live Go value. This is D44's own argument against sourcing key lists from `DefaultConfig()` (*"`DefaultConfig()` is not that list; it is today's tuning"*), extended to the two shapes D44's audit didn't reach. A future `Config` version with a different tier count or a different supported player-count range gets its own frozen length and key set, exactly as it would get its own frozen object key lists.

5. **This is a correction to D44's decode mechanism, not a new `Config` version.** No migration, no version bump, and no change in outcome for any payload that already satisfied D44's stated intent — a version-1 `Config` with all four contracts and all four player-count map entries decodes exactly as it did before this decision; one missing a trailing array element or a map key now fails where the under-specified mechanism previously let it through.

**Required fixtures (two, alongside D44's two and [D47](D47-order-enum-zero-wire-encoding.md)'s one):** a `Config` payload whose `contracts` array has three elements — Tier IV entirely absent — asserted to fail decode; and a `Config` payload whose `map_by_players` omits the `"5"` key, asserted to fail decode. Each exercises one of this decision's two new checks specifically, the same way D44's own two fixtures exercise the nested-object key-set check and D47's exercises the enum-omission convention.

## Reasoning

**Why one shape-aware walk rather than a separate array/map pass.** D44 already chose recursion over a flat outer-object check for exactly this reason — a second, parallel mechanism is a second place for a future field to be added and only wired into one of the two. Extending the same walk with a per-field "what shape is this" branch keeps the completeness guarantee stated once, next to the frozen data it reads.

**Why frozen literal data rather than derived.** A derived key set (from `DefaultConfig()`, or from wherever the game's player-count bounds live) answers "what does the code think is valid today," not "what did version N actually promise to contain when it shipped." Those two answers are supposed to diverge the moment tuning changes something the frozen version must not — that divergence is the entire reason D44 introduced frozen, hand-written data as a category distinct from anything live in the first place.

**Why this doesn't need a version bump.** A version's frozen shape is a claim about what a payload declaring that version is allowed to contain — D44's array/map gaps were a bug in how faithfully the mechanism checked that claim, not a change to the claim itself. Rows that were genuinely complete under version 1 continue to decode; rows that were already missing a required element or key were never valid version-1 encodings and simply stop being misread as one.

**Why array length needs its own check independent of per-element completeness.** `encoding/json`'s zero-fill-short / truncate-long behavior on a fixed-size Go array happens *before* any per-element check gets a chance to run — by the time code is looking at a decoded `[4]ContractTier`, a short input array has already become a `ContractTier{}` zero value at index 3, indistinguishable from an explicitly-provided all-zero tier. The only place to catch the omission is before that decode happens, against the `[]json.RawMessage` form, which is why this check reads length off the raw JSON rather than off the typed Go value it maps to.

**Why the map key set needs its own check independent of per-value completeness.** The identical argument, one level removed: D44's existing check walks *values already present* in the map and validates each one's shape, which says nothing about whether a key that should be present is missing. A `map_by_players` with three well-formed `MapSpec` values for `{2,3,4}` passes every check D44 specifies and is still missing the entire 5-player configuration.

## Consequences

- `internal/store`'s `Config` decode completeness check (D44's §2, step (a)) gains array-length and map-key-set assertions, driven by frozen per-version literal data declared alongside the frozen object key lists — one recursive, shape-aware function rather than two mechanisms.
- Each `Config` version's frozen-shape declaration now names, for every field, its kind (object / fixed array / int-keyed map) and the corresponding frozen fact (key list / length / key set), recursing into element or value types that are themselves objects. This is the shape every future version's declaration follows, including a future array- or map-typed field roadmap §5 has not added yet.
- Two more required fixtures join D44's two and D47's one already mandated for M3: a `Config` with a three-element `contracts` array, and one whose `map_by_players` omits `"5"`, both asserted to fail decode.
- No `Config` version bump, no migration, and no behavior change for any payload that was already complete under D44's stated (as opposed to literally implemented) intent.
- Reversible in the cheap direction only: tightening this check can only turn a previously-silent successful decode into an error, never the reverse, so nothing that decodes today under the corrected check could have been misread as something else yesterday. Loosening it back would be the expensive direction, and — per D44's own closing line — is itself a version-scoped decision, not a revert.
