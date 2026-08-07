//go:build ignore

// Command check-debug-tags asserts that every source file under internal/debug,
// except the exempt doc.go, carries a build constraint that cannot be satisfied
// without the `debug` tag.
//
// WHY THIS IS NOT THE SAME AS THE go list CHECK IT SITS BESIDE.
//
// check-debug-isolation.sh asserts that only doc.go compiles into
// internal/debug. That check is exact — and it is exact only for the ACTIVE
// GOOS, GOARCH and tag set. `go list` reports .GoFiles for one build
// configuration, so a file constrained `//go:build windows` is simply omitted on
// a Linux runner: invisible to that check, and compiled into a Windows
// production binary. The gate would pass while debug code shipped.
//
// So the two assertions are kept, and they are different: one asks what compiles
// here, the other asks what could compile anywhere.
//
// THE TEST, AND WHY IT IS THE RIGHT ONE.
//
// Evaluate the constraint with `debug` FALSE and every other tag TRUE. If it can
// still be satisfied, the file can reach a build that does not ask for debug.
//
//	//go:build debug              debug=false -> false   accepted
//	//go:build debug && windows   debug=false -> false   accepted
//	//go:build windows            windows=true -> true   REJECTED
//	//go:build debug || windows   windows=true -> true   REJECTED
//
// The last row is the one a reviewer would have to think about and a parser does
// not: an `||` makes the debug tag optional, which is the whole failure dressed
// as a constraint.
//
// RFC-001 §15.1 is the reason any of this matters: a god view reaching
// production is unrecoverable, because you cannot un-leak a map.
//
// THIS CHECK FAILS CLOSED. An unreadable file, an unparseable constraint, a
// missing constraint, or a walk that finds nothing each exit non-zero.
//
// Run: go run scripts/check-debug-tags.go
package main

import (
	"bufio"
	"fmt"
	"go/build/constraint"
	"os"
	"path/filepath"
	"strings"
)

const (
	target     = "internal/debug"
	exemptFile = "doc.go"
	debugTag   = "debug"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal("could not determine the working directory: %v", err)
	}
	dir := filepath.Join(root, target)
	exempt := filepath.Join(dir, exemptFile)

	var problems []string
	inspected := 0

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || path == exempt {
			return nil
		}
		inspected++

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		expr, found, readErr := buildConstraint(path)
		if readErr != nil {
			return fmt.Errorf("could not read %s: %w", rel, readErr)
		}
		if !found {
			problems = append(problems, fmt.Sprintf("%s: no //go:build constraint", rel))
			return nil
		}
		// debug off, everything else on: can this file still be built?
		if expr.Eval(func(tag string) bool { return tag != debugTag }) {
			problems = append(problems, fmt.Sprintf("%s: buildable without the %q tag", rel, debugTag))
		}
		return nil
	})
	if err != nil {
		fatal("%v", err)
	}

	if inspected == 0 {
		fmt.Printf("check-debug-tags: OK - %s holds only %s\n", target, exemptFile)
		return
	}

	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "check-debug-tags: debug code can reach a build that did not ask for it.\n")
		fmt.Fprintf(os.Stderr, "                  RFC-001 §15.1 - a god view in production is unrecoverable.\n")
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "\n                  Every file here needs a constraint that is false when\n")
		fmt.Fprintf(os.Stderr, "                  debug is off. Note that `debug || something` is not one.\n")
		os.Exit(1)
	}

	fmt.Printf("check-debug-tags: OK - %d file(s) under %s, none buildable without %q\n", inspected, target, debugTag)
}

// buildConstraint returns the //go:build expression of a file. Only the lines
// before the package clause are considered, which is where Go looks for one.
func buildConstraint(path string) (constraint.Expr, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "package ") {
			break
		}
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return nil, false, fmt.Errorf("unparseable build constraint %q: %w", line, err)
		}
		return expr, true, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return nil, false, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-debug-tags: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
