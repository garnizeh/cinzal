#!/usr/bin/env bash
#
# Fixture coverage for scripts/check-rules-purity.sh and the AST-based
# call-site checker it now delegates to, scripts/check-fmt-purity.go (issue
# #297). Not wired into `make check` directly — `make purity-selftest` is,
# and that target is on the check: line, same as bots-isolation-selftest and
# bench-regression-selftest for the same reason: deterministic, synthetic
# input, none of a real package's churn.
#
# check-rules-purity.sh's own tree-inspection logic (go list against
# internal/rules, internal/telemetry, internal/bots) has no target-directory
# argument the way check-bots-isolation.go does, because before issue #297 it
# never needed fixture coverage of its own. Rather than bolt on a parallel
# code path, this script points the whole gate at a synthetic module instead:
# PURITY_TEST_ROOT/_DIR/_SELF/_ALLOWED (see check-rules-purity.sh's own
# comment on them) swap in a one-tree, one-package fixture for the real
# three-tree list. Every fixture below is therefore a real, `go list`-able
# module — needed because the import-exactness check shells out to the Go
# toolchain, unlike check-bots-isolation.go's pure go/parser walk.
#
# scripts/check-rules-purity_test.sh is this file's own name, deliberately
# matching scripts/check-bots-isolation_test.sh's and
# scripts/check-bench-regression_test.sh's — the standard this self-test is
# held to: not just the gate's success path, but its failure modes,
# exercised one at a time.

set -euo pipefail

cd "$(dirname "$0")/.."

GATE="./scripts/check-rules-purity.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# run <fixture-root> [dir] — invokes the gate against a fixture module's
# fixture-root/[dir] package and captures output and exit code without
# letting `set -e` abort this script on the (expected) non-zero cases.
run() {
	local fixture_root="$1" dir="${2:-pkg}"
	out="$(PURITY_TEST_ROOT="$fixture_root" PURITY_TEST_DIR="$dir" PURITY_TEST_SELF="fixture" PURITY_TEST_ALLOWED="" "$GATE" 2>&1)" && code=0 || code=$?
}

gomod() {
	printf 'module fixture\n\ngo 1.26\n' >"$1/go.mod"
}

# ---------------------------------------------------------------------------
# Case 1 (clean): fmt used only for Sprintf/Errorf, no forbidden import.
# ---------------------------------------------------------------------------
clean="$tmp/clean"
mkdir -p "$clean/pkg"
gomod "$clean"
cat >"$clean/pkg/x.go" <<'EOF'
package pkg

import "fmt"

func Describe(n int) (string, error) {
	if n < 0 {
		return "", fmt.Errorf("negative: %d", n)
	}
	return fmt.Sprintf("n=%d", n), nil
}
EOF
run "$clean"
[ "$code" -eq 0 ] || fail "clean fixture: expected exit 0, got $code: $out"
printf '%s\n' "$out" | grep -q '^check-rules-purity: OK' || fail "clean fixture: expected an OK line, got: $out"

# ---------------------------------------------------------------------------
# Case 2 (violation, io import): the gap issue #297 names first. io.ReadAll
# on an injected reader — importing io does nothing by itself, but the point
# is that the import alone must already fail regardless of what the reader
# turns out to be at the call site.
# ---------------------------------------------------------------------------
badio="$tmp/bad-io"
mkdir -p "$badio/pkg"
gomod "$badio"
cat >"$badio/pkg/x.go" <<'EOF'
package pkg

import "io"

func Read(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
EOF
run "$badio"
[ "$code" -eq 1 ] || fail "io import: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q -- '-> io$' || fail "io import: expected an offending-import line naming io, got: $out"

# ---------------------------------------------------------------------------
# Case 3 (violation, direct fmt.Println call): the AST walk's baseline
# behaviour — must still catch what the textual predecessor already caught.
# ---------------------------------------------------------------------------
directfmt="$tmp/direct-fmt"
mkdir -p "$directfmt/pkg"
gomod "$directfmt"
cat >"$directfmt/pkg/x.go" <<'EOF'
package pkg

import "fmt"

func Emit() {
	fmt.Println("hello")
}
EOF
run "$directfmt"
[ "$code" -eq 1 ] || fail "direct fmt.Println: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'fmt.Println performs I/O' || fail "direct fmt.Println: expected the I/O violation line, got: $out"

# ---------------------------------------------------------------------------
# Case 4 (violation, indirect fmt.Println reference): the gap issue #297
# names second — assigning fmt.Println to a local before calling it, which
# the old `\bfmt\.Println\(` grep never saw because the selector was never
# immediately followed by "(".
# ---------------------------------------------------------------------------
indirectfmt="$tmp/indirect-fmt"
mkdir -p "$indirectfmt/pkg"
gomod "$indirectfmt"
cat >"$indirectfmt/pkg/x.go" <<'EOF'
package pkg

import "fmt"

func Emit() {
	p := fmt.Println
	p("hello")
}
EOF
run "$indirectfmt"
[ "$code" -eq 1 ] || fail "indirect fmt.Println: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'fmt.Println performs I/O' || fail "indirect fmt.Println: expected the I/O violation line, got: $out"

# ---------------------------------------------------------------------------
# Case 5 (violation, aliased fmt import): the AST rewrite can resolve an
# alias correctly instead of rejecting it outright the way the textual check
# had to — this fixture proves the resolved alias still catches a forbidden
# selector through it, not that aliasing itself is now silently permitted.
# ---------------------------------------------------------------------------
aliasedfmt="$tmp/aliased-fmt"
mkdir -p "$aliasedfmt/pkg"
gomod "$aliasedfmt"
cat >"$aliasedfmt/pkg/x.go" <<'EOF'
package pkg

import f "fmt"

func Emit() {
	f.Println("hello")
}
EOF
run "$aliasedfmt"
[ "$code" -eq 1 ] || fail "aliased fmt.Println: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'fmt.Println performs I/O' || fail "aliased fmt.Println: expected the I/O violation line, got: $out"

# ---------------------------------------------------------------------------
# Case 6 (violation, dot import of fmt): still rejected outright — resolving
# a bare Println after a dot import would need type information this walk
# deliberately does not have (see check-fmt-purity.go's own header).
# ---------------------------------------------------------------------------
dotfmt="$tmp/dot-fmt"
mkdir -p "$dotfmt/pkg"
gomod "$dotfmt"
cat >"$dotfmt/pkg/x.go" <<'EOF'
package pkg

import . "fmt"

func Emit() {
	_ = Sprintf("hello")
}
EOF
run "$dotfmt"
[ "$code" -eq 1 ] || fail "dot import of fmt: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'dot-imports' || fail "dot import of fmt: expected the dot-import violation line, got: $out"

# ---------------------------------------------------------------------------
# Case 7 (infra failure, parse error): must fail rather than silently skip
# the broken file — the same fail-closed standard as every other gate here.
# ---------------------------------------------------------------------------
broken="$tmp/parse-error"
mkdir -p "$broken/pkg"
gomod "$broken"
cat >"$broken/pkg/x.go" <<'EOF'
package pkg

func broken( {
EOF
run "$broken"
[ "$code" -eq 1 ] || fail "parse error: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'check-fmt-purity: FAIL:' || fail "parse error: expected the infra-failure prefix, got: $out"

if [ "$failed" -eq 0 ]; then
	echo "check-rules-purity_test: PASS"
fi
exit "$failed"
