# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **MANDATORY: every shell command in this repository is run as `rtk <command>`, no exceptions.** This includes `git`, `gh`, `grep`, `find`, `wc`, `ls`, `cat`-equivalents, and any command chained with `&&`. `rtk` is a transparent passthrough for anything it has no dedicated filter for, so there is never a reason to drop it — see the RTK section below for the full command reference.

## Repository state

**M0 is closed. M1 — Rules core is open, and nothing is implemented yet.** Every `internal/` package is a `doc.go` and no more.

```text
docs/project/cinzal-gdd.md                  — Game Design Document (v2.16)
docs/project/cinzal-architecture-rfc.md     — Architecture RFC-001 (r17)
docs/project/cinzal-implementation-plan.md  — Roadmap: milestones, exit criteria, open decisions
docs/decisions/                             — Decision log; D1, D2, D3, D4, D5, D6, D7, D8, D9, D10 and D11 are decided, D12–D14 and D16–D22 are open
```

D15 is **not** a decision — it is two factual errors in the source documents, tracked as a task. The log's catalogue says so.

**M1 does not start with its blockers open.** D3–D14 are twelve written decisions that gate specific M1 tasks, and each is an issue with the tradeoffs already laid out. A task whose blocking decision is still open is not ready to write, however obvious the answer looks at the time. Roadmap §3 is the source: *"none should be resolved by whoever hits it first at 2am."*

`go.mod`, the `Makefile` and `scripts/check-packages.sh` exist, and **all four CI gates are live and required on `main`** — purity, fog boundary, debug isolation, and the gitleaks secret scan. Run `make check` before pushing; it runs what CI runs.

One caveat carried out of M0: `make generate-check` currently reports **VACUOUS, not passing** — there are no generated paths declared until M3 and M5. Do not read it as a green check.

The package layout is fixed by [D01](docs/decisions/D01-package-layout.md) and is not negotiable by convenience:

- `internal/game` is a **leaf** holding the shared vocabulary — `PlayerView`, `Order`, `Config`, `Event`, IDs. It imports nothing, and declares no `any`, `interface{}` or unconstrained type parameter.
- `internal/rules` owns the match state and imports only the standard library plus `internal/game`.
- **`internal/render` and `internal/web` must never directly import `internal/rules`.** Everything they need arrives through `internal/match` as `game` types.

There is no `game.State` and there must never be one.

Treat the two docs as the spec. **The RFC is authoritative on architecture; the GDD is authoritative on rules.** Both are heavily changelogged at the top of each file — read the changelog before assuming a section is current, since later entries correct earlier ones (e.g. GDD mechanics have moved through v0.9 → v2.16, and several early designs — ghost paths, warehouse supply limits, seat-order tie-breaks — were deliberately cut). If GDD and RFC ever seem to disagree, the RFC's own changelog explains which GDD revision it's paired with ("Companion doc" header).

## What Cinzal is

A digital strategy game (2–5 players) with **simultaneous, secret orders** on a partially-hidden procedural graph map. Players run criminal factions smuggling "cargo" between warehouses and border checkpoints across 15 rounds (~30–35 min/match), inferring rivals' positions from public traces rather than direct observation. Full pitch and design pillars: GDD §1–2.

## The one constraint that shapes everything

**Fog is private.** The client must never receive the full match state — only what a given player's fog-of-war entitles them to see. This is a rule of the game (GDD §7.1), not a UX choice, and it is the reason for nearly every architectural decision in the RFC:

- All game state passes through a single projection function, `Project(s State, seat SeatID) PlayerView` (RFC §3, §9). No second path to the client is ever allowed — no debug JSON in production, no template that reaches around it.
- Package-level enforcement: `internal/render` and `internal/web` must never import anything that exposes `MatchState`; only the fog-filtered `PlayerView` type crosses that boundary. This is enforced by a `go list`-based CI check, not by convention (RFC §5, §9.1).
- Server-rendered HTML (HTMX), not a JSON API — a JSON endpoint is a second surface that must independently be kept fog-safe, and it's far easier to over-return fields in JSON than in hand-written HTML (RFC §3).
- `internal/rules` is a pure package — no I/O, no `time`, no `math/rand`, nothing network-touching, enforced by CI. It computes `Resolve()` (the whole round pipeline) and `Project()` (the fog boundary) as deterministic pure functions (RFC §6.1, §6.3).

When implementing anything in this codebase, ask first: does this leak state past the fog boundary? The RFC's fog test suite is explicitly *negative* — it asserts hidden facts are *absent*, not just unused (RFC §16.3) — and that's the standard to hold new code to.

## Planned architecture (specified, not yet implemented)

- **Language/stack:** Go 1.26.5, `templ` for typed templates, HTMX 2.x + SSE for interactivity, `sqlc` + `goose` + `pgx/v5` against Postgres 16. Zero hand-written JavaScript in v1. WASM (client-side rules) and rich map interaction are explicitly deferred to RFC-002 (RFC §4, §10).
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

## How work lands here

Full detail in [CONTRIBUTING.md](CONTRIBUTING.md). The parts an agent gets wrong without being told:

**Everything goes through a pull request.** `main` is protected against direct pushes for the maintainer too, squash-only with linear history, and **every review conversation must be resolved** before merge. One task is one pull request is one commit — sized so `git bisect` lands on something coherent when the determinism check eventually fires.

**Your pull request description becomes the commit message.** `main` squashes with the PR title as subject and the PR body as body. Write it for whoever reads `git log` in a year.

**Work is tracked as three different things**, not one: *decisions* produce a written document in `docs/decisions/` and block the tasks that depend on them; *tasks* produce code; *exit demonstrations* prove a milestone met its criteria, and several can only be shown by breaking something on purpose. A task that cannot cite a GDD or RFC section is not a task — it is a decision, and it should be filed as one.

### Verifying that CodeRabbit actually reviewed

CodeRabbit runs on the free OSS tier and frequently skips a pull request with "Review limit reached" — **and its status check reports success anyway.** This has been the common case on this repository, not the exception.

**There is exactly one reliable signal, and it is negative: a finding still raised against the current head.** That means *not addressed*. Everything else — including the `✅ Addressed` marker — is at best confirmation and at worst nothing at all.

Four separate misreadings, every one made in this repository, produced that sentence:

| Looks like a verdict | Actually means |
|---|---|
| Green status check | Nothing. It is green on a skipped review too. |
| "Review limit reached" | *That attempt* was skipped. An earlier review may already cover earlier commits. |
| **No review record on your latest commit** | Nothing. A clean incremental review posts no review at all. Absence does not distinguish "not reviewed" from "reviewed and clean". |
| **No `✅ Addressed` marker on a finding you fixed** | Nothing. The marker is **not guaranteed** — a finding can be reviewed on a later commit, not re-raised, and never marked. |
| `@coderabbitai review` → "Already reviewed" | The review ran. This is an answer, not a refusal. |

So the procedure after pushing a fix is: **check whether the finding is still raised against the head.** If it is not, and you believe the fix is right, resolve the thread and record *why* in the reply — that is your judgement closing it, not a confirmation, and the difference belongs on the record.

Wait for the review rather than merging without one, and if a merge genuinely cannot wait, say so in the pull request description so the commit on `main` records what went in unreviewed.

*If you write tooling against this: the marker wording **varies with how many commits the fix took** — `Addressed in commit <sha>` for one, `Addressed in commits <sha> to <sha>` for several. A pattern for either form alone silently misses the other and reads it as "unaddressed". Match `Addressed in commits? …`. Both directions of this mistake were made here, while checking for exactly this.*

### The review flow, and what each outcome means

```text
open PR ──▶ auto-review ──▶ findings ──▶ you push a fix ──▶ auto incremental review
                 │                              │                      │
          quota exhausted?               threads resolve          ✅ Addressed
          "limit reached"                (some automatically)      (no new review)
                 │                              │                      │
          wait, then                     resolve the rest ──────▶ merge when CLEAN
          @coderabbitai review              manually
```

| Situation | What to do |
|---|---|
| Quota was available on push | The automatic review fires on its own. **Do not trigger manually** — the command is for when automatic reviews are paused. |
| `"Review limit reached"` | Wait the stated refill (20–45 min), then `@coderabbitai review`. When no time is given, the allowance is out for longer. |
| Manual trigger → `"Already reviewed"` | It ran. Look for the `✅ Addressed` markers; do not keep retrying. |
| You pushed a fix and see no new review | Expected when the fix was clean. Check the finding comments' `updated_at`, not the reviews list. |
| Merge blocked, `BLOCKED`, threads pending | `main` requires conversation resolution. Some threads auto-resolve when your diff makes them outdated; **the rest must be resolved explicitly**, and they will not resolve themselves. |
| You disagree with a finding | Reply on the thread with the reasoning. CodeRabbit answers, and **it concedes when it is wrong** — it did on a suggestion to introduce a `game.State`, which would have inverted D01. |
| Replying to a thread returns 404 | The thread went outdated after your push. Post a pull-request-level comment instead; the reasoning still needs to be on the record. |

**Verify findings before applying them.** They are usually right and have caught real defects here — an unaccounted `.XTestImports` hole, a fog oracle in the draft endpoint, two false claims in the RFC. But one suggestion would have broken the architecture it was reviewing. Check each against the specs before acting, and when a finding is right about the problem and wrong about the fix, say so rather than adopting it.

**Findings often reach further than the file they land on.** Twice here, the correct fix was in a spec section or an unrelated issue that had inherited the same wrong sentence — a gate specification in issue #9 would have rejected the engine on its first commit. When a finding exposes a wrong statement, grep for it.

### Gates fail closed

Every check in this project reports **failure** when it cannot run — missing tool, empty `go list` output, unreadable config. This is not defensive habit: `go list` over a package set that does not exist yet returns nothing, `grep` over nothing succeeds, and a gate built the obvious way reports green having inspected zero packages.

That is the same failure as a review bot reporting success on a review it skipped, and this repository has now watched both happen. **A gate that passes when it cannot run is worse than no gate, because it looks like protection.** Hold new checks to it, and never "fix" a noisy gate by letting it skip.

<!-- rtk-instructions v2 -->
# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)
```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (60-99% savings)
```bash
rtk cargo test          # Cargo test failures only (90%)
rtk go test             # Go test failures only (90%)
rtk jest                # Jest failures only (99.5%)
rtk vitest              # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk pytest              # Python test failures only (90%)
rtk rake test           # Ruby test failures only (90%)
rtk rspec               # RSpec test failures only (60%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)
```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)
```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)
```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma generate     # Prisma without ASCII art (88%)
rtk uv run <cmd>        # Compact uv project command output
```

### Files & Search (60-75% savings)
```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%). Format flags (-c, -l, -L, -o, -Z) run raw.
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)
```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff <f1> <f2>       # Ultra-compact file diff; use `rtk git diff` for repo diffs
```

### Infrastructure (85% savings)
```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl pods        # Compact pod list
rtk kubectl services    # Compact service list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)
```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands
```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% reduction in raw Bash output** on common development operations. This measures bytes stripped from command output, not the resulting reduction in API billing — input tokens (prompt, system instructions, history) are unaffected.
<!-- /rtk-instructions -->