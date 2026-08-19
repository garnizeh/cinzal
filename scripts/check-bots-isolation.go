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
// — regardless of whether the file imports internal/rules at all. Because
// it is independent, it does not lean on the allow-list to catch what it
// misses: a length given as a package-level constant (const seedLen = 32)
// or a constant expression (16+16) is still exactly [32]byte, so this walk
// resolves those too, evaluating internal/bots's own integer constants
// (evalConstInt) rather than treating only a bare integer literal as
// readable. What stays out of reach is a length borrowed from ANOTHER
// package's constant (otherpkg.SeedLen) — resolving that needs the other
// package built and type-checked, which is what this whole family of gate
// (see check-game-types.go's own header) avoids by walking syntax instead
// of go/types. A rules-qualified one is not a gap: rules.SeedLen would
// already fail the selector check above for not being on the allow-list,
// whatever position it appeared in.
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
	files := make([]*ast.File, 0, len(prodFiles))
	for _, name := range prodFiles {
		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fatal("could not parse %s: %v", filepath.Join(target, name), err)
		}
		files = append(files, file)
	}
	filesScanned := len(files)

	// Collected across every production file before any file is walked for
	// findings: a length constant can be declared in one file and used in
	// another (const blocks are package-scoped, not file-scoped), and Go
	// itself allows a constant to be referenced before its own declaration
	// appears in source order.
	consts := collectIntConsts(files)

	var findings []finding
	for _, file := range files {
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
				if isSeedShaped(node, consts) {
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

// isSeedShaped reports whether t is a fixed-length array of 32 bytes — a
// length that EVALUATES to 32 (a literal in any base, a package-level
// constant, or an arithmetic expression over either — see evalConstInt),
// and byte's own predeclared alias uint8 counts as byte. Not a slice
// (Len == nil) and not any other length or element type.
func isSeedShaped(t *ast.ArrayType, consts map[string]constDef) bool {
	if t.Len == nil {
		return false
	}
	n, ok := evalConstInt(t.Len, consts, 0, nil)
	if !ok || n != 32 {
		return false
	}
	elt, ok := t.Elt.(*ast.Ident)
	return ok && (elt.Name == "byte" || elt.Name == "uint8")
}

// constDef is a package-level constant's defining expression, plus the
// iota value in force at its declaration (0 outside a const block, or when
// the spec does not use iota).
type constDef struct {
	expr ast.Expr
	iota int64
}

// collectIntConsts gathers every package-level `const Name = expr` (and
// `const ( A = 1; B; C = 2 )`-style blocks, where an omitted Values list
// repeats the previous spec's — the standard iota idiom) across files, by
// name. It does not evaluate anything yet: evalConstInt does that lazily,
// which is what lets a constant reference another declared earlier or
// later in source order, in the same file or a different one — exactly how
// Go's own package-level constants are scoped.
func collectIntConsts(files []*ast.File) map[string]constDef {
	consts := make(map[string]constDef)
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			var lastValues []ast.Expr
			for iotaIdx, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				values := vs.Values
				if len(values) == 0 {
					values = lastValues
				} else {
					lastValues = values
				}
				for i, name := range vs.Names {
					if name.Name == "_" || i >= len(values) {
						continue
					}
					consts[name.Name] = constDef{expr: values[i], iota: int64(iotaIdx)}
				}
			}
		}
	}
	return consts
}

// evalConstInt evaluates expr as a constant integer, over the subset of Go
// that a real length expression plausibly uses: integer literals in any
// base, named package-level constants (recursively, guarded against a
// cycle by seen), iota, parens, and the arithmetic/bitwise binary and unary
// operators. Anything else — a call, a float or string constant, a
// constant from another package — reports ok=false: not a value this
// function can vouch for, not an assertion that it is safe.
func evalConstInt(expr ast.Expr, consts map[string]constDef, iota int64, seen map[string]bool) (n int64, ok bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		v, err := strconv.ParseInt(e.Value, 0, 64) // base 0: honours 0x, 0o/0, 0b, and _ separators
		if err != nil {
			return 0, false
		}
		return v, true

	case *ast.Ident:
		if e.Name == "iota" {
			return iota, true
		}
		if seen[e.Name] {
			return 0, false // a const referring back to itself, directly or through others
		}
		def, found := consts[e.Name]
		if !found {
			return 0, false
		}
		nextSeen := make(map[string]bool, len(seen)+1)
		for k := range seen {
			nextSeen[k] = true
		}
		nextSeen[e.Name] = true
		return evalConstInt(def.expr, consts, def.iota, nextSeen)

	case *ast.ParenExpr:
		return evalConstInt(e.X, consts, iota, seen)

	case *ast.UnaryExpr:
		x, ok := evalConstInt(e.X, consts, iota, seen)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.SUB:
			return -x, true
		case token.ADD:
			return x, true
		case token.XOR:
			return ^x, true
		default:
			return 0, false
		}

	case *ast.BinaryExpr:
		x, ok := evalConstInt(e.X, consts, iota, seen)
		if !ok {
			return 0, false
		}
		y, ok := evalConstInt(e.Y, consts, iota, seen)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			return x + y, true
		case token.SUB:
			return x - y, true
		case token.MUL:
			return x * y, true
		case token.QUO:
			if y == 0 {
				return 0, false
			}
			return x / y, true
		case token.REM:
			if y == 0 {
				return 0, false
			}
			return x % y, true
		case token.SHL:
			if y < 0 {
				return 0, false
			}
			return x << uint64(y), true
		case token.SHR:
			if y < 0 {
				return 0, false
			}
			return x >> uint64(y), true
		case token.AND:
			return x & y, true
		case token.OR:
			return x | y, true
		case token.XOR:
			return x ^ y, true
		case token.AND_NOT:
			return x &^ y, true
		default:
			return 0, false
		}

	default:
		return 0, false
	}
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
