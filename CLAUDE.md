# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Every shell command runs as `rtk <command>`, no exceptions** — including `git`, `gh`, and anything chained with `&&`. `rtk` passes through transparently when it has no dedicated filter, so there's never a reason to drop it.

> **Always write in English** — code, comments, commit messages, PR titles/descriptions, issue text, docs, and replies to the user. The GDD, RFC, and every existing doc are English-only, and mixing languages in history or docs would break that consistency. The one exception is the trigger phrases in `.claude/skills/*/SKILL.md` frontmatter, which are deliberately bilingual because they match what the maintainer types.

> **Process lives in the harness, not in this file.** [`.claude/WORKFLOW.md`](.claude/WORKFLOW.md) is the stage-by-stage contract — what each stage receives, produces, and must satisfy before the next starts — and [`.claude/skills/README.md`](.claude/skills/README.md) is the lookup table for the fifteen skills under [`.claude/skills/`](.claude/skills/). **Start there rather than improvising a process.** This file is the always-loaded context those skills assume: what the repo is, where it stands, and the constraints that hold in every stage.

## Repository state

**M0, M1 and M2 are closed. M3 — Persistence is underway.** `internal/game` and `internal/rules` are fully implemented: the whole game, deterministic and headless, with every RFC §16.1 layer that does not need a database (golden replays, RNG index accounting, fog negatives, anchor parity, cross-round state, property/adversarial tests, and, since M2, the bot-determinism row — a bot-populated match still replays byte-identically and its per-round index count doesn't move, including the Autopilot handover case). `internal/bots` (six production files — `bot.go`, `legalspace.go`, `drifter.go`, `runner.go`, `operator.go`, `doc.go` — the `Bot` interface, the tier registry, the legal-order enumerator, and the three tiers themselves) and `cmd/simulate` (the headless sweep-and-CSV harness) are both fully implemented per M2's deliverables. `internal/telemetry` ([D34](docs/decisions/D34-telemetry-package-placement.md)) computes all 17 of GDD §22's headless per-match rows — sixteen with a numeric result, and row 1 with none, since [D43](docs/decisions/D43-row-1-unmeasurable-post-d39.md) found D39's own remedy converted away the quantity row 1 counts. M2's seven exit-criteria rows (roadmap §4) are answered: three met, three deferred to M5.5 ([#241](https://github.com/garnizeh/cinzal/issues/241)), one closed with no action. Every other `internal/` package (`match`, `render`, `web`, `auth`, `mail`, `debug`) is still `doc.go` only; `internal/store` has two files — `pool.go`, the pgx/v5 connection pool and its lifecycle ([#310](https://github.com/garnizeh/cinzal/issues/310)), and `migrate.go`, the embedded `goose` migrations run at boot behind a session-scoped advisory lock ([#311](https://github.com/garnizeh/cinzal/issues/311)) — and `cmd/replay`/`cmd/server` are each a `doc.go` plus an empty `main`.

```text
docs/project/cinzal-gdd.md                  — Game Design Document (v2.32)
docs/project/cinzal-architecture-rfc.md     — Architecture RFC-001 (r51)
docs/project/cinzal-implementation-plan.md  — Roadmap: milestones, exit criteria, open decisions
docs/decisions/                             — Decision log; D1–D14, D16–D20 and D23–D56 decided
                                              (D15 reclassified as a task, #40); D21–D22 open, block M5/M6
```

M3's tracking issue is [#332](https://github.com/garnizeh/cinzal/issues/332). Filing an issue or merging a PR updates it in the same turn.

`make check` runs `packages purity purity-selftest fog debug-isolation secrets vuln bots-isolation bots-isolation-selftest simulate-deps generate-check generate-check-selftest lint test bench-regression-selftest prod dev`, all live. `generate-check` joined that list in issue #316, once `GENERATED` gained sqlc's real output (issue #315) to compare against — its still-empty templ half (`GENERATED_TEMPL`) stays that way until M5 fills it in, documented at its own Makefile definition. One more is true of the gate set but not visible in that list: `bench-compare` runs only on a PR that touches `internal/rules/gen`, `.github/workflows/ci.yml`, `.github/actions/changed-paths/action.yml`, `scripts/check-bench-regression.sh` or the `Makefile`. What each gate asserts and what a failure means is in the [`gates-run`](.claude/skills/gates-run/SKILL.md) skill; the reasoning behind each is in [CONTRIBUTING.md](CONTRIBUTING.md).

**"Runs in CI" and "blocks a merge" are two different facts, and the second one is checkable rather than assumed.** Branch protection requires six status checks — `check`, `secrets`, `bench-compare`, `vuln`, `replay (ubuntu-latest)` and `replay (macos-latest)` — and there are no repository rulesets, so that is the whole enforcement surface. `vuln` and the two `replay` legs were promoted in issue #391, having run on every PR while blocking nothing: a red `vuln` (a newly-disclosed CVE against a dependency already in `go.sum`) used to merge. **`bench` is deliberately not required** — it carries `if: github.event_name == 'push'` and reports `skipped` on every PR, so requiring it would deadlock all of them. Read the list rather than this sentence when it matters: `rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required_status_checks.contexts`.

Package layout is fixed by [D01](docs/decisions/D01-package-layout.md):

- `internal/game` is a **leaf** — shared vocabulary (`PlayerView`, `Order`, `Config`, `Event`, IDs). Imports nothing; no `any`/`interface{}`/unconstrained type params.
- `internal/rules` owns match state; imports only stdlib + `internal/game`.
- **`internal/render` and `internal/web` must never import `internal/rules` directly** — everything arrives via `internal/match` as `game` types.
- There is no `game.State` and there must never be one.

**The RFC is authoritative on architecture; the GDD is authoritative on rules.** Both are changelogged at the top — check it before trusting a section, since later entries correct earlier ones, and most non-obvious rules exist to close a loophole found in design review. If GDD and RFC disagree, the RFC's "Companion doc" header says which GDD revision it's paired with.

## What Cinzal is

A digital strategy game (2–5 players) with **simultaneous, secret orders** on a partially-hidden procedural graph map. Players run criminal factions smuggling "cargo" between warehouses and border checkpoints across 15 rounds (~30–35 min/match), inferring rivals' positions from public traces rather than direct observation. Full pitch and design pillars: GDD §1–2.

GDD sections that matter most when implementing: §6–7 (map generation, fog/sight/trail), §9 (Order structure, step-allowance formula in §9.1a), §14–15 (event/incident decks, confrontation, legality), §21 (randomness inventory — keep in sync with RFC §6.4), §22 (telemetry the simulation harness answers, RFC §16.4).

## The one constraint that shapes everything

**Fog is private.** The client must never receive the full match state — only what a player's fog-of-war entitles them to see (GDD §7.1, a game rule, not a UX choice). It drives most architectural decisions in the RFC:

- All game state passes through one projection function, `Project(s State, seat SeatID) PlayerView` (RFC §3, §9) — no debug JSON, no template that reaches around it.
- Enforced at package level by CI (`go list`-based, RFC §5, §9.1): `internal/render`/`internal/web` must never import anything that exposes `MatchState`.
- Server-rendered HTML (HTMX), not a JSON API — a JSON endpoint is a second surface that must independently stay fog-safe (RFC §3).
- `internal/rules` is pure — no I/O, `time`, `math/rand`, or network — enforced by CI. `Resolve()` and `Project()` are deterministic pure functions (RFC §6.1, §6.3).

Before implementing anything: does this leak state past the fog boundary? The RFC's fog suite asserts hidden facts are *absent*, not just unused (RFC §16.3) — hold new code to that standard.

## Planned architecture (specified, not yet implemented)

- **Stack:** Go 1.27.0, `templ`, HTMX 2.x + SSE, `sqlc` + `goose` + `pgx/v5` on Postgres 18.6. Zero hand-written JS in v1; WASM and rich map interaction deferred to RFC-002 (RFC §4, §10).
- **State model:** event sourcing, no snapshots — `state = fold(Resolve, initial(seed, cfg), orderLog)`. The order log is the only source of truth; `events`/`match_summary` are rebuildable caches, never authority (RFC §7.1–7.3).
- **Determinism:** `seed + order log` must reproduce a match forever. Avoid: map-range order, floats, `time.Now()`, concurrency inside `Resolve` (RFC §6.3). All RNG draws go through one seeded `*RNG` with a consumption table (RFC §6.4) — conditional/early-terminated draws must stay lazy or replays desync.
- **Round tick:** one `Tick()` guarded by `SELECT ... FOR UPDATE`, triggered by full submission or a deadline sweeper — no broker, no queue (RFC §8). Bot orders generate inside the tick, never ahead of it.
- **Effects vs. state:** `Resolve` returns pure `[]Event`; only the tick's caller dispatches side effects. This is what keeps refold/replay/rebuild from re-sending historical notifications (RFC §7.4).
- **Bots:** one `Decide(PlayerView, Config, RNG) Order` interface, four roles, three difficulty tiers. Seeing only `PlayerView` doubles as an executable fog-projection test (RFC §14).
- **Debug tooling:** gated behind `//go:build debug`, never a runtime flag — an accidental runtime god-view in production is unrecoverable (RFC §15.1).

Build order (RFC §21): rules core → bots/simulation → persistence → round lifecycle → playable web → async/email → onboarding — riskiest unknowns (fun/balance, determinism) resolve before any UI. Milestone 2 (bot simulation) answers open GDD balance questions by measurement, not guesswork.

## How work lands here

Full detail in [CONTRIBUTING.md](CONTRIBUTING.md); the procedure is in the harness. Three framing facts an agent gets wrong without being told, because they decide which skill applies before any of them is loaded:

- **Everything goes through a PR.** `main` is protected even for the maintainer, squash-only, linear history, every review conversation must resolve before merge. One task = one PR = one commit.
- **The PR description becomes the commit message** — write it for whoever reads `git log` in a year.
- **Work is tracked as three things:** *decisions* (`docs/decisions/`, block dependent tasks), *tasks* (produce code or docs), *exit demonstrations* (prove a milestone met its criteria, often by breaking something on purpose). A task that can't cite a GDD/RFC section is really a decision — file it as one.

## Absence of a signal is not evidence of a state

The one idea behind more of this repository's tooling than any other. It took four misreadings here to arrive at, and it has two standing consequences:

**Gates fail closed.** Every check reports **failure** when it can't run — missing tool, empty `go list` output, unreadable config. A gate built the obvious way reports green having inspected zero packages. **A gate that passes when it can't run is worse than no gate.** Hold new checks to this, and never "fix" a noisy gate by letting it skip.

**A green CodeRabbit check means nothing.** It runs on the free OSS tier and often skips a PR with "Review limit reached" while its status check still reports success — the common case here. There are exactly two reliable signals, and neither is the status check. The negative one: *a finding still raised against the current head means not addressed.* The positive one, which is the merge gate: CodeRabbit's main PR comment reads exactly "No actionable comments were generated in the recent review. 🎉", bound to the current head SHA. Do not merge on a green check or on silence — but once that exact text lands against the head, the merge runs the same turn without a separate go-ahead, per the maintainer's standing 2026-08-27 authorization in [`.claude/WORKFLOW.md`](.claude/WORKFLOW.md). Note also that `pulls/<n>/comments` is not the complete set of findings — a finding that can't be anchored to a diff position lands only in the review object's own `body`. The full procedure, including the `gh api` calls, is in the [`coderabbit-triage`](.claude/skills/coderabbit-triage/SKILL.md) skill.
