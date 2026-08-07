module github.com/garnizeh/cinzal

// This is a FLOOR, not a pin: Go 1.27 satisfies it. A `toolchain` directive
// would not change that - it only controls which toolchain is fetched when the
// local one is older, and Go rejects one equal to the line above as redundant.
//
// Exact-version enforcement is a CI concern (GOTOOLCHAIN, or a fixed go-version
// in the workflow) and lands with #8. Until then this records intent with a
// floor behind it. See RFC-001 §4 and the r12 changelog entry.
go 1.26.5
