# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Every shell command runs as `rtk <command>`, no exceptions** — including `git`, `gh`, and anything chained with `&&`. `rtk` passes through transparently when it has no dedicated filter, so there's never a reason to drop it.

> **Always write in English** — code, comments, commit messages, PR titles/descriptions, issue text, and any other artifact that lands in this repository — regardless of the language the request came in. The GDD, RFC, and every existing doc are English-only, and mixing languages in history or docs would break that consistency. Conversational replies to the user may still match the user's own language; this rule is about what gets committed or posted, not how you talk to them.

## Repository state

**M0, M1 and M2 are closed. M3 — Persistence is next.** `internal/game` and `internal/rules` are fully implemented: the whole game, deterministic and headless, with every RFC §16.1 layer that does not need a database (golden replays, RNG index accounting, fog negatives, anchor parity, cross-round state, property/adversarial tests, and, since M2, the bot-determinism row — a bot-populated match still replays byte-identically and its per-round index count doesn't move, including the Autopilot handover case). `internal/bots` (six production files — `bot.go`, `legalspace.go`, `drifter.go`, `runner.go`, `operator.go`, `doc.go` — the `Bot` interface, the tier registry, the legal-order enumerator, and the three tiers themselves) and `cmd/simulate` (the headless sweep-and-CSV harness) are both fully implemented per M2's deliverables. `internal/telemetry` ([D34](docs/decisions/D34-telemetry-package-placement.md)) computes all 17 of GDD §22's headless per-match rows — sixteen with a numeric result, and row 1 with none, since [D43](docs/decisions/D43-row-1-unmeasurable-post-d39.md) found D39's own remedy converted away the quantity row 1 counts. M2's seven exit-criteria rows (roadmap §4) are answered: three met, three deferred to M5.5 ([#241](https://github.com/garnizeh/cinzal/issues/241)), one closed with no action. Every other `internal/` package (`match`, `render`, `web`, `store`, `auth`, `mail`, `debug`) is still `doc.go` only, and `cmd/replay`/`cmd/server` are each a `doc.go` plus an empty `main`.

```text
docs/project/cinzal-gdd.md                  — Game Design Document (v2.31)
docs/project/cinzal-architecture-rfc.md     — Architecture RFC-001 (r37)
docs/project/cinzal-implementation-plan.md  — Roadmap: milestones, exit criteria, open decisions
docs/decisions/                             — Decision log; D1–D14, D23–D31 (M1) and D32–D43 (M2) decided (D15 reclassified as a task, #40), D16–D22 open (block M4/M5/M6)
```

`make check` runs `packages purity purity-selftest fog debug-isolation secrets bots-isolation bots-isolation-selftest simulate-deps lint test bench-regression-selftest prod dev`, all live and required on `main`. Three of those are M2's own additions. `bots-isolation` is the load-bearing one ([#195](https://github.com/garnizeh/cinzal/issues/195), RFC §14.5): `internal/bots` may not name `MatchState`, the graph, or the match seed. It can't be an import-graph check the way `fog` is — `bots` legitimately imports `rules` for `BotRNG` — so it walks every `rules.X` selector `internal/bots` uses against an allow-list (`scripts/bots-isolation-allowlist.txt`) that must be widened on purpose, in a reviewed PR, before a new symbol can be named at all; `bots-isolation-selftest` is that gate's own fixture coverage, and `simulate-deps` asserts `cmd/simulate` depends on only `rules`/`bots`/`game`/`telemetry`. `generate-check` is not part of `check` yet: `GENERATED` is still empty until M3/M5 land real generated paths, and a gate that can only report VACUOUS stays out of the aggregate rather than passing by not running — run `make generate-check` standalone if you want to see that message. `bench-compare` is also a required check, separate from `make check` — it only runs on a pull request that touches `internal/rules/gen`, `.github/workflows/ci.yml`, `scripts/check-bench-regression.sh`, or the `Makefile`; see CONTRIBUTING.md's "The benchmark regression gate." Since [#336](https://github.com/garnizeh/cinzal/issues/336), CI's `check` job runs `make check-nosecrets` — everything above except `secrets` — and skips itself (along with `replay`'s two-OS matrix) on a pull request that cannot touch Go source or its own tooling; `secrets` runs unconditionally in its own job instead, because it scans docs too and must never be skippable by what a diff touches. Both jobs' path check (`.github/actions/changed-paths`) fails open: an unresolvable diff runs everything rather than skipping.

Package layout is fixed by [D01](docs/decisions/D01-package-layout.md):

- `internal/game` is a **leaf** — shared vocabulary (`PlayerView`, `Order`, `Config`, `Event`, IDs). Imports nothing; no `any`/`interface{}`/unconstrained type params.
- `internal/rules` owns match state; imports only stdlib + `internal/game`.
- **`internal/render` and `internal/web` must never import `internal/rules` directly** — everything arrives via `internal/match` as `game` types.
- There is no `game.State` and there must never be one.

**The RFC is authoritative on architecture; the GDD is authoritative on rules.** Both are changelogged at the top — check it before trusting a section, since later entries correct earlier ones. If GDD and RFC disagree, the RFC's "Companion doc" header says which GDD revision it's paired with.

## What Cinzal is

A digital strategy game (2–5 players) with **simultaneous, secret orders** on a partially-hidden procedural graph map. Players run criminal factions smuggling "cargo" between warehouses and border checkpoints across 15 rounds (~30–35 min/match), inferring rivals' positions from public traces rather than direct observation. Full pitch and design pillars: GDD §1–2.

## The one constraint that shapes everything

**Fog is private.** The client must never receive the full match state — only what a player's fog-of-war entitles them to see (GDD §7.1, a game rule, not a UX choice). It drives most architectural decisions in the RFC:

- All game state passes through one projection function, `Project(s State, seat SeatID) PlayerView` (RFC §3, §9) — no debug JSON, no template that reaches around it.
- Enforced at package level by CI (`go list`-based, RFC §5, §9.1): `internal/render`/`internal/web` must never import anything that exposes `MatchState`.
- Server-rendered HTML (HTMX), not a JSON API — a JSON endpoint is a second surface that must independently stay fog-safe (RFC §3).
- `internal/rules` is pure — no I/O, `time`, `math/rand`, or network — enforced by CI. `Resolve()` and `Project()` are deterministic pure functions (RFC §6.1, §6.3).

Before implementing anything: does this leak state past the fog boundary? The RFC's fog suite asserts hidden facts are *absent*, not just unused (RFC §16.3) — hold new code to that standard.

## Planned architecture (specified, not yet implemented)

- **Stack:** Go 1.27.0, `templ`, HTMX 2.x + SSE, `sqlc` + `goose` + `pgx/v5` on Postgres 16. Zero hand-written JS in v1; WASM and rich map interaction deferred to RFC-002 (RFC §4, §10).
- **State model:** event sourcing, no snapshots — `state = fold(Resolve, initial(seed, cfg), orderLog)`. The order log is the only source of truth; `events`/`match_summary` are rebuildable caches, never authority (RFC §7.1–7.3).
- **Determinism:** `seed + order log` must reproduce a match forever. Avoid: map-range order, floats, `time.Now()`, concurrency inside `Resolve` (RFC §6.3). All RNG draws go through one seeded `*RNG` with a consumption table (RFC §6.4) — conditional/early-terminated draws must stay lazy or replays desync.
- **Round tick:** one `Tick()` guarded by `SELECT ... FOR UPDATE`, triggered by full submission or a deadline sweeper — no broker, no queue (RFC §8). Bot orders generate inside the tick, never ahead of it.
- **Effects vs. state:** `Resolve` returns pure `[]Event`; only the tick's caller dispatches side effects. This is what keeps refold/replay/rebuild from re-sending historical notifications (RFC §7.4).
- **Bots:** one `Decide(PlayerView, Config, RNG) Order` interface, four roles, three difficulty tiers. Seeing only `PlayerView` doubles as an executable fog-projection test (RFC §14).
- **Debug tooling:** gated behind `//go:build debug`, never a runtime flag — an accidental runtime god-view in production is unrecoverable (RFC §15.1).

Build order (RFC §21): rules core → bots/simulation → persistence → round lifecycle → playable web → async/email → onboarding — riskiest unknowns (fun/balance, determinism) resolve before any UI. Milestone 2 (bot simulation) answers open GDD balance questions by measurement, not guesswork.

## Working with the GDD

When a rule seems odd, check the changelog — most non-obvious rules exist to close a loophole or deadlock found during design review, and the changelog says which one.

Sections that matter most when implementing: §6–7 (map generation, fog/sight/trail), §9 (Order structure, step-allowance formula in §9.1a), §14–15 (event/incident decks, confrontation, legality), §21 (randomness inventory — keep in sync with RFC §6.4), §22 (telemetry the simulation harness answers, RFC §16.4).

## How work lands here

Full detail in [CONTRIBUTING.md](CONTRIBUTING.md). What an agent gets wrong without being told:

- **Everything goes through a PR.** `main` is protected even for the maintainer, squash-only, linear history, every review conversation must resolve before merge. One task = one PR = one commit.
- **The PR description becomes the commit message** — write it for whoever reads `git log` in a year.
- **Work is tracked as three things:** *decisions* (`docs/decisions/`, block dependent tasks), *tasks* (produce code), *exit demonstrations* (prove a milestone met its criteria, often by breaking something on purpose). A task that can't cite a GDD/RFC section is really a decision — file it as one.

### Verifying that CodeRabbit actually reviewed

CodeRabbit runs on the free OSS tier and often skips a PR with "Review limit reached" — **and its status check reports success anyway.** This has been the common case here.

**The only reliable signal is negative: a finding still raised against the current head means not addressed.** Everything else — a green check, no new review on your latest commit, a missing `✅ Addressed` marker — is inconclusive; a clean incremental review posts nothing, and the marker isn't guaranteed even on a real fix.

**`pulls/<n>/comments` is not the complete set of findings — check the review body too.** A finding CodeRabbit can't anchor to a diff position (typically a line untouched by the specific commit range an incremental review diffed against) never becomes a comment at all; it lands as an "Outside diff range comments" block inside the *review object's own `body` field* instead, with no comment ID and nothing in `pulls/<n>/comments`:

```
gh api repos/<owner>/<repo>/pulls/<n>/reviews/<review_id> --jq .body
```

Read this for every review whose `body` isn't empty, not just its positioned comments — a comments-only audit reads as complete and silently isn't. Found 2026-08-20 on PR #228: a full `pulls/<n>/comments` audit reported zero unaddressed findings while a real one sat unposted in the review body, and the user had to paste it by hand.

Procedure: after pushing a fix, check whether the finding is still raised against the head. If not, and the fix is right, resolve the thread and record *why* in the reply. If quota was available, the review fires on its own — don't trigger manually. On "Review limit reached," wait 20–45 min then `@coderabbitai review`; if it answers "Already reviewed," that's an answer, not a refusal — look for `✅ Addressed` markers rather than retrying. If a merge genuinely can't wait for review, say so in the PR description. When matching the marker in tooling, note the wording varies with commit count — match `Addressed in commits? …`, not just the singular form.

If you disagree with a finding, reply with reasoning — CodeRabbit answers and concedes when it's wrong (it did on a suggestion that would have introduced `game.State`, inverting D01). If a reply 404s, the thread went outdated after your push; post a PR-level comment instead.

**Verify findings before applying them** — usually right (caught real defects here), but check each against the specs; when a finding is right about the problem and wrong about the fix, say so instead of adopting it. **Findings can point past their own file** — twice here the real bug was a spec section or unrelated issue carrying the same wrong statement; when a finding exposes a wrong statement, grep for it elsewhere.

### Gates fail closed

Every check here reports **failure** when it can't run — missing tool, empty `go list` output, unreadable config. A gate built the obvious way reports green having inspected zero packages, which is the same failure as a review bot reporting success on a skipped review. **A gate that passes when it can't run is worse than no gate.** Hold new checks to this, and never "fix" a noisy gate by letting it skip.
