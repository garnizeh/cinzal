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
// a rename of something odd; it is wrong when the false positives land on
// ordinary prose in the package that holds the game's vocabulary.
//
// Walking the syntax tree removes the question entirely: literals and comments
// are not type expressions, so they are never examined.
//
// The rule enforced is STRONGER than D01's wording, deliberately. D01 says "no
// any, no interface{}, no unconstrained type parameter". An interface WITH
// methods is the same smuggling channel — a rules type can implement it and be
// stored in a game field — so every interface type is rejected, not only the
// empty one.
//
// THIS CHECK FAILS CLOSED. A parse error, an unreadable directory, or a package
// with no files each exit non-zero rather than reporting success.
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

		// Comments are parsed but never walked: only type expressions are
		// examined, so a comment or a string literal cannot produce a finding.
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fatal("could not parse %s: %v", filepath.Join(target, name), err)
		}
		inspected++

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.InterfaceType:
				findings = append(findings, finding{fset.Position(node.Pos()), "an interface type"})
			case *ast.Ident:
				if node.Name == "any" {
					findings = append(findings, finding{fset.Position(node.Pos()), "the any alias"})
				}
			case *ast.TypeSpec:
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

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-game-types: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
