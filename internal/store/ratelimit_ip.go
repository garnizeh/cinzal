package store

import (
	"fmt"
	"net"
	"strings"
)

// DeriveIPKey derives the auth_ip rate-limit key from an X-Forwarded-For
// header value, per D20's trusted-hop rule (RFC-001 §12.3).
//
// This is the derivation internal/auth's HTTP layer calls in M5, once that
// package exists — internal/auth is doc.go-only today, and issue #314's own
// acceptance criteria require this rule implemented and tested now, alongside
// the table it feeds keys into. It lives in internal/store rather than
// waiting on internal/auth for exactly that reason; it touches no database
// and imports nothing beyond the standard library, so it costs the package
// graph nothing (purity_test.go's TestPackageGraphExcludesForeignInternalPackages
// is untouched by it).
//
// The key is the xff entry trustedHops positions from the right; everything
// left of that point is client-supplied and ignored, so no header content an
// attacker controls can select their own bucket. This needs at least
// trustedHops entries, not trustedHops+1 — a single trusted hop that appends
// to (rather than replaces) the header produces exactly one entry when the
// client sent none at all, the ordinary case for NGINX's
// $proxy_add_x_forwarded_for and HAProxy's option forwardfor, and that one
// entry is already the real client's address. Fewer entries than trustedHops
// — a misconfiguration, or a connection bypassing the expected proxy chain —
// falls back to remoteAddr instead: a stricter failure, not a bypass, since
// it funnels every client behind a correctly configured load balancer into
// one shared bucket rather than picking an attacker-chosen one.
//
// remoteAddr is expected in host-only form (no port) — the caller (M5)
// strips it, e.g. via net.SplitHostPort on r.RemoteAddr, before calling this.
//
// IPv4 keys on the exact address (/32). IPv6 keys on the /64 prefix, the
// usual single-customer allocation: /128 would let an attacker escape a
// bucket by requesting a fresh address inside a delegation they already
// control.
//
// Returns an error for trustedHops < 1 (a caller/configuration bug — there is
// no meaningful "trust zero hops") and for a chosen entry that does not parse
// as an IP address. Both are fail-closed-friendly: a caller that treats a
// derivation error as "deny" rather than guessing a key never opens a bypass.
func DeriveIPKey(xff string, trustedHops int, remoteAddr string) (string, error) {
	if trustedHops < 1 {
		return "", fmt.Errorf("store: trustedHops must be at least 1, got %d", trustedHops)
	}

	hops := splitForwardedFor(xff)
	chosen := remoteAddr
	if len(hops) >= trustedHops {
		chosen = hops[len(hops)-trustedHops]
	}

	return normalizeIPKey(chosen)
}

// splitForwardedFor splits an X-Forwarded-For value on commas, trims
// whitespace from each entry, and drops empty entries — a trailing or
// doubled comma must not be counted as an extra hop.
func splitForwardedFor(xff string) []string {
	if strings.TrimSpace(xff) == "" {
		return nil
	}
	parts := strings.Split(xff, ",")
	hops := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			hops = append(hops, p)
		}
	}
	return hops
}

// normalizeIPKey parses raw as an IP address and applies D20's prefix rule:
// the exact address for IPv4 (/32), the /64 prefix for IPv6.
func normalizeIPKey(raw string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return "", fmt.Errorf("store: not a valid IP address: %q", raw)
	}

	if v4 := ip.To4(); v4 != nil {
		return v4.String(), nil
	}

	// IPv6: /64 prefix — the usual single-customer allocation (D20).
	masked := ip.Mask(net.CIDRMask(64, 128))
	if masked == nil {
		return "", fmt.Errorf("store: could not apply /64 mask to IPv6 address: %q", raw)
	}
	return masked.String(), nil
}
