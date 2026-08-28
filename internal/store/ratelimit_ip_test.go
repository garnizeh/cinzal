package store

import "testing"

// TestDeriveIPKeyTrustedHopIgnoresSpoofedLeftEntries is issue #314's own
// acceptance criterion: a spoofed X-Forwarded-For chain must derive the
// trusted hop's address, never the attacker-supplied leftmost one.
func TestDeriveIPKeyTrustedHopIgnoresSpoofedLeftEntries(t *testing.T) {
	// An attacker prepends two fake entries; only the rightmost was
	// actually appended by the one trusted proxy in front of this app.
	const xff = "6.6.6.6, 7.7.7.7, 203.0.113.9"

	got, err := DeriveIPKey(xff, 1, "10.0.0.1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if got != "203.0.113.9" {
		t.Fatalf("DeriveIPKey(%q, trustedHops=1) = %q, want the rightmost (trusted) hop %q, not an attacker-supplied entry",
			xff, got, "203.0.113.9")
	}
	if got == "6.6.6.6" || got == "7.7.7.7" {
		t.Fatalf("DeriveIPKey returned an attacker-supplied entry: %q", got)
	}
}

// TestDeriveIPKeyTrustedHopsCountsFromTheRight asserts the general
// positional rule, not just the single-hop case: with two trusted proxies,
// the key is the second entry from the right.
func TestDeriveIPKeyTrustedHopsCountsFromTheRight(t *testing.T) {
	const xff = "6.6.6.6, 7.7.7.7, 203.0.113.9"

	got, err := DeriveIPKey(xff, 2, "10.0.0.1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if got != "7.7.7.7" {
		t.Fatalf("DeriveIPKey(%q, trustedHops=2) = %q, want %q", xff, got, "7.7.7.7")
	}
}

// TestDeriveIPKeyFallsBackWhenFewerEntriesThanTrustedHops asserts D20's
// stricter-not-a-bypass fallback: a misconfiguration or a direct connection
// bypassing the expected proxy chain falls back to remoteAddr rather than
// picking an attacker-influenced entry.
func TestDeriveIPKeyFallsBackWhenFewerEntriesThanTrustedHops(t *testing.T) {
	got, err := DeriveIPKey("6.6.6.6", 2, "10.0.0.1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if got != "10.0.0.1" {
		t.Fatalf("DeriveIPKey with only 1 entry and trustedHops=2 = %q, want remoteAddr fallback %q", got, "10.0.0.1")
	}
}

// TestDeriveIPKeyEmptyXFFFallsBackToRemoteAddr is the ordinary single-hop
// case D20 names explicitly: NGINX's $proxy_add_x_forwarded_for and
// HAProxy's option forwardfor produce zero entries when the client sent
// none, and that is already the real client's own address.
func TestDeriveIPKeyEmptyXFFFallsBackToRemoteAddr(t *testing.T) {
	got, err := DeriveIPKey("", 1, "203.0.113.50")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if got != "203.0.113.50" {
		t.Fatalf("DeriveIPKey(\"\", 1, remoteAddr) = %q, want remoteAddr %q", got, "203.0.113.50")
	}
}

// TestDeriveIPKeyIPv4IsExact asserts the /32 rule: an IPv4 address is used
// unmodified, not truncated to any prefix.
func TestDeriveIPKeyIPv4IsExact(t *testing.T) {
	got, err := DeriveIPKey("198.51.100.23", 1, "10.0.0.1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if got != "198.51.100.23" {
		t.Fatalf("DeriveIPKey IPv4 = %q, want the exact address %q (D20: /32)", got, "198.51.100.23")
	}
}

// TestDeriveIPKeyIPv6IsTruncatedToSlash64 asserts D20's IPv6 rule directly:
// two addresses inside the same /64 delegation must derive the same key, so
// an attacker requesting a fresh address inside a delegation they already
// control cannot escape their own bucket.
func TestDeriveIPKeyIPv6IsTruncatedToSlash64(t *testing.T) {
	first, err := DeriveIPKey("2001:db8:abcd:1234:5678::1", 1, "::1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	second, err := DeriveIPKey("2001:db8:abcd:1234:aaaa::2", 1, "::1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if first != second {
		t.Fatalf("two addresses in the same /64 delegation derived different keys: %q vs %q (D20: /64 prefix)", first, second)
	}

	// A different /64 delegation must derive a different key.
	third, err := DeriveIPKey("2001:db8:abcd:9999::1", 1, "::1")
	if err != nil {
		t.Fatalf("DeriveIPKey: %v", err)
	}
	if third == first {
		t.Fatalf("an address in a different /64 delegation derived the same key as %q: %q", first, third)
	}
}

// TestDeriveIPKeyRejectsNonPositiveTrustedHops asserts trustedHops < 1 is a
// configuration error, not a silently accepted "trust nothing" case.
func TestDeriveIPKeyRejectsNonPositiveTrustedHops(t *testing.T) {
	for _, hops := range []int{0, -1} {
		if _, err := DeriveIPKey("1.2.3.4", hops, "10.0.0.1"); err == nil {
			t.Fatalf("DeriveIPKey with trustedHops=%d returned nil error, want one", hops)
		}
	}
}

// TestDeriveIPKeyRejectsUnparseableAddress asserts a chosen entry that is
// not a valid IP address errors rather than silently producing a wrong or
// empty key — fail-closed-friendly, per D20.
func TestDeriveIPKeyRejectsUnparseableAddress(t *testing.T) {
	if _, err := DeriveIPKey("not-an-ip", 1, "10.0.0.1"); err == nil {
		t.Fatal("DeriveIPKey with an unparseable chosen entry returned nil error, want one")
	}
}
