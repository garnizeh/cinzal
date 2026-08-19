//go:build ignore

// Command check-bots-isolation asserts that internal/bots's production code
// cannot name rules.MatchState, the graph, or the match seed — issue #195,
// RFC-001 §14.5: "What bots must never do: read MatchState, the graph, or
// the seed."
//
// WHY THIS CANNOT BE AN IMPORT-GRAPH CHECK.
//
// scripts/check-fog-boundary.sh works because internal/render and
// internal/web have no legitimate reason to import internal/rules at all —
// forbidding the import forbids the name. internal/bots is different: it
// legitimately imports internal/rules for Legal, Affordances, Steps and
// BotRNG (bot.go, legalspace.go, runner.go). The forbidden thing here is not
// the import, it is which IDENTIFIERS the import is used to reach — so this
// walks declarations the way scripts/check-game-types.go walks
// internal/game, not the import graph the way check-fog-boundary.sh does.
//
// WHY AN ALLOW-LIST FILE, RATHER THAN A HARDCODED DENY-LIST.
//
// scripts/bots-isolation-allowlist.txt names every rules identifier
// internal/bots may reference. A rules.X selector whose X is not on that
// list fails, full stop — including a symbol that does not exist in
// internal/rules yet. That is deliberate: a deny-list of MatchState and its
// neighbours passes silently the day rules exports something else just as
// unsafe; an allow-list must be widened on purpose, in a reviewed pull
// request, before the new symbol can be named at all.
//
// A qualified identifier catches every position the type-level wording
// names in one mechanism: MatchState named as a declared type, a parameter,
// a struct field, a `:=`-inferred local (`s, err := rules.NewMatch(...)`
// binds s's type without ever spelling MatchState, but rules.NewMatch itself
// is the selector this walk sees and NewMatch is not on the allow-list), and
// a type assertion (`x.(rules.Graph)` names rules.Graph as a selector too).
// Nothing here resolves what a selector's type actually IS — it does not
// need to, because the allow-list is checked by name, not by shape.
//
// THE [32]byte CHECK IS SEPARATE, AND DOES NOT MENTION rules AT ALL.
//
// RFC-001 §6.4's seed is [32]byte (internal/rules/rng.go). A hand-rolled
// helper that takes or stores a bare [32]byte never has to name the rules
// package to be carrying the seed, so this is a second, independent walk:
// every array type of exactly this shape, anywhere a type can appear, fails
// — regardless of whether the file imports internal/rules at all.
//
// WHY TEST FILES ARE OUT OF SCOPE.
//
// internal/bots/*_test.go legitimately builds real matches — rules.NewMatch,
// rules.Project, rules.Resolve — to drive bots against realistic corpora
// (legalspace_test.go's TestSampleCorpusFromGoldenMatches,
// operator_test.go). That is test harness code proving Decide's behaviour
// against real state, not Decide itself reading state it was never handed;
// forbidding it there would forbid testing bots against anything but
// hand-built fixtures. internal/bots/bot_test.go's own
// TestPackageNamesNoMatchStateNowhere already covers every file in the
// package, test files included, for the literal identifier MatchState — see
// that test's doc comment for why its coverage is deliberately narrower than
// this gate's (it predates this gate; this gate is what issue #190 named as
// its type-level successor).
//
// THIS CHECK FAILS CLOSED. A parse error, an unreadable allow-list, or a
// directory holding nothing but doc.go each exit non-zero rather than
// reporting success on an inspection that did not happen.
//
// Run: go run scripts/check-bots-isolation.go [target-dir] [allowlist-file]
// Both arguments are optional and exist so the self-test
// (scripts/check-bots-isolation_test.sh) can point this at a fixture
// directory instead of the real package.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultTarget    = "internal/bots"
	defaultAllowlist = "scripts/bots-isolation-allowlist.txt"
	rulesImportPath  = "github.com/garnizeh/cinzal/internal/rules"
)

type finding struct {
	pos  token.Position
	what string
}

func main() {
	target := defaultTarget
	if len(os.Args) > 1 && os.Args[1] != "" {
		target = os.Args[1]
	}
	allowlistPath := defaultAllowlist
	if len(os.Args) > 2 && os.Args[2] != "" {
		allowlistPath = os.Args[2]
	}

	root, err := os.Getwd()
	if err != nil {
		fatal("could not determine the working directory: %v", err)
	}

	allowlist, err := readAllowlist(allowlistPath)
	if err != nil {
		fatal("could not read the allow-list %s: %v", allowlistPath, err)
	}

	dir := target
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, target)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fatal("could not read %s: %v", target, err)
	}

	var prodFiles []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		prodFiles = append(prodFiles, name)
	}
	sort.Strings(prodFiles)

	// "internal/bots containing no Go files other than doc.go" — the exact
	// wording issue #195 gives for VACUOUS. Zero production files at all
	// (not even doc.go) hits the same message: either way, nothing here
	// could possibly declare a forbidden identifier, and reporting a plain
	// pass on that would be the same failure generate-check's own VACUOUS
	// path exists to name instead of hiding.
	inspectable := 0
	for _, name := range prodFiles {
		if name != "doc.go" {
			inspectable++
		}
	}
	if inspectable == 0 {
		fmt.Fprintf(os.Stderr, "check-bots-isolation: VACUOUS — %s holds no production Go file but doc.go.\n", target)
		fmt.Fprintf(os.Stderr, "                       Nothing was inspected, so this is not a pass. See\n")
		fmt.Fprintf(os.Stderr, "                       the Makefile's generate-check for the same convention.\n")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	var findings []finding
	filesScanned := 0

	for _, name := range prodFiles {
		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fatal("could not parse %s: %v", filepath.Join(target, name), err)
		}
		filesScanned++

		rulesAlias, dotImportPos := rulesBinding(file)
		if dotImportPos != token.NoPos {
			findings = append(findings, finding{
				fset.Position(dotImportPos),
				fmt.Sprintf("dot-imports %s, which binds every exported identifier into file scope and defeats the allow-list check below", rulesImportPath),
			})
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				if rulesAlias == "" {
					return true
				}
				if x, ok := node.X.(*ast.Ident); ok && x.Name == rulesAlias {
					if !allowlist[node.Sel.Name] {
						findings = append(findings, finding{
							fset.Position(node.Pos()),
							fmt.Sprintf("rules.%s is not on the isolation allow-list (%s)", node.Sel.Name, allowlistPath),
						})
					}
				}
			case *ast.ArrayType:
				if isSeedShaped(node) {
					findings = append(findings, finding{
						fset.Position(node.Pos()),
						"a [32]byte — the match seed's own shape (internal/rules/rng.go); internal/bots must never take, hold, or return one",
					})
				}
			}
			return true
		})
	}

	if len(findings) > 0 {
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].pos.Filename != findings[j].pos.Filename {
				return findings[i].pos.Filename < findings[j].pos.Filename
			}
			return findings[i].pos.Offset < findings[j].pos.Offset
		})
		fmt.Fprintf(os.Stderr, "check-bots-isolation: %s may not name MatchState, the graph, or the seed.\n", target)
		fmt.Fprintf(os.Stderr, "                       RFC-001 §14.5. Offending references:\n")
		for _, f := range findings {
			rel, err := filepath.Rel(root, f.pos.Filename)
			if err != nil {
				rel = f.pos.Filename
			}
			fmt.Fprintf(os.Stderr, "  %s:%d:%d: %s\n", rel, f.pos.Line, f.pos.Column, f.what)
		}
		os.Exit(1)
	}

	fmt.Printf("check-bots-isolation: OK - %d production file(s) in %s, no reference outside %s's allow-list\n", filesScanned, target, allowlistPath)
}

// rulesBinding reports the identifier internal/rules is bound to in file —
// its import alias, or the default package name "rules" when unaliased — and
// dotImportPos, the position of the import spec itself when it is a dot
// import instead (token.NoPos otherwise). alias is empty whenever the file
// does not import internal/rules at all, which is common: most of
// internal/bots's non-test files do, but nothing requires it.
func rulesBinding(file *ast.File) (alias string, dotImportPos token.Pos) {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != rulesImportPath {
			continue
		}
		if imp.Name == nil {
			return "rules", token.NoPos
		}
		switch imp.Name.Name {
		case "_":
			return "", token.NoPos // blank import binds no identifier; nothing to select
		case ".":
			return "", imp.Pos()
		default:
			return imp.Name.Name, token.NoPos
		}
	}
	return "", token.NoPos
}

// isSeedShaped reports whether t is exactly [32]byte — a fixed-length array
// literal, not a slice (Len == nil) and not any other length or element
// type.
func isSeedShaped(t *ast.ArrayType) bool {
	if t.Len == nil {
		return false
	}
	lit, ok := t.Len.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT || lit.Value != "32" {
		return false
	}
	elt, ok := t.Elt.(*ast.Ident)
	return ok && elt.Name == "byte"
}

// readAllowlist reads one bare identifier per line from path. Blank lines
// and lines starting with # are ignored.
func readAllowlist(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool)
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed[line] = true
	}
	return allowed, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-bots-isolation: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
