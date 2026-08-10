package gen

// Rand draws a deterministic value in [0, n), attributed to purpose in the
// caller's own consumption ledger — *rules.RNG.Next's exact contract, taken
// here as a plain function value rather than the concrete type, because
// this package must not import internal/rules (see doc.go). The caller
// (internal/rules, when it builds a match's initial state) supplies its RNG
// as a closure: func(purpose string, n int) int { return rng.Next(Purpose(purpose), n) }.
//
// n <= 0 is a programming error in this package, exactly as it is for
// *rules.RNG.Next — every call site here computes n from a candidate slice
// or a fixed range that is never empty by construction.
type Rand func(purpose string, n int) int

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
