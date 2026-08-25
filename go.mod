module github.com/garnizeh/cinzal

// On its own this is a FLOOR, not a pin: Go 1.28 satisfies it. A `toolchain`
// directive would not change that. It names a MINIMUM toolchain to switch to,
// so it raises the floor rather than capping it - and one equal to the line
// below is redundant and does not survive: `go build` errors with "updates to
// go.mod needed" until `go mod tidy` runs, and tidy deletes the line. There is
// no directive that pins a version from inside go.mod.
//
// CI turns it into a pin without repeating the number: setup-go reads this line
// via go-version-file, and GOTOOLCHAIN=local forbids switching to any other
// toolchain. A step then asserts `go env GOVERSION` matches, because a pin that
// fails to apply looks exactly like one that worked.
//
// This line is therefore the single source of the Go version. See
// .github/workflows/ci.yml, RFC-001 §4, and the r12 changelog entry.
go 1.27.0
