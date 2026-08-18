package rules

// BotPurpose names one BotRNG draw's consumer, following a
// "bot.<tier>.<mechanic>" convention (e.g. "bot.drifter.select") purely for
// RNG-trace legibility (RFC-001 §15.3) — it is never an input to the draw's
// distribution, exactly like Purpose.
//
// Unlike Purpose, BotPurpose is deliberately not a closed enum with a §6.4
// table row: a bot's draw count is a fact about the bot's own algorithm, not
// one the rules determine, so there is no rules-derivable ground truth for a
// completeness table to check itself against
// (docs/decisions/D32-bot-rng-stream.md). Kept in its own file, apart from
// Purpose's declarations in rng_purpose.go, so TestPurposeTableMatchesDeclaredConstants's
// file walk there never has to distinguish the two types by anything but Go's
// own type system.
type BotPurpose string

// NextBot draws exactly like RNG.Next — same HMAC(seed, round, seq, purpose)
// shape, same lazy single-draw contract, same panic on n <= 0 — against this
// BotRNG's own inner stream, entirely separate from Resolve's *RNG. The
// HMAC key already differs per seat via NewBotRNG, so no two BotRNG streams
// can collide regardless of what purpose strings they reuse.
//
// Draws made here are invisible to Consumed/consumed map[Purpose]int and the
// §16.2 invariant it backs: that map lives on the private *RNG this BotRNG
// wraps, and BotRNG exposes no way to read it — §16.2 stays unconditionally
// true regardless of how many seats a round is bot-filled (D32).
func (b *BotRNG) NextBot(purpose BotPurpose, n int) int {
	return b.r.Next(Purpose(purpose), n)
}
