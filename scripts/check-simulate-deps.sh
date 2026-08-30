#!/usr/bin/env bash
#
# Asserts cmd/simulate's dependency graph is exactly what RFC-001 §16.4
# claims for it — "It needs only rules and bots" — plus the packages that
# claim always implied but never stated by name: internal/game (rules and
# bots both hand cmd/simulate game.Config/game.Order/game.Event values it
# has to name to call them), internal/telemetry (D34: the GDD §22 metric set
# this whole command exists to produce), and internal/opsmetrics (D45: the
# two RFC §7.3 fold metrics — cmd/simulate cannot import internal/match/fold
# to reach FoldMeasured, so RunMatch calls opsmetrics.Default.Observe
# directly around its own per-match Resolve sequence; issue #320 widened
# this allow-list in the same PR that wired the call in, per D45's own
# consequence). internal/rules/gen counts as internal/rules' own family,
# pulled in transitively by rules.NewMatch, not a sixth package. Issue #199's
# own acceptance criterion turns that sentence into something CI enforces
# rather than only states.
#
# THIS CHECK FAILS CLOSED, the same discipline as check-packages.sh: a
# `go list -deps` that silently produced nothing would make an empty diff
# look like a pass, which is the same failure as a review bot reporting
# success on a review it skipped.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

fail() { echo "check-simulate-deps: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

deps="$(cd "$ROOT" && go list -deps ./cmd/simulate)" || fail "go list -deps ./cmd/simulate did not succeed"
[ -n "$deps" ] || fail "go list -deps ./cmd/simulate reported nothing"

internal_deps="$(printf '%s\n' "$deps" | grep -E "^$MODULE/internal/" || true)"
[ -n "$internal_deps" ] || fail "go list -deps ./cmd/simulate named no internal/ package at all"

for req in internal/rules internal/bots internal/game internal/telemetry internal/opsmetrics; do
    printf '%s\n' "$internal_deps" | grep -qx "$MODULE/$req" \
        || fail "go list -deps ./cmd/simulate does not contain $req, required by RFC-001 §16.4 / issue #199 (internal/opsmetrics: D45/#320)"
done

foreign="$(printf '%s\n' "$internal_deps" | grep -vE "^$MODULE/internal/(rules(/gen)?|bots|game|telemetry|opsmetrics)\$" || true)"

if [ -n "$foreign" ]; then
    echo "check-simulate-deps: cmd/simulate may depend only on internal/rules, internal/bots, internal/game, internal/telemetry and internal/opsmetrics (RFC-001 §16.4, D45)" >&2
    printf '  %s\n' $foreign >&2
    exit 1
fi

echo "check-simulate-deps: OK - $(printf '%s\n' "$internal_deps" | wc -l | tr -d ' ') internal package(s), all within the RFC-001 §16.4 set"
