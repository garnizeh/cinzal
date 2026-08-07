# D02 — Where does the order draft live between clicks?

**Status:** decided
**Blocks:** M0 — Foundations (indirectly), M5 — Playable web (substantively)
**Decided:** 2026-08-06
**Issue:** [#4](https://github.com/garnizeh/cinzal/issues/4)

## The question

A player builds a route by clicking nodes on the map, one request per click. Where does that half-built order live between clicks?

## Why it is open

**RFC-001 §11** defines the endpoint and stops there:

```
POST /m/{id}/order/node/{node}   append/remove a node from the route draft
```

**§11.2** describes the interaction — "each click is a ~100ms HTMX swap returning the SVG with the path drawn and the step budget updated" — which requires the server to know the draft so far, and still never says where it comes from.

The gap matters beyond tidiness. The draft is per-seat state on the request path, and §11.1a already establishes that a submitted order carries a `round` field so a stale submission can be rejected rather than silently applied. Whatever holds the draft has to answer that same staleness question.

## Options

**A — Stateless: the draft round-trips in the form.** Re-posted as hidden inputs on every click, re-validated server-side each time.

- For: no table, no TTL, no cleanup job, no instance affinity, no staleness window.
- Against: larger POST bodies; no resuming a half-built order on another device.

**B — Server-side draft, in a table or a session store.**

- For: smaller payloads; the server is the single holder; cross-device resume.
- Against: a table, a TTL policy, a cleanup job, an instance-affinity question against the two-instance deployment of §18, and a second piece of per-seat state that can disagree with the submitted order.

## Decision

**Option A — stateless.**

The draft is a `game.OrderDraft`, carried in the form as hidden inputs. Every click posts the full draft plus the node that was clicked. The handler applies the mutation, validates the result with `rules.Legal` through `internal/match`, and re-renders the board fragment with the new draft embedded.

Three properties are fixed by this decision:

1. **The draft carries `round`**, exactly as the submission does (§11.1a). A draft built against round 5 that arrives when the match is on round 6 is rejected with *"this round has closed"* and the board is re-rendered at the current round.
2. **Nothing is stored.** There is no draft table, no session entry, and nothing to expire. A player who closes the tab loses an unsubmitted draft, which is correct: they had not submitted.
3. **The draft is a partial `Order`.** Submitting is the same payload with the submit button pressed, so there is exactly one shape to validate and one to test.

## Reasoning

**This is the same argument RFC §7.3 already made, at a smaller scale.** Snapshots were declined there not on performance grounds but because "a snapshot is a second source of truth for state whose only source of truth is supposed to be the log." A server-side draft is that shape of thing again: a second holder of per-seat intent that can disagree with the order actually submitted. The RFC also allows request-scoped memoisation in the same section, on the grounds that it "has no staleness window at all" — and the stateless draft has none either, for the same reason.

**What option B buys is a feature nobody asked for.** Cross-device resume of a half-built route, inside a 60–90 second order window, is not a use case this game has. The cost is a table, a TTL policy, a cleanup job, and an affinity question — real machinery, permanently, in exchange for that.

**It preserves two properties the RFC values explicitly.** §11.1 wants the async flow testable with `curl`; §11.2 wants the whole game to degrade to plain forms so "a failed asset load does not brick a match." A draft held in the form satisfies both without effort. A draft held server-side satisfies them too, but only as long as nobody adds a session assumption to the handler.

**On the fog question, since it is the one this project asks first: there is no leak.** The draft contains node IDs the player selected from nodes already in their own `PlayerView`. Sending it back to them discloses nothing they did not author. It is the one category of state that is safe to hand the client, because the client is where it came from.

**On payload size.** A route is at most four node IDs, plus a stance, an action, up to three items and two add-ons. This is tens of bytes. The instinct that round-tripping state is expensive is calibrated for a different scale of application; RFC §1 warns about exactly that ("nearly every performance instinct you might have is wrong at this scale").

## Consequences

- `internal/match` must expose the draft operations — `DraftNode`, `DraftAction`, and so on — returning a validated `game.OrderDraft` and a `game.PlayerView`. This is what lets `web` stay clear of `internal/rules`, which is the one real cost of [D01](D01-package-layout.md)'s decision. The two were taken together.
- The affordance rules of RFC §10.2 are rendered from the draft on each click: the action selector arrives already disabled with its reason when Pushing On is declared, node targets arrive unclickable once the route enters a Hidden node.
- No schema change. Nothing to add to RFC §7.2.
- Reversible at moderate cost. Moving to a server-side draft later means adding a table and changing the handlers, but not the validation, the templates, or the submitted payload.
