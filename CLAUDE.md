# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository state

**No code exists yet.** This repo currently contains only design documentation:

```
docs/project/cinzal-gdd.md              — Game Design Document (v2.8)
docs/project/cinzal-architecture-rfc.md — Architecture RFC-001 (r11)
```

There is no `go.mod`, no source tree, no build/lint/test tooling. When implementation starts, build the `internal/`, `cmd/`, and `wasm/` layout specified in the RFC (§5) rather than inventing a different structure — the package boundaries described there (especially the `rules` / `render` / `web` import restrictions) are load-bearing for the game's core security property (see below), not arbitrary organization.

Treat the two docs as the spec. **The RFC is authoritative on architecture; the GDD is authoritative on rules.** Both are heavily changelogged at the top of each file — read the changelog before assuming a section is current, since later entries correct earlier ones (e.g. GDD mechanics have moved through v0.9 → v2.8, and several early designs — ghost paths, warehouse supply limits, seat-order tie-breaks — were deliberately cut). If GDD and RFC ever seem to disagree, the RFC's own changelog explains which GDD revision it's paired with ("Companion doc" header).

## What Cinzal is

A digital strategy game (2–5 players) with **simultaneous, secret orders** on a partially-hidden procedural graph map. Players run criminal factions smuggling "cargo" between warehouses and border checkpoints across 15 rounds (~30–35 min/match), inferring rivals' positions from public traces rather than direct observation. Full pitch and design pillars: GDD §1–2.

## The one constraint that shapes everything

**Fog is private.** The client must never receive the full match state — only what a given player's fog-of-war entitles them to see. This is a rule of the game (GDD §7.1), not a UX choice, and it is the reason for nearly every architectural decision in the RFC:

- All game state passes through a single projection function, `Project(s State, seat SeatID) PlayerView` (RFC §3, §9). No second path to the client is ever allowed — no debug JSON in production, no template that reaches around it.
- Package-level enforcement: `internal/render` and `internal/web` must never import anything that exposes `MatchState`; only the fog-filtered `PlayerView` type crosses that boundary. This is enforced by a `go list`-based CI check, not by convention (RFC §5, §9.1).
- Server-rendered HTML (HTMX), not a JSON API — a JSON endpoint is a second surface that must independently be kept fog-safe, and it's far easier to over-return fields in JSON than in hand-written HTML (RFC §3).
- `internal/rules` is a pure package — no I/O, no `time`, no `math/rand`, nothing network-touching, enforced by CI. It computes `Resolve()` (the whole round pipeline) and `Project()` (the fog boundary) as deterministic pure functions (RFC §6.1, §6.3).

When implementing anything in this codebase, ask first: does this leak state past the fog boundary? The RFC's fog test suite is explicitly *negative* — it asserts hidden facts are *absent*, not just unused (RFC §16.3) — and that's the standard to hold new code to.

## Planned architecture (once code exists)

- **Language/stack:** Go 1.23+, `templ` for typed templates, HTMX 2.x + SSE for interactivity, `sqlc` + `goose` + `pgx/v5` against Postgres 16. Zero hand-written JavaScript in v1. WASM (client-side rules) and rich map interaction are explicitly deferred to RFC-002 (RFC §4, §10).
- **State model:** event sourcing, no snapshots. `state = fold(Resolve, initial(seed, cfg), orderLog)`. The order log is the only source of truth; `events`/`match_summary` tables are derived/rebuildable caches, never authority (RFC §7.1–7.3).
- **Determinism:** `seed + order log` must reproduce a match exactly, forever. Four known hazards to avoid in Go specifically: ranging over maps in resolution, floating point, ambient `time.Now()`, and any concurrency inside `Resolve` (RFC §6.3). All RNG draws go through a single seeded `*RNG` with a documented consumption table (RFC §6.4) — conditional/early-terminated draws must stay lazy or replays desync.
- **Round tick:** advanced by either full submission or a deadline sweeper, both funneling through one `Tick()` guarded by `SELECT ... FOR UPDATE` — no broker, no queue (RFC §8). Bot orders are generated inside the tick, never submitted ahead of time.
- **Effects vs. state:** `Resolve` returns pure `[]Event`; only the tick's caller dispatches side effects (email, SSE, telemetry). This split exists specifically so refolding/replay/rebuild never re-sends historical notifications (RFC §7.4) — a real bug class the RFC calls out explicitly.
- **Bots:** one `Decide(PlayerView, Config, RNG) Order` interface, four roles (filler, autopilot, simulation, practice), three difficulty tiers. Bots seeing only `PlayerView` doubles as an executable test of the fog projection (RFC §14).
- **Debug tooling:** gated behind a `//go:build debug` build tag, never a runtime flag — the RFC treats an accidental runtime god-view in production as unrecoverable (RFC §15.1).

See RFC §21 for the intended build order (rules core → bots/simulation → persistence → round lifecycle → playable web → async/email → onboarding), chosen so the riskiest unknowns (is the game fun/balanced, is it deterministic) resolve before any UI is built. Milestone 2 (bot simulation) is called out as the one worth not skipping, since it's how open balance questions in the GDD (contested lease pricing, encounter rate, endgame camping) get answered by measurement instead of guesswork.

## Working with the GDD

The GDD is long and iteratively patched; when a rule seems odd, it's worth checking whether a later changelog entry explains the reasoning (most non-obvious rules — e.g. why posts only see their own node, why Evasive costs a shakedown, why contract destinations are "Rumoured" not "Known" — exist specifically to close a loophole or fix a deadlock found during design review, and the changelog entry says which one).

Key sections likely to matter most when implementing:
- §6–7 — map generation constraints and the fog/sight/trail model (the deduction game's core)
- §9 — the Order structure and the step-allowance formula (§9.1a) — order of operations matters and is spelled out explicitly
- §14–15 — event/incident decks, confrontation resolution, order legality
- §21 — the full randomness inventory (pairs with RFC §6.4's RNG consumption table — keep both in sync)
- §22 — telemetry/metrics the simulation harness (RFC §16.4) is built to answer
