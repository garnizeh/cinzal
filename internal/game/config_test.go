package game

import "testing"

func TestDefaultConfigValidatesForEverySupportedPlayerCount(t *testing.T) {
	cfg := DefaultConfig()

	for _, players := range []int{2, 3, 4, 5} {
		if err := cfg.Validate(players); err != nil {
			t.Errorf("DefaultConfig().Validate(%d) = %v, want nil", players, err)
		}
	}
}

func TestValidateRejectsUnsupportedPlayerCount(t *testing.T) {
	cfg := DefaultConfig()

	for _, players := range []int{0, 1, 6} {
		if err := cfg.Validate(players); err == nil {
			t.Errorf("Validate(%d) = nil, want an error (no PostCapByPlayers/MapByPlayers entry)", players)
		}
	}
}

// TestValidateRejects20Rounds pins the exact case GDD §16.2 works through:
// a 20-round table cannot be dealt from either deck's drawn pool.
func TestValidateRejects20Rounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rounds = 20

	if err := cfg.Validate(4); err == nil {
		t.Fatal("Validate() = nil for Rounds=20, want an error — GDD §16.2's 18-incident-rounds-against-13-cards case")
	}
}

// TestValidateAcceptsExactly15Rounds guards the boundary: 15 is not merely
// under the limit, it is the limit — 12 event rounds against 12 drawn cards,
// 13 incident rounds against 13 drawn cards, both exact.
func TestValidateAcceptsExactly15Rounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rounds = 15

	if err := cfg.Validate(4); err != nil {
		t.Fatalf("Validate() = %v for Rounds=15, want nil (exact boundary, GDD §16.2)", err)
	}
}

// TestValidateRejectsRoundsOneOverBoundary confirms the boundary is real —
// 16 rounds needs 13 event rounds against a 12-card pool, one more than the
// deck can cover.
func TestValidateRejectsRoundsOneOverBoundary(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rounds = 16

	if err := cfg.Validate(4); err == nil {
		t.Fatal("Validate() = nil for Rounds=16, want an error — one round past the deck's coverage")
	}
}

func TestValidateRejectsZeroRounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rounds = 0

	if err := cfg.Validate(4); err == nil {
		t.Fatal("Validate() = nil for Rounds=0, want an error")
	}
}

// TestValidateFailsClosedOnEmptyConfig guards the fail-closed convention
// this repository holds every gate to: a zero-value Config — the shape a
// missing or malformed load would produce — must be rejected, not treated
// as a permissive default.
func TestValidateFailsClosedOnEmptyConfig(t *testing.T) {
	var cfg Config

	if err := cfg.Validate(4); err == nil {
		t.Fatal("Validate() = nil for a zero-value Config, want an error")
	}
}

// TestDefaultConfigMatchesGDDContractTable pins GDD §8.3's numbers so a
// balancing edit to this table is a deliberate, reviewed change to this
// test, not a silent drift.
func TestDefaultConfigMatchesGDDContractTable(t *testing.T) {
	want := [4]ContractTier{
		{InfamyRequired: 0, MinDistance: 3, MaxDistance: 4, Payment: 8, RP: 2, Deadline: 4, Penalty: 3, PenaltyInfamy: 0},
		{InfamyRequired: 3, MinDistance: 4, MaxDistance: 6, Payment: 14, RP: 3, Deadline: 5, Penalty: 5, PenaltyInfamy: 0},
		{InfamyRequired: 6, MinDistance: 5, MaxDistance: 8, Payment: 20, RP: 5, Deadline: 5, Penalty: 8, PenaltyInfamy: 0},
		{InfamyRequired: 9, MinDistance: 6, MaxDistance: 0, Payment: 30, RP: 8, Deadline: 6, Penalty: 12, PenaltyInfamy: 2},
	}

	got := DefaultConfig().Contracts
	if got != want {
		t.Fatalf("DefaultConfig().Contracts = %+v, want %+v", got, want)
	}
}

// TestDefaultConfigMatchesGDDMapTable pins GDD §6.1's node/edge table.
func TestDefaultConfigMatchesGDDMapTable(t *testing.T) {
	want := map[int]MapSpec{
		2: {Nodes: 15, MinEdges: 21, MaxEdges: 23},
		3: {Nodes: 22, MinEdges: 31, MaxEdges: 35},
		4: {Nodes: 25, MinEdges: 36, MaxEdges: 40},
		5: {Nodes: 28, MinEdges: 40, MaxEdges: 45},
	}

	got := DefaultConfig().MapByPlayers
	if len(got) != len(want) {
		t.Fatalf("DefaultConfig().MapByPlayers has %d entries, want %d", len(got), len(want))
	}
	for players, spec := range want {
		if got[players] != spec {
			t.Errorf("MapByPlayers[%d] = %+v, want %+v", players, got[players], spec)
		}
	}
}

// TestValidateRejectsZeroMaxGenAttempts guards the bounded-retry, fail-loudly
// discipline D9's decision log requires of rules/gen (#59): a Config that
// cannot bound the generator's retry loop must not validate.
func TestValidateRejectsZeroMaxGenAttempts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxGenAttempts = 0

	if err := cfg.Validate(4); err == nil {
		t.Fatal("Validate() = nil for MaxGenAttempts=0, want an error")
	}
}

// TestSuppressZeroValueSuppressesNothing guards D11's decision that an
// ordinary match's Suppress field must be all-false by construction, not by
// convention at each call site.
func TestSuppressZeroValueSuppressesNothing(t *testing.T) {
	var s SubsystemSuppression

	if s.Leases || s.Incidents || s.Events || s.InfamyTiers || s.Items {
		t.Fatalf("zero-value SubsystemSuppression has a field set: %+v", s)
	}
}
