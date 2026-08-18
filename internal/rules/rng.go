package rules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
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

// BotRNG is the stream Bot.Decide draws from — never Resolve's own *RNG
// (docs/decisions/D32-bot-rng-stream.md). A distinct type, not a mode flag
// on RNG: BotRNG has no Next method and RNG has no NextBot method, so a call
// site cannot pass the wrong one to Decide and have it compile.
type BotRNG struct{ r *RNG }

// NewBotRNG constructs the stream a bot's Decide call draws from for one
// (seat, round) pair, discarded after that one call. Deterministic in
// (matchSeed, seat), independent of whether seat was bot- or human-
// controlled in any other round — a returning player reclaiming a seat in
// round 8 changes nothing about what round 9's bot RNG would produce if the
// seat reverts to Autopilot later. round is not folded into this derivation
// itself: NewRNG's own HMAC message already includes round on every draw, so
// per-round independence for a fixed seat falls out of reusing it unchanged;
// the only new derivation here is per-seat, which two bot seats must never
// share (RFC-001 §14.5, D32).
//
// seat is encoded as a full uint64, not truncated to 32 bits: game.SeatID is
// a plain int (platform width), and Config.Validate places no upper bound on
// it for a custom player count, so a narrower encoding could in principle
// collide two distinct seats onto the same stream — exactly the non-
// collusion property this derivation exists to guarantee.
func NewBotRNG(matchSeed [32]byte, seat game.SeatID, round int) *BotRNG {
	msg := binary.BigEndian.AppendUint64([]byte("cinzal.bot.rng"), uint64(seat))

	mac := hmac.New(sha256.New, matchSeed[:])
	mac.Write(msg)
	digest := mac.Sum(nil)

	var botSeed [32]byte
	copy(botSeed[:], digest)
	return &BotRNG{r: NewRNG(botSeed, round)}
}
