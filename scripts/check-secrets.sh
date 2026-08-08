#!/usr/bin/env bash
#
# Scans for committed credentials — the gate added in #12, scoped in #37.
#
# TWO SCANS, ASKING TWO DIFFERENT QUESTIONS.
#
#   gitleaks dir   does the tree being merged CONTAIN a credential?
#   gitleaks git   does this change ADD one, even if a later commit removes it?
#
# The second is not redundant. A credential added in one commit and deleted in
# the next is absent from the tree and present in the history forever, so the
# tree scan alone reports OK on a secret that has already leaked.
#
# WHY THE HISTORY SCAN IS SCOPED TO A COMMIT RANGE.
#
# `gitleaks git` with no --log-opts scans `--all`: every commit on every ref the
# clone can see. CI checks out with fetch-depth: 0, which fetches every branch,
# so a credential on ANY branch failed the secret scan of EVERY pull request.
# That is what #37 records — three pull requests touching neither secrets nor
# each other failed this gate over a credential in a fourth.
#
# The question a merge gate asks is about the change, so the history scan reads
# the commits the change adds and nothing else. A credential committed in an
# earlier pull request is still caught, by that pull request's own run, which is
# where it is actionable. A repository-wide audit is a legitimate and separate
# thing — a scheduled job, not a per-pull-request gate.
#
# THE FAIL-OPEN THIS SCRIPT EXISTS TO CLOSE, WHICH IS MEASURED AND NOT THEORY.
#
# Handed a range it cannot resolve, gitleaks 8.30.1 does not fail. It reports
#
#     INF 0 commits scanned.
#     INF no leaks found
#
# and exits 0. A typo in the range, a sha the clone does not have, a base and
# head that are the same commit — every one of them is a PASS that inspected
# nothing, and it looks exactly like a clean scan. That is this milestone's
# defining failure mode (see the M0 section of the implementation plan), one
# `--log-opts` away from being reintroduced by the fix for #37 itself.
#
# So the range is validated with `git rev-list` BEFORE gitleaks sees it, and the
# number of commits gitleaks reports scanning is checked AFTER. The first proves
# the range is real; the second proves gitleaks used it.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$ROOT/.gitleaks.toml"

fail() { echo "check-secrets: FAIL: $*" >&2; exit 1; }

command -v gitleaks >/dev/null 2>&1 || fail "gitleaks is not on PATH"
command -v git >/dev/null 2>&1 || fail "git is not on PATH"
[ -r "$CONFIG" ] || fail "$CONFIG is missing or unreadable — the custom rules in it
                    cover the two credential shapes this project handles, so a
                    scan without it is not the scan this gate specifies"
git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
    || fail "$ROOT is not a git work tree — the history scan cannot run"

# --ignore-gitleaks-allow is not optional. Without it a `gitleaks:allow` comment
# on the offending line suppresses the finding, so anyone able to write the line
# is able to switch the gate off for it — a required check with an inline
# bypass. The config allowlist stays, because it is reviewed, versioned, and
# names specific documented placeholders; an inline marker is none of those.
#
# --no-color is here for the parser below: the commit count is read back out of
# this output, and ANSI escapes around the digits would make that unreliable.
ARGS=(--config "$CONFIG" --no-banner --redact --ignore-gitleaks-allow --no-color)

# ---------------------------------------------------------------------------
# Which commits are "the change".
#
# CINZAL_SECRETS_RANGE is set by CI, which knows the base and head of the thing
# being merged. When it is set it is authoritative and MUST resolve: a caller
# that meant to scope the scan and got the range wrong must not silently get a
# scan of nothing.
#
# Unset is the local `make secrets` case, where there is no pull request to read
# a base from. It derives the same range from origin/main when that is possible
# and otherwise falls back to the whole history of HEAD — over-scanning, never
# under-scanning. There is no path here that scans zero commits.
# ---------------------------------------------------------------------------

range="${CINZAL_SECRETS_RANGE:-}"

if [ -n "$range" ]; then
    origin="supplied by the caller"
    count="$(git -C "$ROOT" rev-list --count "$range" 2>&1)" \
        || fail "the commit range '$range' does not resolve in this clone:
                    $count
                    A range gitleaks cannot resolve is a scan of zero commits
                    reported as success. Check the fetch depth of the checkout."
    [ "$count" -gt 0 ] \
        || fail "the commit range '$range' resolved to zero commits.
                    Nothing would have been inspected, and gitleaks would have
                    reported 'no leaks found' for it. Failing instead."
else
    if git -C "$ROOT" rev-parse --verify --quiet origin/main >/dev/null \
        && [ "$(git -C "$ROOT" rev-list --count origin/main..HEAD)" -gt 0 ]; then
        range="origin/main..HEAD"
        origin="derived locally — the commits this branch adds to origin/main"
    else
        range="HEAD"
        origin="fallback — origin/main is unknown or HEAD adds nothing to it, so
                    the whole history of HEAD is scanned"
    fi
    count="$(git -C "$ROOT" rev-list --count "$range")" \
        || fail "could not count the commits in '$range'"
    [ "$count" -gt 0 ] || fail "'$range' resolved to zero commits — nothing to inspect"
fi

# ---------------------------------------------------------------------------
# Scan 1: the tree being merged.
# ---------------------------------------------------------------------------

if ! gitleaks dir "$ROOT" "${ARGS[@]}"; then
    echo "check-secrets: a credential is present in the tree." >&2
    echo "               RFC-001 §18. Remove it, and treat it as disclosed:" >&2
    echo "               rotate the credential rather than only deleting the line." >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Scan 2: the commits the change adds.
# ---------------------------------------------------------------------------

echo "check-secrets: history scan over '$range' ($count commits; $origin)"

if out="$(gitleaks git "$ROOT" "${ARGS[@]}" --log-opts "$range" 2>&1)"; then
    status=0
else
    status=$?
fi
printf '%s\n' "$out" >&2

if [ "$status" -ne 0 ]; then
    echo "check-secrets: a credential was added by a commit in this change." >&2
    echo "               It is in the history even if the tree is now clean, so" >&2
    echo "               deleting the line is not enough — rotate the credential," >&2
    echo "               then rewrite the branch so the commit no longer carries it." >&2
    exit 1
fi

# gitleaks reported success. Prove it looked at something before believing it.
#
# The `|| scanned=""` is load-bearing under `set -o pipefail`: grep exits 1 when
# it matches nothing, which would abort the script here with exit 1 and no
# explanation at all. Failing is right, failing silently is not — the empty
# case is handled immediately below, and it needs to be reached to say so.
scanned="$(printf '%s\n' "$out" | grep -oE '[0-9]+ commits scanned' | grep -oE '^[0-9]+' | tail -1)" \
    || scanned=""

[ -n "$scanned" ] \
    || fail "gitleaks exited 0 without reporting how many commits it scanned.
                    That report is how this gate knows the scan happened at all,
                    so its absence is a failure and not a detail. It most likely
                    means the output format changed with the gitleaks version."

[ "$scanned" -gt 0 ] \
    || fail "gitleaks scanned 0 commits and reported no leaks, though git resolves
                    '$range' to $count. The scan did not happen. This is the exact
                    shape of a gate that passes because it could not run."

# Deliberately NOT an equality check against $count, which was measured and does
# not hold: git rev-list counts merge commits, gitleaks does not report them as
# scanned, so a branch with a merge in range gives 4 against 3. Requiring
# equality would fail those honestly. "It scanned something" is the property
# that distinguishes a real scan from the fail-open above; "it scanned exactly
# what git counted" is a different and wrong claim.

echo "check-secrets: OK — tree clean, $scanned commits of history scanned"
