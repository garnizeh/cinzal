package store

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file unit-tests everything issue #318 added that never touches a
// database: the Config codec's version dispatch and recursive key-set
// check (config_codec.go), the seed length check, and seatsFromRows' seat-
// contiguity check. CreateMatch/LoadMatch's own real-Postgres behavior —
// the one transaction, the rollback-on-failure, the round trip against a
// live matches/match_players row — lives in matches_integration_test.go.

// mutatedConfig returns a Config that differs from game.DefaultConfig() in
// several fields and is not the zero value — issue #318's own "fails
// closed" acceptance criterion: a round-trip test run only against
// DefaultConfig() (or, worse, the zero value) cannot distinguish a real
// codec from one that just returns a hard-coded default for everything.
func mutatedConfig() game.Config {
	cfg := game.DefaultConfig()
	cfg.Rounds = 12
	cfg.LeaseCostPerBlock = 99
	cfg.StepsByTier = [4]int{9, 8, 7, 6}
	cfg.Suppress.Incidents = true
	cfg.Contracts[0].Payment = 12345
	return cfg
}

// TestConfigCodecRoundTripsExactly is D44 §3's round-trip property for
// Config: decode(encode(x)) == x. reflect.DeepEqual is D44's own stated
// sufficient check ("Config needs one, or reflect.DeepEqual is sufficient
// since it holds no pointers").
func TestConfigCodecRoundTripsExactly(t *testing.T) {
	want := mutatedConfig()

	zero := game.Config{}
	if reflect.DeepEqual(want, zero) {
		t.Fatal("fails closed: mutatedConfig() must not be the zero value")
	}
	if reflect.DeepEqual(want, game.DefaultConfig()) {
		t.Fatal("fails closed: mutatedConfig() must differ from DefaultConfig() in at least one field")
	}

	encoded, err := EncodeConfig(want)
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	got, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(%s): %v", encoded, err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round trip not equal.\n want: %+v\n got:  %+v\n wire: %s", want, got, encoded)
	}
}

// TestDecodeConfigRejectsUnrecognizedVersion is D44's "an unrecognized
// version number is a hard decode error with no fallback."
func TestDecodeConfigRejectsUnrecognizedVersion(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	env["v"] = json.RawMessage("2")
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with v=2 returned nil error, want a rejection (no version 2 exists)")
	}
}

// TestDecodeConfigRejectsFieldMissingInsideNestedObject is D44 §3's
// explicitly required fixture: "a Config fixture missing one field *inside*
// a nested object (e.g. Contracts[0] without payment) must be asserted to
// fail decode, exercising the recursive check rather than only the outer
// one." This is the fixture that distinguishes checkExactKeys' recursive
// pass from a check that only inspects the top-level object.
func TestDecodeConfigRejectsFieldMissingInsideNestedObject(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var contracts []map[string]json.RawMessage
	if err := json.Unmarshal(top["contracts"], &contracts); err != nil {
		t.Fatalf("unmarshal contracts: %v", err)
	}

	// Delete "payment" from Contracts[0] only — every other key, at every
	// other level, is left intact and structurally valid.
	delete(contracts[0], "payment")

	contractsRaw, err := json.Marshal(contracts)
	if err != nil {
		t.Fatalf("marshal contracts: %v", err)
	}
	top["contracts"] = contractsRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with contracts[0].payment missing returned nil error, want a rejection (D44's recursive key-set check)")
	}
}

// TestDecodeConfigRejectsUnknownKeyInsideNestedObject is the mirror-image
// fixture: an extra key one level down, which only a recursive check (not
// a top-level-only DisallowUnknownFields) can be relied on to catch before
// it silently loses information on the next re-encode.
func TestDecodeConfigRejectsUnknownKeyInsideNestedObject(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var pressure map[string]json.RawMessage
	if err := json.Unmarshal(top["pressure"], &pressure); err != nil {
		t.Fatalf("unmarshal pressure: %v", err)
	}
	pressure["surprise"] = json.RawMessage("true")
	pressureRaw, err := json.Marshal(pressure)
	if err != nil {
		t.Fatalf("marshal pressure: %v", err)
	}
	top["pressure"] = pressureRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with an unknown key inside pressure returned nil error, want a rejection")
	}
}

// TestDecodeConfigRejectsShortContractsArray is D48's (#345) first required
// fixture: "a Config payload whose contracts array has three elements —
// Tier IV entirely absent — asserted to fail decode." encoding/json would
// otherwise decode a 3-element JSON array straight into game.Config's
// [4]ContractTier with no error, silently leaving Tier IV as an all-zero
// ContractTier — the array-length check this exercises has to run before
// that decode, against the raw JSON array, per D48 step 2.
func TestDecodeConfigRejectsShortContractsArray(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var contracts []json.RawMessage
	if err := json.Unmarshal(top["contracts"], &contracts); err != nil {
		t.Fatalf("unmarshal contracts: %v", err)
	}
	contracts = contracts[:3] // drop Tier IV entirely
	contractsRaw, err := json.Marshal(contracts)
	if err != nil {
		t.Fatalf("marshal contracts: %v", err)
	}
	top["contracts"] = contractsRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with a 3-element contracts array returned nil error, want a rejection (D48's array-length check)")
	}
}

// TestDecodeConfigRejectsMapByPlayersMissingKey is D48's second required
// fixture: "a Config payload whose map_by_players omits the '5' key,
// asserted to fail decode." Every present value still passes its own
// per-value shape check (D44/D48 step 1), so only a check of the map's own
// key set — independent of iterating what's present — can catch this.
func TestDecodeConfigRejectsMapByPlayersMissingKey(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var mapByPlayers map[string]json.RawMessage
	if err := json.Unmarshal(top["map_by_players"], &mapByPlayers); err != nil {
		t.Fatalf("unmarshal map_by_players: %v", err)
	}
	delete(mapByPlayers, "5")
	mapByPlayersRaw, err := json.Marshal(mapByPlayers)
	if err != nil {
		t.Fatalf("marshal map_by_players: %v", err)
	}
	top["map_by_players"] = mapByPlayersRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with map_by_players missing the \"5\" key returned nil error, want a rejection (D48's map-key-set check)")
	}
}

// TestDecodeConfigRejectsShortStepsByTierArray covers D48's array-length
// check on StepsByTier specifically — Contracts is not the only [4]-shaped
// field D48 names, and CooldownByTier shares the identical mechanism, so
// this and the array-length check exercised above are the representative
// pair rather than one fixture per field.
func TestDecodeConfigRejectsShortStepsByTierArray(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var steps []json.RawMessage
	if err := json.Unmarshal(top["steps_by_tier"], &steps); err != nil {
		t.Fatalf("unmarshal steps_by_tier: %v", err)
	}
	steps = steps[:3]
	stepsRaw, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps_by_tier: %v", err)
	}
	top["steps_by_tier"] = stepsRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with a 3-element steps_by_tier array returned nil error, want a rejection (D48's array-length check)")
	}
}

// TestDecodeConfigRejectsPostCapByPlayersMissingKey covers D48's map-key-set
// check on PostCapByPlayers specifically — its values are plain ints with no
// per-value object shape to check, so the map's own key-set check is the
// *only* thing standing between a corrupted row and a silently short map.
func TestDecodeConfigRejectsPostCapByPlayersMissingKey(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	var postCap map[string]json.RawMessage
	if err := json.Unmarshal(top["post_cap_by_players"], &postCap); err != nil {
		t.Fatalf("unmarshal post_cap_by_players: %v", err)
	}
	delete(postCap, "2")
	postCapRaw, err := json.Marshal(postCap)
	if err != nil {
		t.Fatalf("marshal post_cap_by_players: %v", err)
	}
	top["post_cap_by_players"] = postCapRaw
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with post_cap_by_players missing the \"2\" key returned nil error, want a rejection (D48's map-key-set check)")
	}
}

// TestDecodeConfigRejectsTrailingData mirrors orderlog.fromRows' own guard
// (a CodeRabbit finding on PR #393, issue #317): a payload with garbage
// appended after a well-formed envelope must not silently decode as if the
// garbage weren't there.
func TestDecodeConfigRejectsTrailingData(t *testing.T) {
	encoded, err := EncodeConfig(mutatedConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}
	tampered := append(encoded, []byte(`{}`)...)

	if _, err := DecodeConfig(tampered); err == nil {
		t.Fatal("DecodeConfig with trailing data returned nil error, want a rejection")
	}
}

// TestDecodeConfigRejectsMalformedEnvelope covers the envelope shape itself
// — missing "config" key entirely, and an unrecognized top-level key —
// distinct from any check inside the nested config object.
func TestDecodeConfigRejectsMalformedEnvelope(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"missing config key", `{"v":1}`},
		{"unknown top-level key", `{"v":1,"config":{},"extra":true}`},
		{"not an object", `[1,2,3]`},
		{"empty", ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeConfig([]byte(tt.json)); err == nil {
				t.Fatalf("DecodeConfig(%q) returned nil error, want a rejection", tt.json)
			}
		})
	}
}

// TestDecodeSeedRejectsWrongLength is D44's "a 31-byte row must fail
// loudly on read, per D44 — silently zero-padding produces a valid-looking
// match whose every RNG draw is wrong," checked at both boundaries (short
// and long) plus the exact-32 success case.
func TestDecodeSeedRejectsWrongLength(t *testing.T) {
	for _, n := range []int{0, 1, 31, 33, 64} {
		if _, err := decodeSeed(make([]byte, n)); err == nil {
			t.Errorf("decodeSeed(%d bytes) returned nil error, want a rejection naming the length", n)
		}
	}

	seed, err := decodeSeed(make([]byte, 32))
	if err != nil {
		t.Fatalf("decodeSeed(32 bytes) = %v, want nil error", err)
	}
	if seed != ([32]byte{}) {
		t.Fatalf("decodeSeed(32 zero bytes) = %v, want the zero array", seed)
	}
}

// TestSeatsFromRowsRejectsGapOrDuplicate is #318's own acceptance
// criterion: "the players argument to rules.NewMatch and the match_players
// row count are checked to agree on reload; a disagreement is an error,
// not a fold." A gap or an out-of-order seat is exactly that disagreement,
// caught before cfg.Validate ever runs against a wrong player count.
func TestSeatsFromRowsRejectsGapOrDuplicate(t *testing.T) {
	tests := []struct {
		name string
		seat []game.SeatID
	}{
		{"gap: seats 0 and 2, no 1", []game.SeatID{0, 2}},
		{"duplicate: seat 0 twice", []game.SeatID{0, 0}},
		{"starts at 1, not 0", []game.SeatID{1, 2}},
		{"out of order", []game.SeatID{1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]ListMatchPlayersRow, len(tt.seat))
			for i, seat := range tt.seat {
				rows[i] = ListMatchPlayersRow{Seat: seat, Faction: "test"}
			}
			if _, err := seatsFromRows(rows); err == nil {
				t.Fatalf("seatsFromRows(%v) returned nil error, want a rejection", tt.seat)
			}
		})
	}
}

// TestSeatsFromRowsAcceptsContiguousRoster is the positive case: a dense
// 0..n-1 roster in order converts cleanly, and len(result) is exactly the
// players count cfg.Validate should be checked against.
func TestSeatsFromRowsAcceptsContiguousRoster(t *testing.T) {
	rows := []ListMatchPlayersRow{
		{Seat: 0, Faction: "a"},
		{Seat: 1, Faction: "b"},
		{Seat: 2, Faction: "c"},
	}
	seats, err := seatsFromRows(rows)
	if err != nil {
		t.Fatalf("seatsFromRows: %v", err)
	}
	if len(seats) != 3 {
		t.Fatalf("len(seats) = %d, want 3", len(seats))
	}
	if seats[1].Faction != "b" {
		t.Fatalf("seats[1].Faction = %q, want %q", seats[1].Faction, "b")
	}
}

// TestCreateMatchRejectsNoSeats and TestCreateMatchRejectsInvalidConfig
// both exercise CreateMatch's input validation against a zero-value *Store
// — neither ever needs to touch the database, since both checks run before
// any query does (matches.go's own doc comment on CreateMatch).
func TestCreateMatchRejectsNoSeats(t *testing.T) {
	var s Store
	_, _, err := s.CreateMatch(t.Context(), [32]byte{}, game.DefaultConfig(), nil, pgUUID(), nil, nil)
	if err == nil {
		t.Fatal("CreateMatch with zero seats returned nil error, want a rejection")
	}
}

func TestCreateMatchRejectsInvalidConfig(t *testing.T) {
	var s Store
	cfg := game.DefaultConfig()
	cfg.Rounds = 0 // Config.Validate rejects Rounds < 1
	seats := []SeatSpec{{Faction: "test"}, {Faction: "test"}}
	_, _, err := s.CreateMatch(t.Context(), [32]byte{}, cfg, seats, pgUUID(), nil, nil)
	if err == nil {
		t.Fatal("CreateMatch with an invalid config returned nil error, want cfg.Validate's own error")
	}
}

// TestNoConfigUpdateQuery is issue #318's own acceptance criterion: "There
// is no query anywhere in the package that updates matches.config,
// asserted by a source-level test over the query files." RFC §6.2's whole
// point — a match's config is frozen at creation and never reinterpreted —
// is enforced structurally by there being no UPDATE ... matches SET
// config = ... query for a future caller to reach for; this test is what
// notices if one appears, since a code reviewer might not catch a new
// .sql file the same day it's added.
func TestNoConfigUpdateQuery(t *testing.T) {
	entries, err := os.ReadDir("queries")
	if err != nil {
		t.Fatalf("cannot read internal/store/queries: %v", err)
	}

	// Matches "UPDATE matches ... ;" across newlines (SQL statements in this
	// repo's *.sql files are typically multi-line), case-insensitive.
	updateMatches := regexp.MustCompile(`(?is)UPDATE\s+matches\b[^;]*;`)

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		checked++

		b, err := os.ReadFile("queries/" + e.Name())
		if err != nil {
			t.Fatalf("read queries/%s: %v", e.Name(), err)
		}

		for _, stmt := range updateMatches.FindAllString(string(b), -1) {
			if strings.Contains(strings.ToLower(stmt), "config") {
				t.Errorf("queries/%s contains an UPDATE matches statement touching config, forbidden by RFC-001 §6.2 (D44's frozen-Config decision):\n%s", e.Name(), stmt)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no .sql files found under internal/store/queries — this check inspected nothing")
	}
}

// pgUUID returns an arbitrary pgtype.UUID for tests that don't care about
// the actual value — both callers below reject before CreatedBy is ever
// used against a real connection.
func pgUUID() pgtype.UUID {
	return pgtype.UUID{}
}
