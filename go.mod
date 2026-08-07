module github.com/garnizeh/cinzal

// On its own this is a FLOOR, not a pin: Go 1.27 satisfies it. A `toolchain`
// directive would not change that - it only controls which toolchain is fetched
// when the local one is older, and Go rejects one equal to the line below as
// redundant.
//
// CI turns it into a pin without repeating the number: setup-go reads this line
// via go-version-file, and GOTOOLCHAIN=local forbids switching to any other
// toolchain. A step then asserts `go env GOVERSION` matches, because a pin that
// fails to apply looks exactly like one that worked.
//
// This line is therefore the single source of the Go version. See
// .github/workflows/ci.yml, RFC-001 §4, and the r12 changelog entry.
go 1.26.5
