package game

import "math/bits"

// RoundSet is a compact bitset of rounds, one bit per RoundNumber. GDD §4
// runs a match to 15 rounds by default, so a uint32 leaves headroom past
// that without needing multiple words (RFC §9.2: "a 15-bit set per node
// per seat").
type RoundSet uint32

// Has reports whether round r is a member of the set. Round numbers outside
// 1-32 are never members, since they cannot have been recorded by With.
func (s RoundSet) Has(r RoundNumber) bool {
	if r < 1 || r > 32 {
		return false
	}
	return s&(1<<uint(r-1)) != 0
}

// With returns a copy of s with round r added. RoundSet is a value type —
// like time.Time, this never mutates the receiver. r outside 1-32 is
// returned unchanged rather than panicking or wrapping, since a round
// number is server-computed and never adversarial input here.
func (s RoundSet) With(r RoundNumber) RoundSet {
	if r < 1 || r > 32 {
		return s
	}
	return s | 1<<uint(r-1)
}

// Count is the number of rounds in the set. NodeStats.ObservedRounds and
// ObscuredRounds are both the Count of a RoundSet (RFC §9.2).
func (s RoundSet) Count() int {
	return bits.OnesCount32(uint32(s))
}

// SeatArchive is one seat's observation history: every round it had sight
// of a node, every round a real trail entry there was suppressed out from
// under it, and every trail entry it has ever received. It is the Board's
// data source (GDD §7.5) and never crosses to any seat but its own owner
// (RFC §9.2).
type SeatArchive struct {
	// Sight records which rounds this seat had sight of each node — a
	// round with no traffic is still an observation, and without this set
	// the Heat Map's rate would silently read as 100% (RFC §9.2: "sight
	// with no traffic is itself an observation").
	Sight map[NodeID]RoundSet

	// Obscured records rounds where a real trail entry at a node was
	// erased world-wide by Rain or Blackout specifically, and only where
	// the suppression actually erased something (D13). These rounds are
	// excluded from the Heat Map's denominator entirely, not counted as a
	// zero-traffic observation.
	Obscured map[NodeID]RoundSet

	// Trail is every trail entry this seat has ever received, stamped
	// with the round it arrived — GDD §7.5's Log tool: "never expires,
	// never scrolls away."
	Trail []StampedTrailEntry
}

// StampedTrailEntry is one archived trail entry plus the round it arrived.
// The archive spans the whole match, unlike PlayerView.Trail, which is
// always the current round and so carries no round of its own.
type StampedTrailEntry struct {
	TrailEntry

	Round RoundNumber // when this entry arrived (RFC §9.2)
}

// NodeStats is the Heat Map's per-node rate, derived from SeatArchive (RFC
// §9.2, GDD §7.5): "tracks on 4 of 6 rounds observed · 67%", with anything
// under 3 observations flagged low-confidence by the reader of this type.
type NodeStats struct {
	// ObservedRounds is the size of Sight[node] — the rate's denominator.
	ObservedRounds int

	// TrafficRounds is how many of those observed rounds carried a tracks
	// entry — the rate's numerator.
	TrafficRounds int

	// ObscuredRounds is the size of Obscured[node] — rounds excluded from
	// the rate entirely, not counted as zero-traffic (D13).
	ObscuredRounds int
}
