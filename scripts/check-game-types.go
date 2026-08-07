//go:build ignore

// Command check-game-types asserts that internal/game declares no dynamic
// container: no `any`, no interface type of any shape, and no type parameter.
//
// WHY THIS EXISTS AS A PARSER RATHER THAN A GREP.
//
// The import gate in check-fog-boundary.sh proves that internal/render and
// internal/web cannot NAME the match state. It does not stop a state VALUE
// travelling inside a type they CAN name, so D01 forbids dynamic containers in
// internal/game — which imports nothing, making that channel closable outright.
//
// A textual scan cannot do this precisely. `any` is an ordinary English word and
// `interface` an ordinary technical one, so a grep flags them inside string
// literals and comments: `const msg = "any of these stances"` would fail a gate
// it does not violate. Biasing toward false positives is right when the cost is
// a rename of something odd; it is wrong when they land on ordinary prose in the
// package that holds the game's vocabulary.
//
// WHY ONLY TYPE EXPRESSIONS ARE WALKED.
//
// `any` is a predeclared alias rather than a keyword, so it is a legal name for
// a field, parameter or variable — and both the declaration and every later use
// of such a name must be left alone. `type Event struct { any string }` declares
// no dynamic container at all.
//
// Rather than resolve identifiers, this walks only the places a type can be
// written and descends the type expressions found there. That makes the
// distinction structural: a name is never in a type position, so it is never
// reached. Comments and string literals are not type expressions either, so the
// grep's false positives cannot occur by construction.
//
// THE RULE IS STRONGER THAN D01'S WORDING, DELIBERATELY.
//
// D01 says "no any, no interface{}, no unconstrained type parameter". An
// interface WITH methods is the same smuggling channel — a rules type can
// implement it and be stored in a game field — so every interface type is
// rejected, not only the empty one, and type parameters are rejected whether
// constrained or not.
//
// THIS CHECK FAILS CLOSED. A parse error, an unreadable directory, or a package
// with no non-test files each exit non-zero rather than reporting success.
//
// Run: go run scripts/check-game-types.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const target = "internal/game"

type finding struct {
	pos  token.Position
	what string
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal("could not determine the working directory: %v", err)
	}
	dir := filepath.Join(root, target)

	entries, err := os.ReadDir(dir)
	if err != nil {
		fatal("could not read %s: %v", target, err)
	}

	fset := token.NewFileSet()
	var findings []finding
	inspected := 0

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fatal("could not parse %s: %v", filepath.Join(target, name), err)
		}
		inspected++

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.Field:
				walkType(fset, node.Type, &findings)
			case *ast.ValueSpec:
				walkType(fset, node.Type, &findings)
			case *ast.TypeAssertExpr:
				walkType(fset, node.Type, &findings)
			case *ast.CompositeLit:
				walkType(fset, node.Type, &findings)
			case *ast.TypeSpec:
				walkType(fset, node.Type, &findings)
				if node.TypeParams != nil {
					findings = append(findings, finding{fset.Position(node.TypeParams.Pos()), "a type parameter"})
				}
			case *ast.FuncDecl:
				if node.Type != nil && node.Type.TypeParams != nil {
					findings = append(findings, finding{fset.Position(node.Type.TypeParams.Pos()), "a type parameter"})
				}
			}
			return true
		})
	}

	if inspected == 0 {
		fatal("%s contains no non-test Go files - nothing was inspected", target)
	}

	if len(findings) > 0 {
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].pos.Filename != findings[j].pos.Filename {
				return findings[i].pos.Filename < findings[j].pos.Filename
			}
			return findings[i].pos.Offset < findings[j].pos.Offset
		})
		fmt.Fprintf(os.Stderr, "check-game-types: %s must declare no dynamic container.\n", target)
		fmt.Fprintf(os.Stderr, "                  D01, and the fog boundary it protects: a value of any type\n")
		fmt.Fprintf(os.Stderr, "                  can hide inside one, including the match state.\n")
		for _, f := range findings {
			rel, err := filepath.Rel(root, f.pos.Filename)
			if err != nil {
				rel = f.pos.Filename
			}
			fmt.Fprintf(os.Stderr, "  %s:%d:%d: %s\n", rel, f.pos.Line, f.pos.Column, f.what)
		}
		os.Exit(1)
	}

	fmt.Printf("check-game-types: OK - %d file(s) in %s, no dynamic containers\n", inspected, target)
}

// walkType descends a type expression, reporting the any alias and every
// interface type it contains. It is called only on nodes that hold a type, so a
// field or variable NAMED `any` is never reached: a name is not a type
// expression.
func walkType(fset *token.FileSet, expr ast.Expr, findings *[]finding) {
	if expr == nil {
		return
	}
	switch t := expr.(type) {
	case *ast.Ident:
		if t.Name == "any" {
			*findings = append(*findings, finding{fset.Position(t.Pos()), "the any alias"})
		}
	case *ast.InterfaceType:
		*findings = append(*findings, finding{fset.Position(t.Pos()), "an interface type"})
	case *ast.ParenExpr:
		walkType(fset, t.X, findings)
	case *ast.StarExpr:
		walkType(fset, t.X, findings)
	case *ast.Ellipsis:
		walkType(fset, t.Elt, findings)
	case *ast.ArrayType:
		walkType(fset, t.Elt, findings)
	case *ast.MapType:
		walkType(fset, t.Key, findings)
		walkType(fset, t.Value, findings)
	case *ast.ChanType:
		walkType(fset, t.Value, findings)
	case *ast.IndexExpr: // generic instantiation with one argument
		walkType(fset, t.Index, findings)
	case *ast.IndexListExpr: // generic instantiation with several
		for _, idx := range t.Indices {
			walkType(fset, idx, findings)
		}
	case *ast.StructType:
		if t.Fields != nil {
			for _, f := range t.Fields.List {
				walkType(fset, f.Type, findings)
			}
		}
	case *ast.FuncType:
		for _, list := range []*ast.FieldList{t.Params, t.Results} {
			if list == nil {
				continue
			}
			for _, f := range list.List {
				walkType(fset, f.Type, findings)
			}
		}
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-game-types: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
