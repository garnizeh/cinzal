package rules

import "github.com/garnizeh/cinzal/internal/rules/gen"

// genRand adapts an *RNG into a gen.Rand closure, the shape rules/gen's
// architecture note (internal/rules/gen/doc.go) requires: gen must not
// import internal/rules, so the caller supplies the seeded draw as a plain
// function value instead of the concrete *RNG type. initial (issue #61) and
// TestGenerateWiredToRealRNG* (issue #59) share this one adapter rather than
// each rolling their own.
func genRand(rng *RNG) gen.Rand {
	return func(purpose string, n int) int {
		return rng.Next(Purpose(purpose), n)
	}
}
