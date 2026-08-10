package gen

import (
	"strings"
	"testing"
)

func TestExhaustedErrorMessage(t *testing.T) {
	err := &ExhaustedError{
		Attempts:   50,
		MostFailed: constraintStartPlacement,
		Failures:   map[string]int{constraintStartPlacement: 47, constraintDegree: 3},
	}

	msg := err.Error()
	for _, want := range []string{"50", constraintStartPlacement, "47"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ExhaustedError.Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestParamsValidate(t *testing.T) {
	base := Params{Nodes: 15, MinEdges: 21, MaxEdges: 23, Players: 2, MaxAttempts: 100}

	if err := base.validate(); err != nil {
		t.Fatalf("validate() on a well-formed Params = %v, want nil", err)
	}

	cases := map[string]Params{
		"too few nodes":        withNodes(base, minSupportedNodes-1),
		"min edges zero":       withEdges(base, 0, 5),
		"max less than min":    withEdges(base, 10, 5),
		"zero players":         withPlayers(base, 0),
		"negative players":     withPlayers(base, -1),
		"zero max attempts":    withMaxAttempts(base, 0),
		"negative max attempt": withMaxAttempts(base, -1),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if err := p.validate(); err == nil {
				t.Errorf("validate() = nil, want an error")
			}
		})
	}

	// The boundary itself must be accepted, not just values strictly above it.
	if err := withNodes(base, minSupportedNodes).validate(); err != nil {
		t.Errorf("validate() at exactly minSupportedNodes (%d) = %v, want nil", minSupportedNodes, err)
	}
}
