package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoFallbackDSN is #310's own acceptance criterion: "there is no
// fallback DSN anywhere in the package, asserted by a source-level test
// that greps for a hard-coded postgres:// or localhost outside _test.go
// files." A future edit that reaches for a "just for local dev" default
// would otherwise reintroduce exactly the fallback RFC-001 §18 rules out —
// this fails the build the moment it lands, rather than trusting review to
// catch it.
//
// This inspects string literals via go/parser rather than raw text, so a
// doc comment that merely *discusses* localhost (as this package's own
// pool.go does, explaining why Open must reject an empty DSN before
// pgxpool falls back to one) does not trip it — only actual source-level
// use of the forbidden substrings does.
//
// This fails closed: an unreadable package directory, a file that fails to
// parse, or zero .go files found are all test failures, not skips — the
// same property scripts/check-packages.sh's own header explains for the CI
// gates.
func TestNoFallbackDSN(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read internal/store: %v", err)
	}

	forbidden := []string{"postgres://", "localhost"}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		checked++

		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("cannot parse %s: %v", e.Name(), err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				val = lit.Value
			}
			for _, needle := range forbidden {
				if strings.Contains(val, needle) {
					t.Errorf("%s:%s contains string literal %q — a hard-coded fallback DSN, forbidden by RFC-001 §18",
						e.Name(), fset.Position(lit.Pos()), val)
				}
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no non-test .go files found in internal/store — this check inspected nothing")
	}
}

// TestPackageGraphExcludesForeignInternalPackages is #310's other
// acceptance criterion: "go list -deps ./internal/store contains no
// internal/ package other than internal/game and internal/rules;
// specifically it must never contain internal/web, internal/render,
// internal/match or internal/mail." A store package that could reach web or
// render would let a persistence bug reintroduce a fog leak through the
// back door — the same reasoning RFC-001 §5 gives for the render/web import
// rule, one layer down.
//
// Fails closed: a `go list` that errors, or returns nothing, is a test
// failure — not a pass that inspected zero packages.
func TestPackageGraphExcludesForeignInternalPackages(t *testing.T) {
	const module = "github.com/garnizeh/cinzal"
	allowed := map[string]bool{
		module + "/internal/store": true,
		module + "/internal/game":  true,
		module + "/internal/rules": true,
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not on PATH: %v", err)
	}

	// "." is internal/store itself — go test's working directory is always
	// the package directory, so this is exactly `go list -deps
	// ./internal/store` run from the module root.
	cmd := exec.Command(goBin, "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . did not succeed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatal("go list -deps . reported no packages")
	}

	// The not-vacuous signal is that go list saw internal/store itself and
	// pulled in a real dependency tree (stdlib plus pgx today) — not that it
	// found a *foreign* internal/ package, since internal/store legitimately
	// imports neither internal/game nor internal/rules yet (#310's own
	// scope: "nothing here knows what a match is"). Requiring a foreign hit
	// to prove the check ran would make this test fail on the correct,
	// clean state.
	sawSelf := false
	for _, pkg := range lines {
		if pkg == module+"/internal/store" {
			sawSelf = true
		}
	}
	if !sawSelf {
		t.Fatal("go list -deps . did not include internal/store itself — this check inspected nothing")
	}
	const minDeps = 5 // stdlib + pgx alone is dozens; guards against a truncated/empty result slipping past sawSelf
	if len(lines) < minDeps {
		t.Fatalf("go list -deps . reported only %d packages, suspiciously few: %v", len(lines), lines)
	}

	// Scoped to this module's own internal/ tree — go list -deps also
	// reports the standard library's *own* internal packages (e.g.
	// crypto/internal/fips140deps/...), which are not the concern here and
	// must not be flagged as if they were.
	for _, pkg := range lines {
		if !strings.HasPrefix(pkg, module+"/internal/") {
			continue
		}
		if !allowed[pkg] {
			t.Errorf("internal/store depends on %s, which is not internal/game or internal/rules", pkg)
		}
	}
}

// TestPackageDirectoryMatchesModule is a cheap guard that the test above is
// actually running from internal/store and not some other working
// directory a future refactor might leave it in.
func TestPackageDirectoryMatchesModule(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if filepath.Base(wd) != "store" {
		t.Fatalf("test is running from %q, not internal/store", wd)
	}
}
