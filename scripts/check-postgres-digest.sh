#!/usr/bin/env bash
#
# Issue #326: asserts compose.yaml's pinned Postgres image and
# internal/store/pgimage.Ref (the digest D46 chose for the whole
# persistence layer's test suite) agree. D46's own reasoning for rejecting
# a compose file as the TEST mechanism was "two descriptions of the same
# pinned image drift apart" — #326 introduces a second, legitimate consumer
# of that same digest anyway (a developer's persistent local database,
# compose.yaml), so this gate is what keeps that second description honest
# instead of letting it drift the way D46 warned against.
#
# THIS CHECK FAILS CLOSED, the same discipline as check-simulate-deps.sh: an
# extraction that finds nothing is a failure, not a vacuous pass.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PGIMAGE_FILE="$ROOT/internal/store/pgimage/pgimage.go"
COMPOSE_FILE="$ROOT/compose.yaml"

fail() { echo "check-postgres-digest: FAIL: $*" >&2; exit 1; }

[ -f "$PGIMAGE_FILE" ] || fail "$PGIMAGE_FILE does not exist"
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist"

go_digest="$(grep -oE 'const Ref = "postgres@sha256:[0-9a-f]+"' "$PGIMAGE_FILE" \
    | grep -oE 'postgres@sha256:[0-9a-f]+' || true)"
[ -n "$go_digest" ] || fail "could not find 'const Ref = \"postgres@sha256:...\"' in $PGIMAGE_FILE"

compose_digest="$(grep -oE 'image: postgres@sha256:[0-9a-f]+' "$COMPOSE_FILE" \
    | grep -oE 'postgres@sha256:[0-9a-f]+' || true)"
[ -n "$compose_digest" ] || fail "could not find an 'image: postgres@sha256:...' line in $COMPOSE_FILE"

if [ "$go_digest" != "$compose_digest" ]; then
    fail "digest mismatch — internal/store/pgimage.Ref is $go_digest, compose.yaml's image is $compose_digest. Update both together (D46, #326)."
fi

echo "check-postgres-digest: OK - compose.yaml and internal/store/pgimage.Ref agree on $go_digest"
