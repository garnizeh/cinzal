package main

import (
	"reflect"
	"testing"
)

func TestParseSweepFlagsValid(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2,3", "Rounds=10,15"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	if len(dims) != 2 {
		t.Fatalf("len(dims) = %d, want 2", len(dims))
	}
	if dims[0].field != "LeaseCostPerBlock" || !reflect.DeepEqual(dims[0].values, []any{1, 2, 3}) {
		t.Errorf("dims[0] = %+v, want field LeaseCostPerBlock values [1 2 3]", dims[0])
	}
	if dims[1].field != "Rounds" || !reflect.DeepEqual(dims[1].values, []any{10, 15}) {
		t.Errorf("dims[1] = %+v, want field Rounds values [10 15]", dims[1])
	}
}

func TestParseSweepFlagsErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
	}{
		{"empty sweep list", nil},
		{"unknown field", []string{"NotAField=1,2"}},
		{"composite field", []string{"StepsByTier=1,2"}},
		{"composite map field", []string{"PostCapByPlayers=1,2"}},
		{"no equals sign", []string{"LeaseCostPerBlock"}},
		{"empty field name", []string{"=1,2"}},
		{"no values", []string{"LeaseCostPerBlock="}},
		{"unparsable value", []string{"LeaseCostPerBlock=notanumber"}},
		{"duplicate field across flags", []string{"Rounds=10,15", "Rounds=20,25"}},
		{"duplicate value within one dimension", []string{"LeaseCostPerBlock=2,2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSweepFlags(tt.raw); err == nil {
				t.Errorf("parseSweepFlags(%v) = nil error, want an error", tt.raw)
			}
		})
	}
}

// TestExpandSweepOrder is issue #200's own row-order requirement: two runs
// of the same invocation must produce byte-identical output, which starts
// with the cartesian product itself being deterministic and in dial order
// — dims[0]'s values vary slowest.
func TestExpandSweepOrder(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2", "Rounds=10,15,20"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}

	points := expandSweep(dims)
	if len(points) != 6 {
		t.Fatalf("len(points) = %d, want 6", len(points))
	}

	want := [][2]any{
		{1, 10}, {1, 15}, {1, 20},
		{2, 10}, {2, 15}, {2, 20},
	}
	for i, p := range points {
		if p.values[0] != want[i][0] || p.values[1] != want[i][1] {
			t.Errorf("points[%d].values = %v, want %v", i, p.values, want[i])
		}
		if p.cfg.LeaseCostPerBlock != want[i][0] {
			t.Errorf("points[%d].cfg.LeaseCostPerBlock = %d, want %v", i, p.cfg.LeaseCostPerBlock, want[i][0])
		}
		if p.cfg.Rounds != want[i][1] {
			t.Errorf("points[%d].cfg.Rounds = %d, want %v", i, p.cfg.Rounds, want[i][1])
		}
	}
}

// TestExpandSweepDoesNotAliasBaseConfig guards against a reflect.Set on
// one point's cfg silently mutating another's, on two levels: the swept
// scalar field itself, and — since a Go struct copy only copies a map
// field's header, not its underlying data — game.DefaultConfig()'s own
// map fields (PostCapByPlayers, MapByPlayers), which expandSweep must
// give each point its own copy of by constructing every point from a
// fresh game.DefaultConfig() call rather than copying a previous point's
// Config forward.
func TestExpandSweepDoesNotAliasBaseConfig(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2,3"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	points := expandSweep(dims)
	for i, p := range points {
		want := i + 1
		if p.cfg.LeaseCostPerBlock != want {
			t.Errorf("points[%d].cfg.LeaseCostPerBlock = %d, want %d — points are aliasing one Config", i, p.cfg.LeaseCostPerBlock, want)
		}
	}

	for i := 1; i < len(points); i++ {
		if reflect.ValueOf(points[0].cfg.PostCapByPlayers).Pointer() == reflect.ValueOf(points[i].cfg.PostCapByPlayers).Pointer() {
			t.Errorf("points[0] and points[%d] share the same PostCapByPlayers map instance", i)
		}
		if reflect.ValueOf(points[0].cfg.MapByPlayers).Pointer() == reflect.ValueOf(points[i].cfg.MapByPlayers).Pointer() {
			t.Errorf("points[0] and points[%d] share the same MapByPlayers map instance", i)
		}
	}
}

func TestSweepSpecString(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2,3", "Rounds=10,15"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	got := sweepSpecString(dims)
	want := "LeaseCostPerBlock:1,2,3;Rounds:10,15"
	if got != want {
		t.Errorf("sweepSpecString() = %q, want %q", got, want)
	}
}

func TestConfigKeyDistinguishesPoints(t *testing.T) {
	dims, err := parseSweepFlags([]string{"LeaseCostPerBlock=1,2"})
	if err != nil {
		t.Fatalf("parseSweepFlags() = %v", err)
	}
	points := expandSweep(dims)
	if configKey(points[0]) == configKey(points[1]) {
		t.Errorf("configKey collided for distinct points: %v vs %v", points[0].values, points[1].values)
	}
}
