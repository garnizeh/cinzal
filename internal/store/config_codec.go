package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is D44's Q2/Q3 for matches.config: "the JSONB payload carries
// an explicit version field ... alongside the config data," decoded in two
// mandatory steps — a recursive exact-key-set check (catches a field
// *missing* from a stored row, the "LeaseCostPerBlock silently becomes 0"
// hazard the whole decision exists to close) and then a strict
// DisallowUnknownFields decode into the version's own frozen struct type
// (catches a field the row should not have). D44 is explicit that a
// Config's two directions need different treatment than orders.payload's:
// a Config is a frozen snapshot RFC §6.2 says is never reinterpreted, with
// no fallback if a later reader cannot prove it read exactly what the
// writer meant — an unrecognized version, or an incomplete key set at any
// nesting level, is a hard decode error, never a default.
//
// configV1Keys and its four nested-type siblings are frozen the moment this
// commit ships, independent of game.Config's own struct definition:
// game.Config is free to gain new fields for a future version (roadmap
// §5's "Config as data" workstream promises it will), but v1's key lists
// never change, or a version-1 row written today would start decoding
// differently once that future field lands — exactly the silent
// reinterpretation D44 forbids. A new field is a new version, not an edit
// to these slices.

// configVersion is the only version DecodeConfig currently accepts. There
// is no migration path yet because there is nothing to migrate from —
// EncodeConfig has only ever written this one version.
const configVersion = 1

// configV1Keys is v1's frozen top-level key list for the "config" object
// inside the envelope — every key game.Config's own json tags name at the
// moment D44 shipped, and no other.
var configV1Keys = []string{
	"rounds", "steps_by_tier", "cooldown_by_tier", "post_cap_by_players",
	"lease_cost_per_block", "lease_block_rounds", "shakedown_cost",
	"ledger_cost", "gate_fee", "starting_balance", "contracts",
	"map_by_players", "max_gen_attempts", "scavenging", "pressure", "suppress",
}

var contractTierV1Keys = []string{
	"infamy_required", "min_distance", "max_distance", "payment", "rp",
	"deadline", "penalty", "penalty_infamy", "offer_weight",
}

var mapSpecV1Keys = []string{"nodes", "min_edges", "max_edges"}

var scavengingTableV1Keys = []string{"cash_roll", "cash_amount", "reveal_roll"}

var pressureConfigV1Keys = []string{"threshold", "cash_penalty", "infamy_penalty"}

var subsystemSuppressionV1Keys = []string{"leases", "incidents", "events", "infamy_tiers", "items"}

// D48 (#345): Config has two JSON shapes D44's original recursive object
// check never reached — fixed-size arrays and int-keyed maps — and each
// needs its own completeness check, not just a per-element/per-value shape
// check, per D48's decision: "the frozen length (arrays) and frozen key set
// (maps) are literal, hand-written data ... never computed from
// len(DefaultConfig().Contracts) [or] maps.Keys(DefaultConfig().MapByPlayers)."

// stepsByTierV1Length and cooldownByTierV1Length are StepsByTier's and
// CooldownByTier's frozen [4]int lengths (index 0 = TierNobody, index 3 =
// TierLegend, per config.go's own doc comments) — D48 names both as fixed
// arrays alongside Contracts.
const (
	stepsByTierV1Length    = 4
	cooldownByTierV1Length = 4
	contractsV1Length      = 4
)

// configV1PlayerCounts is v1's frozen legal player-count key set for
// MapByPlayers and PostCapByPlayers — D48's Q2: "a literal, hand-written key
// set per version," not derived from any live game-package constant, since a
// derived set would answer "what does the code think is valid today," not
// "what did version 1 actually promise to contain when it shipped."
var configV1PlayerCounts = []string{"2", "3", "4", "5"}

// configEnvelope is the actual matches.config JSONB shape: a version tag
// alongside the config data (D44's "e.g. {"v": 1}"), not the config fields
// flattened into the top level — keeping the version metadata and the
// versioned payload in separate keys is what lets configV1Keys check
// "config"'s key set exactly, without also having to carve "v" out of that
// same set by hand.
type configEnvelope struct {
	V      int             `json:"v"`
	Config json.RawMessage `json:"config"`
}

// EncodeConfig is CreateMatch's write-side half of D44: cfg, wrapped in the
// version-1 envelope. There is no validation here — Store.CreateMatch calls
// cfg.Validate before this runs, and EncodeConfig's own job is only to
// produce the bytes a later DecodeConfig can trust, not to re-decide
// whether cfg is legal.
func EncodeConfig(cfg game.Config) ([]byte, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("store: encode config: %w", err)
	}
	b, err := json.Marshal(configEnvelope{V: configVersion, Config: raw})
	if err != nil {
		return nil, fmt.Errorf("store: encode config envelope: %w", err)
	}
	return b, nil
}

// DecodeConfig is LoadMatch's read-side half of D44. It never hands back a
// partially-valid game.Config: any structural failure (unrecognized
// version, a wrong key set at any nesting level, an unknown key, trailing
// data after the JSON value) returns the zero Config alongside its error,
// so a caller that forgets to check err cannot mistake it for real data.
func DecodeConfig(data []byte) (game.Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var env configEnvelope
	if err := dec.Decode(&env); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config envelope: %w", err)
	}
	// Mirrors orderlog.fromRows' own trailing-data guard (CodeRabbit finding
	// on PR #393): json.Decoder.Decode stops at the first complete value, so
	// without this check a payload with garbage appended after a
	// well-formed envelope would decode successfully and silently ignore
	// the rest.
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return game.Config{}, fmt.Errorf("store: decode config envelope: trailing data after JSON value")
	}
	if len(env.Config) == 0 {
		return game.Config{}, fmt.Errorf("store: decode config envelope: missing or empty \"config\" key")
	}

	switch env.V {
	case 1:
		return decodeConfigV1(env.Config)
	default:
		return game.Config{}, fmt.Errorf("store: decode config: unrecognized version %d", env.V)
	}
}

// decodeConfigV1 is D44's two-step decode, both mandatory: (a) the
// recursive exact-key-set check below, which is the only thing that can
// catch a field genuinely absent from raw — DisallowUnknownFields alone
// only rejects a key the struct doesn't have, and is silent about a key the
// struct expects but raw doesn't carry; then (b) a second, redundant
// DisallowUnknownFields decode into game.Config itself, now that (a) has
// already ruled out both directions at every level named below.
func decodeConfigV1(raw json.RawMessage) (game.Config, error) {
	if err := checkExactKeys(raw, configV1Keys); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: %w", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		// checkExactKeys above already proved raw decodes as an object with
		// exactly the right keys, so this can only fail if raw is not a
		// JSON object at all — checkExactKeys itself would already have
		// returned that error first. Kept as a defensive return, never
		// expected to be reached.
		return game.Config{}, fmt.Errorf("store: decode config v1: %w", err)
	}

	// D48 step 2: StepsByTier and CooldownByTier are [4]int fixed arrays
	// with no per-element object shape to check, only their length — a
	// short JSON array silently zero-fills the trailing elements of the Go
	// array on decode, with no error, unless caught here first.
	if err := checkArrayLength(top["steps_by_tier"], stepsByTierV1Length); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: steps_by_tier: %w", err)
	}
	if err := checkArrayLength(top["cooldown_by_tier"], cooldownByTierV1Length); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: cooldown_by_tier: %w", err)
	}

	// contracts is a 4-element JSON array (game.Config.Contracts is
	// [4]ContractTier), not a map — D48 step 2 requires both the array's
	// own length check (independent of, and prior to, any per-element
	// check: a short array must be caught before it is ever decoded into
	// the fixed-size Go array, which would silently zero-fill it) and each
	// present element's own exact-key check against contractTierV1Keys.
	var contracts []json.RawMessage
	if err := json.Unmarshal(top["contracts"], &contracts); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: contracts: %w", err)
	}
	if len(contracts) != contractsV1Length {
		return game.Config{}, fmt.Errorf("store: decode config v1: contracts has %d elements, want exactly %d (GDD §8.3's four tiers)", len(contracts), contractsV1Length)
	}
	for i, c := range contracts {
		if err := checkExactKeys(c, contractTierV1Keys); err != nil {
			return game.Config{}, fmt.Errorf("store: decode config v1: contracts[%d]: %w", i, err)
		}
	}

	// map_by_players and post_cap_by_players are both keyed by player count
	// (2-5). D48 step 3 requires the map's own key set to be checked exactly
	// against configV1PlayerCounts — independent of, and in addition to,
	// checking each present value's own shape — since a map missing its "5"
	// entry passes every per-value check with no error. map_by_players'
	// values are MapSpec objects and additionally get the recursive
	// object-shape check (D48 step 1); post_cap_by_players' values are plain
	// ints with no further shape to check.
	if err := checkExactKeys(top["map_by_players"], configV1PlayerCounts); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: map_by_players: %w", err)
	}
	var mapByPlayers map[string]json.RawMessage
	if err := json.Unmarshal(top["map_by_players"], &mapByPlayers); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: map_by_players: %w", err)
	}
	for k, v := range mapByPlayers {
		if err := checkExactKeys(v, mapSpecV1Keys); err != nil {
			return game.Config{}, fmt.Errorf("store: decode config v1: map_by_players[%s]: %w", k, err)
		}
	}

	if err := checkExactKeys(top["post_cap_by_players"], configV1PlayerCounts); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: post_cap_by_players: %w", err)
	}

	if err := checkExactKeys(top["scavenging"], scavengingTableV1Keys); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: scavenging: %w", err)
	}
	if err := checkExactKeys(top["pressure"], pressureConfigV1Keys); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: pressure: %w", err)
	}
	if err := checkExactKeys(top["suppress"], subsystemSuppressionV1Keys); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: suppress: %w", err)
	}

	// Step (b): a second, redundant strict decode straight into game.Config
	// — DisallowUnknownFields recurses into every nested struct field
	// automatically here (unlike game.Order's wrapper structs, none of
	// Config's nested types own a custom UnmarshalJSON that would stop that
	// propagation — see config.go's own doc comment on Config).
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var cfg game.Config
	if err := d.Decode(&cfg); err != nil {
		return game.Config{}, fmt.Errorf("store: decode config v1: %w", err)
	}

	return cfg, nil
}

// checkExactKeys asserts raw is a JSON object whose key set is exactly
// want — not a superset, not a subset. This is D44's own required
// mechanism, stated verbatim in the decision: "assert its key set is
// exactly the frozen key list ... not a superset, not a subset."
func checkExactKeys(raw json.RawMessage, want []string) error {
	if len(raw) == 0 {
		return fmt.Errorf("missing object (key absent from parent)")
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("not a JSON object: %w", err)
	}

	if len(m) != len(want) {
		return fmt.Errorf("has %d keys, want exactly %d (%v)", len(m), len(want), want)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			return fmt.Errorf("missing key %q", k)
		}
	}
	return nil
}

// checkArrayLength asserts raw is a JSON array of exactly want elements —
// D48's array-length check, read off the raw JSON form rather than decoded
// into the fixed-size Go array it maps to: encoding/json zero-fills a short
// array and truncates a long one before any per-element check gets a chance
// to run, so the length assertion has to happen first, against
// []json.RawMessage, per D48's own reasoning ("the only place to catch the
// omission is before that decode happens").
func checkArrayLength(raw json.RawMessage, want int) error {
	if len(raw) == 0 {
		return fmt.Errorf("missing array (key absent from parent)")
	}

	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return fmt.Errorf("not a JSON array: %w", err)
	}

	if len(elems) != want {
		return fmt.Errorf("has %d elements, want exactly %d", len(elems), want)
	}
	return nil
}

// decodeSeed converts a matches.seed BYTEA column into rules.NewRNG's own
// [32]byte shape (rules.NewRNG(seed [32]byte, ...) — D44's own sentence:
// "a 31-byte row must fail loudly on read... silently zero-padding produces
// a valid-looking match whose every RNG draw is wrong." No padding, no
// truncation — a length other than 32 is a hard error naming the length it
// actually got.
func decodeSeed(b []byte) ([32]byte, error) {
	var seed [32]byte
	if len(b) != 32 {
		return seed, fmt.Errorf("store: seed must be exactly 32 bytes, got %d", len(b))
	}
	copy(seed[:], b)
	return seed, nil
}
