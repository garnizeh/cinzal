# D47 — What does an enum's reserved-invalid zero value encode to under D44?

**Status:** decided
**Blocks:** the `game`-side half of D44 (#307's consequences), the order-log append (#317), match creation and reload (#318), and every stored golden fixture (#328, #330)
**Decided:** 2026-08-26
**Issue:** [#344](https://github.com/garnizeh/cinzal/issues/344)

## The question

[D44](D44-config-order-jsonb-encoding.md) requires `MarshalJSON`/`UnmarshalJSON` on every `iota`-based enum reachable from `Order`, backed by a frozen wire-name table, decoded strictly: *"an unrecognized string is a decode error, never a silent fall-through to the zero/invalid sentinel these types already reserve by convention."* It never says what the **zero value itself** encodes to — and `enums.go`'s own stated rule (*"every enum in this file reserves its zero value as invalid"*) makes that the common case, not an edge case, for `Order`:

- `ActionOrder.Item` — meaningful only when `Kind == ActionDeal`; every Pickup, Deliver, Stake Post, Vanish, Surveil, and Nothing order carries `Item == 0`.
- `AddOns.OpenDoorsItem` — zero means "not declared" (the type's own doc comment names this convention explicitly).
- `StanceOrder.Stance` and `ActionOrder.Kind` — zero is unreachable from a *well-formed* order, but the submit handler persists `orders.payload` from what the client posts (RFC §8's `INSERT INTO orders … payload = …`, inside the submit transaction), while GDD §15.0 legality checking and Step 0 degradation run later, inside `Resolve`, over rows already on disk. A hand-crafted or genuinely incomplete payload — the exact case RFC §16.3's adversarial suite is built to submit — reaches the column with these fields unset before any degradation touches it.

`String()`, the table D44 seeds from, returns `"ItemID(0)"` for that value — a string the strict decoder D44 mandates is required to reject. Left unresolved, the encoding produced for the *majority* of real orders is one D44's own decoder must refuse, and the round-trip property D44 makes a required M3 test (`decode(encode(x)) == x` for arbitrary `x`) fails on the ordinary case.

## Why it is open

**Both options named in the issue contradict something D44 already decided.** Giving zero a wire literal (`"none"`, or similar) adds a table entry `UnmarshalJSON` must accept for a value the type reserves as invalid — exactly the "fall-through to the sentinel" D44's strictness rule exists to forbid, unless the rule states an exception. Refusing to encode zero fails to marshal every non-`Deal`, non-declared-item, or GDD-§15.0-degraded order — most of them.

**D44's own field audit undercounts the affected surface.** It says *"`Order` carries four enum-typed fields (`ActionKind`, `Stance`, two `ItemID` sites, one `Sector` behind a pointer)"* — there are **three** `ItemID` sites, not two. `ActionOrder.Item` is missing from that list, and reading `order.go` field by field, it is the site whose zero value appears on the wire most often.

**`omitempty` is not the free answer it looks like, on the surface.** D44's own text (§Q2/Q3 reasoning) flags it without resolving it: dropping a key when its value is zero is consistent with the rule D44 already wrote for `Order` — *"a missing field's zero value must already be a legal historical meaning"* — but leaves two loose ends unaddressed: whether `DisallowUnknownFields` (D44's corruption guard for `Order`) has any bearing on a field's *absence*, and how "omit on zero" interacts with the `*Sector`/`*NodeID` pointer fields D44 already handles separately (`nil` → `null`).

**This is the same failure class D44 was written to close, one level down.** A wrong answer here does not throw: an order encoded before this is settled decodes into a different order afterwards, silently, and folds to a different match.

## Options

**Give zero a named wire literal** (e.g. `"none"`), added to each affected type's frozen table as an explicit entry for the reserved-invalid value. Requires `UnmarshalJSON` to treat that one literal as legal while still rejecting every other unrecognized string — workable, but it invents a name that has to be frozen forever alongside the real constants, for a value that was never meant to be nameable.

**Refuse to encode zero.** `MarshalJSON` returns an error whenever the receiver is the reserved-invalid value. Fails to marshal `ActionOrder.Item` on every order whose `Kind != ActionDeal`, `AddOns.OpenDoorsItem` on every order that doesn't declare Open Doors, and `ActionOrder.Kind`/`StanceOrder.Stance` on the GDD §15.0 illegal-payload rows the order log is required to retain. Not viable — this is most of the log.

**Omit the key when the value is zero** (`omitempty` on the field's JSON tag), with key-absence and an explicit JSON `null` both decoding to the zero value. No wire literal is invented; `UnmarshalJSON`'s strictness only ever governs a *present, non-null* string. Chosen.

## Decision

**The reserved-invalid zero of `ActionKind`, `Stance`, and `ItemID`, wherever they appear directly (not behind a pointer) in `Order`, is represented by the *absence* of the JSON key — never a named literal.** Concretely:

1. `ActionOrder.Kind`, `ActionOrder.Item`, `StanceOrder.Stance`, `AddOns.OpenDoorsItem`, and `ItemDiscard.Item` (the corrected, three-site `ItemID` list — see below) all gain `,omitempty` on their struct tag. `encoding/json`'s `omitempty` check inspects the field's underlying kind (`Uint()  == 0` for these `uintN`-backed named types) before it ever calls the field's own `MarshalJSON` — so at zero, the custom `MarshalJSON` these types carry per D44 is never invoked at all, and the key never appears on the wire. No branch for zero is needed inside any of these types' `MarshalJSON`; the frozen wire-name table continues to hold only the real, non-zero constants, exactly as D44 originally specified.

2. On decode, a key absent from the payload leaves the Go field at its zero value through ordinary `encoding/json` struct-fill — the field's `UnmarshalJSON` is never called, because there are no bytes for that key to call it with. This is not new machinery: it is the same rule D44 already wrote for `Order` (*"a missing field's zero value must already be a legal historical meaning"*), extended to cover the enum fields D44's own audit missed.

3. **The stated exception to D44's strict-decode rule:** `UnmarshalJSON` on `ActionKind`, `Stance`, and `ItemID` must additionally treat the literal JSON `null` — if the key is present with that value — identically to key-absence (decode to zero, no error), because `encoding/json` calls a value-receiver `UnmarshalJSON` with the raw bytes `null` when the key is present and null, unlike the pointer-field optimization D44's `*Sector`/`*NodeID` fields get for free. Strictness governs one case only: **a key present with a non-null string that matches no entry in the frozen table.** Absence and explicit `null` are both legal and both mean "unset"; neither is a fall-through, because neither reaches the table lookup at all.

4. **`DisallowUnknownFields` has no bearing on this.** It rejects a JSON key with no matching struct field; it says nothing about a struct field with no matching JSON key. `omitempty` on encode and ordinary zero-fill on decode need no interaction with it, and no extra code beyond the tag change and the `null`-handling clause above.

5. **`Sector` needs none of this.** The only `Order`-reachable `Sector` site is `PushingOn.Bias *Sector`, already `nil` → `null` per D44. `Bias == nil` already carries "no bias declared"; a non-nil `*Sector` can only ever hold one of the four legitimate wire strings, because `Sector`'s wire encoding is a string (D44 Q4), not a bare number — a payload attempting to smuggle a bare `0` in as a sector fails to decode as a type mismatch, the same as any other unrecognized shape, before a `Sector(0)` value could ever exist to be pointed at. The answer is therefore **not uniform** across the four types the issue names: three (`ActionKind`, `Stance`, `ItemID`) get the omit-on-zero treatment; `Sector` needed nothing new because D44's own pointer convention already closed the gap for it.

6. **Corrected `ItemID` site count, per issue item 3.** `Order` reaches `ItemID` at **three** sites, not the two D44's audit named: `ActionOrder.Item`, `AddOns.OpenDoorsItem`, and `ItemDiscard.Item` (the discard entry's own item — zero only in a malformed `Items` entry, since a well-formed discard always names a real item). All three follow the same rule above.

7. **This convention is `Order`-specific and does not generalize to `Config`.** D44 gives `Config` an explicit version and a recursive exact-key-set check specifically so a missing field is a **hard decode error**, never a default — the opposite of the "absence is legal" rule this decision relies on. If a future `Config` dial becomes an enum with a reserved-invalid zero, D47's answer does not apply to it; that is a fresh question for whichever decision adds that field, because `Config`'s missing-field policy is deliberately the inverse of `Order`'s.

**Required fixture (issue item 4):** an `Order` with `Action.Kind == ActionNothing`, `Action.Item` unset, `AddOns.OpenDoorsItem` unset, and `PushingOn.Bias == nil` — the *ordinary* case, not a corner one — round-tripped through `encode`/`decode` and asserted equal to the original. This joins the two fixtures D44 §3 already requires (a `Config` missing a nested field, rejected; an `Order` using a since-renamed-for-display enum label, still decoding against the frozen table).

## Reasoning

**Why omit-key over a named literal.** A named literal for "unset" (`"none"`, `""`, or similar) has to be frozen into the wire table forever, sitting alongside the real constants, for a value the type's own convention says is not a real value at all — it invites exactly the confusion D44's strictness rule was written to prevent: is `"none"` a *recognized* name that decodes to zero, or does recognizing it quietly reopen the "unrecognized name silently becomes zero" hole the rule closes? Key-absence sidesteps the question entirely: there is no string to look up, so there is nothing for a future reader to conflate with a real, table-backed name. The exception is structural (the table is never consulted) rather than a carve-out inside the table (an entry that behaves differently from every other entry).

**Why `omitempty` doesn't need a version, corruption guard, or new decode machinery for `Order`.** D44 already decided `Order` is unversioned and append-only, with the missing-field convention justified by RFC §7.1's cross-version reread argument — that decision did the hard work; this decision only points out that the `Order` audit stopped one field short of enumerating everywhere it applies. `DisallowUnknownFields` (D44's actual corruption guard for `Order`) is orthogonal to field absence by construction — it is checked once, in ordinary Go `encoding/json`, against keys with no destination, never against destinations with no key.

**Why `Sector` falls out for free.** D44 made a conscious per-type choice at Q4: structural pointer fields get `nil → null`, `iota`-block enums get a frozen string table. Those are two different answers to "how does this field represent unset" chosen for two different Go representations, and `PushingOn.Bias` already has the pointer answer. Extending `omitempty` to a pointer field would contradict D44's `nil → null` choice for no benefit — `Sector`'s reserved-invalid zero was never reachable through `Bias` in the first place, once the string-not-number wire shape is taken into account.

**Why this doesn't extend to `Config`.** The two columns have opposite missing-field philosophies by design (D44's central asymmetry): `Order`'s "absence is a legal historical meaning" versus `Config`'s "absence is proof the payload doesn't match its declared version, reject." Both are correct for the risk shape D44 already argued for each column; this decision does not reopen that argument, it only completes the half of it (`Order`'s enum fields) D44's audit missed.

## Consequences

- `internal/game/order.go` gains `,omitempty` on the JSON tags of `ActionOrder.Kind`, `ActionOrder.Item`, `StanceOrder.Stance`, `AddOns.OpenDoorsItem`, and `ItemDiscard.Item`. No other file changes: the `MarshalJSON`/`UnmarshalJSON` pairs D44 already specifies for `ActionKind`, `Stance`, and `ItemID` need one added clause — treat a present literal `null` as key-absence — and no branch for the zero case, since `omitempty` keeps `MarshalJSON` from ever being called with it.
- D44's own field audit is corrected in place here rather than edited there (per the "record what happened," not "silently fix the prior document" convention): the real `ItemID` site count reachable from `Order` is three, not two.
- The round-trip property D44 makes a required M3 test now has a defined answer for the ordinary case (an order with unset optional enums), not just the corner cases D44's original two fixtures cover; the fixture in the Decision section above is required alongside them.
- Reversible in the cheap direction (adding the "unset" case was always going to be additive), but not in the expensive one: once a real `orders.payload` row has been written with these keys omitted, introducing a named literal for the same meaning later is itself a breaking change to what "the key is absent" is allowed to mean — the same one-way door D44 already names for its own encoding choices.
