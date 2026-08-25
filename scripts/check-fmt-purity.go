//go:build ignore

// Command check-fmt-purity asserts that a directory tree's production
// (non-_test.go) source holds no reference — direct or indirect — to one of
// fmt's I/O-performing selectors: Print, Printf, Println, Fprint, Fprintf,
// Fprintln, Fscan, Fscanf, Fscanln.
//
// WHY THIS CHECK EXISTS SEPARATELY FROM check-rules-purity.sh'S IMPORT LIST.
//
// An import rule can forbid a package outright, but it cannot separate
// formatting from writing within a single package: fmt.Sprintf and
// fmt.Fprintf both live in "fmt", and Sprintf is pure (it returns a string)
// while Fprintf is I/O (it writes to an io.Writer). internal/rules,
// internal/telemetry and internal/bots all legitimately need Errorf and
// Sprintf, so fmt cannot be forbidden at the import level — only the
// I/O-performing selectors on it can, and only at the call site.
// scripts/check-rules-purity.sh runs this program once per tree and folds
// its findings into its own aggregate report.
//
// WHY THIS IS AN AST WALK, NOT A GREP (issue #297).
//
// A textual `grep -E '\bfmt\.(Print|...)\('` pattern only matches a selector
// immediately followed by "(" — a direct call. It misses
// `p := fmt.Println; p("x")`, which performs the exact same write through a
// value that was never textually adjacent to a "(". This walk instead looks
// for the selector expression itself (fmt.Println), wherever it appears in
// the syntax tree — a call's callee, an assignment's right-hand side, a
// function argument, anywhere — so routing it through a local variable first
// buys nothing.
//
// A dot import (`import . "fmt"`) is still rejected outright rather than
// resolved: after a dot import, a bare `Println(...)` could be the
// dot-imported fmt.Println, or a local function of the same name shadowing
// it, and telling those apart needs type information this walk deliberately
// does not have — the same tradeoff check-bots-isolation.go's own header
// makes about rules identifiers, made here for the same reason. An ordinary
// or aliased import (`import f "fmt"`) is resolved correctly instead of
// being rejected outright, because — unlike a dot import — the walk always
// knows exactly which identifier the import bound; that is a direct
// consequence of walking syntax instead of matching text, not a separate
// feature.
//
// CONTRACT WITH THE CALLER.
//
// Unlike check-bots-isolation.go, this program is never the whole gate by
// itself — scripts/check-rules-purity.sh calls it once per tree and needs to
// keep going before failing once, after every tree (and its own import-list
// check) has been inspected. So: a clean tree prints nothing and exits 0. A
// tree with findings prints one "path:line:col: message" line per finding to
// stdout and exits 1 — the caller folds that output into its own report. An
// infra failure this program cannot recover from (bad args, an unreadable
// directory, a parse error, or a tree holding no inspectable production Go
// file) prints "check-fmt-purity: FAIL: ..." to stderr and exits 1 too; the
// caller tells the two apart by that literal prefix, not by exit code, since
// go run's own exit code on a build failure would otherwise collide with the
// "findings" case.
//
// Run: go run scripts/check-fmt-purity.go <target-dir>
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var forbiddenSelectors = map[string]bool{
	"Print": true, "Printf": true, "Println": true,
	"Fprint": true, "Fprintf": true, "Fprintln": true,
	"Fscan": true, "Fscanf": true, "Fscanln": true,
}

type finding struct {
	pos  token.Position
	what string
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "" {
		fatal("usage: check-fmt-purity <target-dir>")
	}
	target := os.Args[1]

	root, err := os.Getwd()
	if err != nil {
		fatal("could not determine the working directory: %v", err)
	}
	dir := target
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, target)
	}

	var goFiles []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		goFiles = append(goFiles, path)
		return nil
	})
	if err != nil {
		fatal("could not walk %s: %v", target, err)
	}
	if len(goFiles) == 0 {
		fatal("%s holds no inspectable production .go file — nothing was inspected, which is not a pass", target)
	}
	sort.Strings(goFiles)

	fset := token.NewFileSet()
	var findings []finding
	for _, path := range goFiles {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fatal("could not parse %s: %v", path, err)
		}

		binding, dotPos := fmtBinding(file)
		if dotPos != token.NoPos {
			findings = append(findings, finding{
				fset.Position(dotPos),
				"dot-imports \"fmt\", which binds every exported identifier into file scope; a bare Println here could be fmt's or a same-named local, and this check does not resolve that — use a plain or aliased import instead",
			})
		}
		if binding == "" {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok || x.Name != binding {
				return true
			}
			if forbiddenSelectors[sel.Sel.Name] {
				findings = append(findings, finding{
					fset.Position(sel.Pos()),
					fmt.Sprintf("fmt.%s performs I/O; fmt stays importable for Sprintf/Errorf but this selector writes to or reads from a Writer/Reader regardless of whether it is called directly or referenced first and called later", sel.Sel.Name),
				})
			}
			return true
		})
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].pos.Filename != findings[j].pos.Filename {
			return findings[i].pos.Filename < findings[j].pos.Filename
		}
		return findings[i].pos.Offset < findings[j].pos.Offset
	})

	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		rel, err := filepath.Rel(root, f.pos.Filename)
		if err != nil {
			rel = f.pos.Filename
		}
		fmt.Printf("%s:%d:%d: %s\n", rel, f.pos.Line, f.pos.Column, f.what)
	}
	os.Exit(1)
}

// fmtBinding reports the identifier "fmt" is bound to in file — its import
// alias, or the default package name "fmt" when unaliased — and dotPos, the
// position of the import spec itself when it is a dot import instead
// (token.NoPos otherwise). binding is empty when the file does not import
// fmt at all, or imports it blank.
func fmtBinding(file *ast.File) (binding string, dotPos token.Pos) {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "fmt" {
			continue
		}
		if imp.Name == nil {
			return "fmt", token.NoPos
		}
		switch imp.Name.Name {
		case "_":
			return "", token.NoPos
		case ".":
			return "", imp.Pos()
		default:
			return imp.Name.Name, token.NoPos
		}
	}
	return "", token.NoPos
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-fmt-purity: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
