# Cinzal

> *Everyone knows what you did. Nobody knows where you are.*

A digital strategy game for 2–5 players, played in rounds of **simultaneous, secret orders** on a partially hidden procedural graph map.

Each player runs a criminal faction in Cinzal — a metropolis in decay — competing for smuggling contracts: take cargo at a warehouse, cross the city without being intercepted, deliver at a border checkpoint. You never see where your rivals are. You see what they *do*: cargo vanishing from a warehouse, a new post staked at a corner, a delivery called out on the police band. From those traces you deduce routes, set ambushes, and lie about your own.

A match runs 15 rounds — roughly 30–35 minutes — and **the number of players does not change the length**, because everyone plays at once. That single property is what makes asynchronous play a timer setting rather than a separate mode.

## Status

**Design and planning complete. No code yet.** This repository currently holds three documents:

| Document | What it is | Authority |
|---|---|---|
| [`docs/project/cinzal-gdd.md`](docs/project/cinzal-gdd.md) | Game Design Document, v2.16 | Authoritative on **rules** |
| [`docs/project/cinzal-architecture-rfc.md`](docs/project/cinzal-architecture-rfc.md) | Architecture RFC-001, r16 | Authoritative on **architecture** |
| [`docs/project/cinzal-implementation-plan.md`](docs/project/cinzal-implementation-plan.md) | Implementation roadmap, p2 | Sequencing, exit criteria, open decisions |

All three are heavily changelogged. Later entries correct earlier ones — read the changelog before assuming a section is current.

## The one constraint that shapes everything

**Fog is private.** The client must never hold the full match state, only what a given player's fog-of-war entitles them to see. This is a rule of the game, not a UX preference, and it drives nearly every architectural decision:

- All state reaching a player passes through exactly one function, `Project(s State, seat SeatID) PlayerView`. There is no second path.
- The rendering packages **cannot name** the full match state — it is not in scope there. Enforced by the package graph and a CI check, not by discipline.
- Server-rendered HTML rather than a JSON API: a JSON endpoint is a second surface that must independently be kept fog-safe, and it is far easier to over-return a field in JSON than in hand-written HTML.
- The rules package is pure — no I/O, no clock, no ambient randomness — so `seed + order log` reproduces any match exactly, forever.

The fog test suite is deliberately **negative**: it asserts hidden facts are *absent*, not merely unused.

## Planned stack

Go 1.26.5 · [templ](https://templ.guide) · HTMX 2.x + SSE · [sqlc](https://sqlc.dev) · [goose](https://github.com/pressly/goose) · pgx/v5 · Postgres 16

Zero hand-written JavaScript in v1. Event-sourced state with no snapshots — the order log is the only source of truth. One static binary plus a database URL is the entire production topology.

Client-side rules via WASM and rich map interaction are deliberately deferred to RFC-002.

## Build order

Chosen so the riskiest unknowns — *is it deterministic, is it balanced* — resolve before any UI exists.

```text
M0  Foundations       CI gates for fog boundary and rules purity
M1  Rules core        the whole game, headless and deterministic
M2  Bots + simulation the measurement gate — answers the open balance questions
M3  Persistence       matches survive restarts and reproduce exactly
M4  Round lifecycle   a full match runs end to end with no browser
M5  Playable web      ← this is what ships
M6  Async             the mode that makes the product distinct
M7  Onboarding        solo scenario ladder
M8  Launch hardening
```

See the [implementation roadmap](docs/project/cinzal-implementation-plan.md) for deliverables, exit criteria, and the open decisions that block each milestone.

## Contributing

Start with [CONTRIBUTING.md](CONTRIBUTING.md) — it covers how work is organised, what every issue must carry, and the four CI gates that are not style checks.

Changes land on `main` through pull requests. `main` is protected against direct pushes, and open decisions are tracked in [`docs/decisions/`](docs/decisions/).

## Licence

[MIT](LICENSE)
