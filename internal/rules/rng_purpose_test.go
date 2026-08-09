package rules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// declaredPurposeConstants parses rng_purpose.go's own source and returns
// every string value assigned to a `Purpose`-typed constant. It reads the
// source rather than trusting a hand-maintained list, so a constant added
// without updating this test's expectations is still caught: the point of
// the acceptance criterion is that the constants and the table cannot drift
// apart silently.
func declaredPurposeConstants(t *testing.T) map[Purpose]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "rng_purpose.go", nil, 0)
	if err != nil {
		t.Fatalf("parse rng_purpose.go: %v", err)
	}

	declared := map[Purpose]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Purpose" {
				continue
			}
			for _, val := range vs.Values {
				lit, ok := val.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", lit.Value, err)
				}
				declared[Purpose(unquoted)] = true
			}
		}
	}
	return declared
}

// TestPurposeTableMatchesDeclaredConstants is the acceptance criterion from
// issue #56: "The purpose constants match #41's completed table one for
// one, with a test that fails if a constant exists without a table row."
// It fails closed — an empty parse result fails rather than vacuously
// passing, the same standard this repository's CI gates hold themselves to.
func TestPurposeTableMatchesDeclaredConstants(t *testing.T) {
	declared := declaredPurposeConstants(t)
	if len(declared) == 0 {
		t.Fatal("parsed zero Purpose constants from rng_purpose.go — the AST walk found nothing, which would make this test vacuously pass")
	}

	inTable := map[Purpose]bool{}
	for _, row := range ConsumptionTable {
		if inTable[row.Purpose] {
			t.Errorf("ConsumptionTable has a duplicate row for %q", row.Purpose)
		}
		inTable[row.Purpose] = true
	}
	if len(inTable) == 0 {
		t.Fatal("ConsumptionTable is empty — nothing was checked")
	}

	for p := range declared {
		if !inTable[p] {
			t.Errorf("Purpose constant %q is declared but has no ConsumptionTable row", p)
		}
	}
	for p := range inTable {
		if !declared[p] {
			t.Errorf("ConsumptionTable has a row for %q, which no declared Purpose constant names", p)
		}
	}
}
