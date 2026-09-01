# D57 — Does the replay bundle store an explicit player count, or derive it from the order log?

**Status:** decided
**Blocks:** M3 — resolves the RFC/code mismatch CodeRabbit's pre-merge "Linked Issues" check flagged on PR #404 (merged) and never got to re-verify against the final head
**Decided:** 2026-08-31
**Issue:** [#407](https://github.com/garnizeh/cinzal/issues/407)

## The question

`internal/match/fold.Fold`/`FoldThrough` take `players int` as a required parameter, genuinely separate from `game.Config` — `Config` carries no scalar player-count field, only maps *keyed by* player count (`MapByPlayers`, `PostCapByPlayers`). A replay bundle has to supply that value from somewhere. Does it store it explicitly, validated against the order log at read time, or derive it purely from the order log's own highest seat index — matching RFC §15.4's and §10.4's literal enumeration of the bundle's contents?

## Why it is open

RFC §10.4 and §15.4 both state the bundle's shape identically:

> A finished match's replay bundle is `{seed, config, orderLog}` — a few kilobytes. (§10.4)

> **Match export.** `{seed, config, orderLog}` for a *finished* match, downloadable by its players. (§15.4)

Neither section, nor issue #322 which cites both verbatim, says how a reader turns those three values into a `players int` for the fold call. That gap was invisible while `cmd/replay` didn't exist — the RFC's enumeration reads as complete because nothing yet consumed the bundle. Issue #322 assumed the literal triple was sufficient and never named the fourth value the fold signature actually requires.

PR #404 answered the gap twice, on the same branch, differently. An earlier commit derived `players` from `max(seat) + 1` over the decoded order log, matching the RFC's literal shape with no new field. A later commit ("cmd/replay: validate bundle player count against order log") added an explicit `Bundle.Players int`, validated in `readBundle` against that same derived value, rejecting a mismatch as corrupted. CodeRabbit's "Linked Issues" pre-merge check caught the deviation on the PR's final head:

> issue `#322` defines the bundle as exactly {seed, config, orderLog}, while this PR stores an additional Players field.

The PR merged (explicit maintainer request) before a review completed against that head — every retry hit CodeRabbit's rate limit — so the finding was never independently argued through before shipping. This decision does that argument now, after the fact, with the code already live.

## Options

**A — Revert to pure derivation.** Drop `Bundle.Players`; `readBundle` computes `players` the same way `exportBundleFromDB` currently cross-checks it, from `max(seat) + 1` over the decoded order log. Matches RFC §10.4/§15.4 literally, with no RFC text change needed. Cost: a bundle whose file was truncated after export — every order-log row for the highest seat lost, by disk corruption, a bad manual edit, or an interrupted download — silently decodes as a *smaller, complete* match instead of failing. Nothing in the literal triple can distinguish "this match really had 3 players" from "this file used to have orders for a 4th seat and doesn't now." The bundle exists specifically to be "the perfect bug report" (§15.4) attached to an issue by a player, i.e. handled and re-uploaded outside any system that could otherwise guarantee its integrity — exactly the file most likely to get truncated by an email client, a chat upload limit, or a copy-paste into an issue body.

**B — Keep the explicit, validated `Players` field (shipped in PR #404).** `exportBundleFromDB` writes the match's actual `match_players` row count; `readBundle` decodes it and rejects any bundle where it disagrees with the order log's derived count. Cost: one more field on the wire, one more line in the RFC's enumeration, and a decision document to reconcile the two — this document. In exchange it converts a silent reinterpretation into a hard decode error naming both numbers, the same shape D44 already chose for `Config`'s own corruption handling ("a hard decode error, never a default," on any version mismatch or incomplete field) and the same shape D47 chose for `Order`'s zero-value encoding. It is not new machinery — `readBundle` already derives the same `max(seat) + 1` value Option A would use as its *only* source of truth; Option B keeps that derivation and adds one more independent number to check it against, which is strictly more information, never less.

**C — Store `Players` but skip the cross-check (trust the field alone).** Considered and rejected without much weighing: this drops the exact property that makes Option B worth its cost — a hand-edited or corrupted `Players` value with a matching, also-edited order log would decode cleanly, and a truncated file that also had `Players` clumsily "fixed" to match would pass too. A stored-but-unchecked field is worse than no field: it looks like a safeguard and isn't one.

## Decision

**B — the explicit `Bundle.Players` field, validated against the order log's derived count in `readBundle`, is correct and stays.** RFC §10.4 and §15.4 are corrected to name the bundle's real shape, `{seed, config, players, orderLog}`, matching what PR #404 already shipped. No behavioural code change follows from this decision — `cmd/replay/bundle.go`'s `Bundle` struct, `exportBundleFromDB`, `writeBundle`, and `readBundle` are unamended; this document brings the RFC into agreement with code that was already correct on the substance CodeRabbit's finding raised, even though the finding was right that the two disagreed at merge time. `cmd/replay/doc.go`'s own package comment, which quoted the stale `{seed, config, orderLog}` triple, is corrected in the same PR to match.

## Reasoning

**This is the project's own standing principle, not a new one invented for this bundle.** CLAUDE.md's "Absence of a signal is not evidence of a state" is exactly the shape of the corruption case Option A cannot catch: a missing seat's orders in a truncated file is not evidence the match had one fewer player, but pure derivation reads it as exactly that, silently. A gate or a decoder "built the obvious way" — deriving from what's present — "reports green having inspected zero" of the rows that are missing. The validated `Players` field is a fail-closed check in the same family as `Config`'s hard-decode-on-mismatch policy (D44) and `Order`'s explicit zero-value table (D47): both already accept "one more explicit field or check, to convert a silent wrong answer into a loud correct rejection" as worth its cost in this codebase, for data of exactly this shape — replay inputs that must reproduce a match byte-for-byte or fail, never approximately.

**The RFC's literal enumeration was never actually complete**, independent of which option won. `players` has to come from somewhere for the fold call to typecheck, and neither §10.4 nor §15.4 named a source. Reading `{seed, config, orderLog}` as *forbidding* a fourth field is reading more into three comma-separated names than the sections argue for — they describe what downloads to a player and what makes a bug report reproducible, not an exhaustive wire-format contract. The gap CodeRabbit found is real (the code and the RFC's words disagreed), but the fix that best serves what §15.4 is actually for — "attach it to an issue and `cmd/replay` reproduces the exact match," including a truncated attachment failing loudly instead of quietly reproducing the wrong match — is closing the gap in the RFC's favor toward Option B, not walking the code back to a strictly weaker guarantee to match three words that were never trying to exclude a fourth.

**Reverting would cost a real property for no offsetting gain.** Nothing about Option A is simpler at the call site — `readBundle` already computes the derived count today; Option A only deletes the field and the check against it, which is less code but strictly less information. There is no performance, storage, or fog concern the extra `int` field creates: it is not `PlayerView`-reachable, discloses nothing beyond the player count every match participant already sees on the board, and consumes no randomness.

## Consequences

- **RFC §10.4** — "A finished match's replay bundle is `{seed, config, orderLog}`" becomes "`{seed, config, players, orderLog}`", with a changelog entry citing this decision.
- **RFC §15.4** — the "Match export" bullet's `{seed, config, orderLog}` gets the same correction, same changelog entry.
- **`cmd/replay/bundle.go`** — no code change. `Bundle.Players`, `exportBundleFromDB`'s population of it from `ListMatchPlayers`, and `readBundle`'s cross-check against the order log's derived count are all confirmed correct as shipped.
- **Issue #322** — closed, historical text unedited (it already cites the RFC by reference rather than repeating the shape as its own claim); no action.
- **CodeRabbit's finding is resolved as "the spec was updated to bless it"**, the first branch of the choice the finding itself offered ("remove the stored field... or update issue #322 and the applicable specification to approve the additional field explicitly").
- **Reversible, but not for free, if a future corruption case shows the check is wrong** (for example, a legitimate reason a finished match's order log could omit a seat's rows entirely, which no case in the current GDD/RFC describes): dropping `Bundle.Players` from the struct is not a silent no-op for bundles already exported under this decision. `readBundle` calls `dec.DisallowUnknownFields()`, so a bundle file that already carries a `"players"` key would fail to decode under a `Bundle` struct that no longer declares that field, with an unknown-field error — not "the field just goes unread." Reverting later needs an explicit compatibility step (tolerating and ignoring a present `"players"` key for one transition period, or a version marker the way `Config` carries one per D44) rather than a bare field deletion.
