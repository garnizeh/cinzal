package rules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// RNG is the engine's single source of randomness (RFC-001 §6.4). Every draw
// is a pure function of (seed, round, seq, purpose, n): two RNGs constructed
// with the same seed and round, driven through the same sequence of Next
// calls, produce the same sequence of values.
//
// An RNG is bound to one round — Resolve constructs a fresh one, seeded
// identically, at the start of every round. It is threaded through
// resolution as a single value: never branched, never copied, never passed
// to a goroutine.
type RNG struct {
	seed  [32]byte
	round int
	seq   uint32

	consumed map[Purpose]int
}

// NewRNG constructs an RNG bound to round, deriving every draw from seed.
func NewRNG(seed [32]byte, round int) *RNG {
	return &RNG{
		seed:     seed,
		round:    round,
		consumed: make(map[Purpose]int),
	}
}

// Round reports the round this RNG is bound to.
func (r *RNG) Round() int { return r.round }

// Seq reports the total number of draws made so far, across every purpose,
// since construction.
func (r *RNG) Seq() uint32 { return r.seq }

// Next draws a deterministic value in [0, n), derived from
// HMAC(seed, round || seq || purpose), and increments seq. purpose never
// affects the value's distribution; it is recorded in the debug trace
// (RFC-001 §15.3) and in this RNG's own consumption count (§16.2) so a
// divergent replay names the draw that went wrong rather than only that one
// did.
//
// n <= 0 is a programming error, not a runtime condition to recover from — a
// silent 0 would be an index into an empty candidate set — so it panics
// rather than returning 0.
//
// Next returns exactly one draw per call. There is no NextN: a batch API
// would make it easy to draw ahead of a branch that might not run, which is
// exactly the early-termination hazard the §6.4 lazy rule exists to prevent
// ("an implementation that draws both blind steps up front and then
// discards one"). The two mandated multi-draw shapes, PartialFisherYates and
// ShuffleConstrained, call Next once per index they consume and document
// that cost themselves.
func (r *RNG) Next(purpose Purpose, n int) int {
	if n <= 0 {
		panic(fmt.Sprintf("rules: RNG.Next(%q, %d): n must be positive", purpose, n))
	}

	msg := make([]byte, 0, 8+4+len(purpose))
	msg = binary.BigEndian.AppendUint64(msg, uint64(r.round))
	msg = binary.BigEndian.AppendUint32(msg, r.seq)
	msg = append(msg, purpose...)

	mac := hmac.New(sha256.New, r.seed[:])
	mac.Write(msg)
	digest := mac.Sum(nil)

	r.seq++
	r.consumed[purpose]++

	draw := binary.BigEndian.Uint64(digest[:8])
	return int(draw % uint64(n))
}

// Consumed reports how many draws purpose has consumed so far on this
// RNG — which is to say, within the round it is bound to. RFC-001 §16.2's
// invariant, "rng.seq consumed == predicted, per the §6.4 table, asserted
// per round", reads this every round of every golden replay (issue #77).
func (r *RNG) Consumed(purpose Purpose) int { return r.consumed[purpose] }
