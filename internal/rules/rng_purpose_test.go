package rules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// declaredPurposeConstant is one `Purpose`-typed constant declaration: the
// identifier and the string value assigned to it, kept as a pair rather
// than collapsed into a set. Two differently-named constants that happen to
// share a string value are a copy-paste bug, not a duplicate to be merged
// away, and this shape is what lets the test tell the two apart.
type declaredPurposeConstant struct {
	name  string
	value Purpose
}

// declaredPurposeConstants parses every non-test .go file in this package
// directory and returns every `Purpose`-typed constant declaration it
// finds. It reads the source rather than trusting a hand-maintained list,
// so a constant added anywhere in the package — not just in this file — is
// still caught: the point of the acceptance criterion is that the
// constants and the table cannot drift apart silently.
//
// A ValueSpec that omits its type inherits the previous spec's type within
// the same const block, per the Go spec's iota-repetition rule; this walk
// carries that type forward so such a constant is not silently skipped.
func declaredPurposeConstants(t *testing.T) []declaredPurposeConstant {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/rules directory: %v", err)
	}

	fset := token.NewFileSet()
	var declared []declaredPurposeConstant
	filesScanned := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		filesScanned++

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}

			lastType := ""
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if ident, ok := vs.Type.(*ast.Ident); ok {
					lastType = ident.Name
				}
				if lastType != "Purpose" {
					continue
				}
				for i, val := range vs.Values {
					lit, ok := val.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					unquoted, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", lit.Value, err)
					}
					constName := "?"
					if i < len(vs.Names) {
						constName = vs.Names[i].Name
					}
					declared = append(declared, declaredPurposeConstant{name: constName, value: Purpose(unquoted)})
				}
			}
		}
	}

	if filesScanned == 0 {
		t.Fatal("scanned zero .go files in internal/rules — the directory walk found nothing, which would make this test vacuously pass")
	}

	return declared
}

// TestPurposeTableMatchesDeclaredConstants is the acceptance criterion from
// issue #56: "The purpose constants match #41's completed table one for
// one, with a test that fails if a constant exists without a table row."
// It fails closed — an empty parse result fails rather than vacuously
// passing, the same standard this repository's CI gates hold themselves to.
func TestPurposeTableMatchesDeclaredConstants(t *testing.T) {
	constants := declaredPurposeConstants(t)
	if len(constants) == 0 {
		t.Fatal("parsed zero Purpose constants from internal/rules — the AST walk found nothing, which would make this test vacuously pass")
	}

	// Every declared constant must have its own unique value. Two different
	// names sharing one string value would collapse to a single map entry
	// below and let a copy-paste bug pass silently, so it is checked before
	// anything is deduplicated.
	valueOwner := map[Purpose]string{}
	declaredNames := map[string]bool{}
	for _, c := range constants {
		if declaredNames[c.name] {
			t.Errorf("Purpose constant %s is declared more than once", c.name)
		}
		declaredNames[c.name] = true

		if owner, ok := valueOwner[c.value]; ok && owner != c.name {
			t.Errorf("Purpose value %q is used by both %s and %s — each declared constant needs its own unique value", c.value, owner, c.name)
			continue
		}
		valueOwner[c.value] = c.name
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

	for _, c := range constants {
		if !inTable[c.value] {
			t.Errorf("Purpose constant %s (%q) is declared but has no ConsumptionTable row", c.name, c.value)
		}
	}
	for p := range inTable {
		if _, ok := valueOwner[p]; !ok {
			t.Errorf("ConsumptionTable has a row for %q, which no declared Purpose constant names", p)
		}
	}
}
