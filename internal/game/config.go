package game

import "fmt"

// Deck sizes are fixed by card design (GDD §14.2, §14.3), not by
// configuration — no scenario or M2 sweep ever varies how many cards are
// drawn for a match, only Rounds decides how many of them get used.
const (
	// eventCardsDrawn is GDD §14.2: 3 cards per category, 4 categories, out
	// of a 24-card pool.
	eventCardsDrawn = 12

	// incidentCardsDrawn is GDD §14.3: 9 hazards and 4 boons, out of a
	// 16-card pool.
	incidentCardsDrawn = 13
)

// ContractTier is one row of GDD §8.3's contract table. Config.Contracts
// indexes this 0 = Tier I ("Small time") through 3 = Tier IV ("The Score").
type ContractTier struct {
	// InfamyRequired is the minimum Infamy to be offered this tier. 0 for
	// Tier I, which the table lists as "—".
	InfamyRequired int `json:"infamy_required"`

	// MinDistance and MaxDistance bound the shortest-path distance between
	// origin and destination. MaxDistance is 0 for Tier IV's "6+", meaning
	// no upper bound — real distances are never 0.
	MinDistance int `json:"min_distance"`
	MaxDistance int `json:"max_distance"`

	Payment int `json:"payment"` // Cr$ on delivery
	RP      int `json:"rp"`      // Reputation on delivery

	Deadline int `json:"deadline"` // rounds until the contract expires, from acceptance

	Penalty int `json:"penalty"` // Cr$ paid on a missed deadline

	// PenaltyInfamy is additional Infamy lost on a missed deadline, on top
	// of Penalty. 0 for every tier except IV, which also costs −2 Infamy.
	PenaltyInfamy int `json:"penalty_infamy"`

	// OfferWeight is this tier's weight in the two non-guaranteed offer
	// slots' independent draw (GDD §8.1, D6): the first of a three-contract
	// offer always targets the player's highest eligible tier outright, no
	// draw; the other two are a weighted draw over every eligible tier,
	// weighted by this field. Must be positive — Config.Validate rejects
	// zero or negative, the same discipline RFC §6.2 already holds Rounds
	// to, so "every eligible tier has zero weight" can never occur as a
	// runtime case the draw needs an undefined fallback for. Default 1 for
	// every tier (DefaultConfig), i.e. an even split, until M2's simulation
	// harness gives a reason to move it.
	OfferWeight int `json:"offer_weight"`
}

// MapSpec is one row of GDD §6.1's node/edge table. Config.MapByPlayers
// keys this by player count.
type MapSpec struct {
	Nodes    int `json:"nodes"`
	MinEdges int `json:"min_edges"`
	MaxEdges int `json:"max_edges"`
}

// ScavengingTable is GDD §9.1's reward table for entering a Hidden node,
// rolled on 1D6. A roll below CashRoll finds nothing; a roll in
// [CashRoll, RevealRoll) finds CashAmount; a roll >= RevealRoll reveals
// every node adjacent to the one just entered as Known.
type ScavengingTable struct {
	CashRoll   int `json:"cash_roll"`
	CashAmount int `json:"cash_amount"`
	RevealRoll int `json:"reveal_roll"`
}

// PressureConfig is GDD §14.4's check: once per round, every Legend-tier
// player rolls 1D6, and a result <= Threshold costs them CashPenalty and
// InfamyPenalty. There is no separate suppression flag for Pressure — see
// SubsystemSuppression.
type PressureConfig struct {
	Threshold     int `json:"threshold"`
	CashPenalty   int `json:"cash_penalty"`
	InfamyPenalty int `json:"infamy_penalty"`
}

// SubsystemSuppression holds the per-subsystem "off" flags GDD §19.1's solo
// scenario ladder needs, decided in D11. Zero value is false for every
// field — an ordinary match, and scenarios 4 and 5, never set any of them.
//
// There is no Pressure field. Pressure's precondition is Legend tier, which
// is unreachable once InfamyTiers is suppressed, so Pressure is already
// fully suppressed as a consequence (D11).
//
// These flags are unread until M7. They exist now because internal/rules is
// the deepest package in the dependency graph, and retrofitting a
// suppression flag after everything depends on Config's shape means
// reopening a package everyone imports (D11).
type SubsystemSuppression struct {
	Leases      bool `json:"leases"`
	Incidents   bool `json:"incidents"`
	Events      bool `json:"events"`
	InfamyTiers bool `json:"infamy_tiers"`
	Items       bool `json:"items"`
}

// Config is every dial the GDD calls tunable, plus the validation that
// stops a config from producing an unservable match (RFC §6.2). It is
// serialised into the match at creation and never read from global state:
// rebalancing never corrupts an in-flight match, and a replay months later
// runs under the rules the match was created with (RFC §6.2).
//
// Config and every type it nests (ContractTier, MapSpec, ScavengingTable,
// PressureConfig, SubsystemSuppression) carry D44's snake_case json struct
// tags — D44's Q1, "wire vocabulary lives in internal/game." Unlike
// game.Order (order_wire.go), none of these types needs a custom
// MarshalJSON/UnmarshalJSON: D44's audit found no iota-based enum reachable
// from Config — every field here is a plain int, bool, map, or an
// all-scalar nested struct — so encoding/json's reflection-based codec,
// driven only by these tags, is the whole wire format. The version
// dispatch, the recursive exact-key-set check that catches a field missing
// from a stored row, and DisallowUnknownFields all live one layer up, in
// internal/store's own EncodeConfig/DecodeConfig (D44's Q2/Q3, "trust,
// versioning and rejection live in internal/store") — matches.config is a
// frozen, never-reinterpreted snapshot (RFC §6.2), which needs a stronger
// guarantee than Order's append-only, ordinary-missing-field discipline.
type Config struct {
	// Rounds is the match length. It is validated against deck arithmetic
	// by Validate, not merely configured — GDD §16.2 works the case for why
	// 20 rounds is unserviceable against the pools sized for 15.
	Rounds int `json:"rounds"`

	// StepsByTier is the step-allowance base before modifiers (GDD §9.1,
	// §9.1a). Index 0 is TierNobody, index 3 is TierLegend.
	StepsByTier [4]int `json:"steps_by_tier"`

	// CooldownByTier is rounds between contract offers (GDD §8.2). Same
	// indexing as StepsByTier.
	CooldownByTier [4]int `json:"cooldown_by_tier"`

	// PostCapByPlayers is the post cap per player, keyed by player count
	// (GDD §10.3).
	PostCapByPlayers map[int]int `json:"post_cap_by_players"`

	// LeaseCostPerBlock is Cr$ per lease block (GDD §10.4). GDD §10.4 calls
	// this "the single most sensitive dial in the game" — M2 sweeps it, so
	// it must never be read as a constant.
	LeaseCostPerBlock int `json:"lease_cost_per_block"`

	// LeaseBlockRounds is rounds held per lease block purchased (GDD §10.4).
	LeaseBlockRounds int `json:"lease_block_rounds"`

	// ShakedownCost is Cr$ an Evasive loser pays to keep their cargo (GDD
	// §15).
	ShakedownCost int `json:"shakedown_cost"`

	// LedgerCost is Cr$ to buy every player's exact balance as of the end
	// of the previous round (GDD §5.1).
	LedgerCost int `json:"ledger_cost"`

	// GateFee is Cr$ charged on every delivery (GDD §6.2).
	GateFee int `json:"gate_fee"`

	// StartingBalance is Cr$ every player starts a match with (GDD §5).
	StartingBalance int `json:"starting_balance"`

	// Contracts is GDD §8.3's contract table. Index 0 is Tier I, index 3 is
	// Tier IV.
	Contracts [4]ContractTier `json:"contracts"`

	// MapByPlayers is GDD §6.1's node/edge table, keyed by player count.
	MapByPlayers map[int]MapSpec `json:"map_by_players"`

	// MaxGenAttempts bounds rules/gen's rejection-and-retry loop (GDD §6.1:
	// "The generator rejects and retries until all hold."). GDD §6.1 states
	// the algorithm, not its termination bound — D9's decision log is
	// explicit that the retry count is an implementation detail for issue
	// #59, not a rules question, but that there must be one and that
	// exhausting it is a returned error, never a partial graph or an
	// infinite loop. A seed that cannot produce a legal map within this
	// many attempts is a bug worth surfacing at match creation.
	MaxGenAttempts int `json:"max_gen_attempts"`

	// Scavenging is the Hidden-node find table (GDD §9.1).
	Scavenging ScavengingTable `json:"scavenging"`

	// Pressure is the Legend-tier end-of-round check (GDD §14.4).
	Pressure PressureConfig `json:"pressure"`

	// Suppress holds the solo-scenario subsystem "off" flags (D11). Zero
	// value for an ordinary match.
	Suppress SubsystemSuppression `json:"suppress"`
}

// DefaultConfig returns the GDD's v1 numbers. Tests and the M2 simulation
// harness build on this rather than hand-assembling a Config, so a table
// value that moves during balancing only has to change here.
func DefaultConfig() Config {
	return Config{
		Rounds:            15,
		StepsByTier:       [4]int{4, 4, 3, 2},
		CooldownByTier:    [4]int{4, 3, 2, 1},
		PostCapByPlayers:  map[int]int{2: 4, 3: 4, 4: 4, 5: 3},
		LeaseCostPerBlock: 3,
		LeaseBlockRounds:  3,
		ShakedownCost:     4,
		LedgerCost:        3,
		GateFee:           1,
		StartingBalance:   12,
		Contracts: [4]ContractTier{
			{InfamyRequired: 0, MinDistance: 3, MaxDistance: 4, Payment: 8, RP: 2, Deadline: 4, Penalty: 3, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 3, MinDistance: 4, MaxDistance: 6, Payment: 14, RP: 3, Deadline: 5, Penalty: 5, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 6, MinDistance: 5, MaxDistance: 8, Payment: 20, RP: 5, Deadline: 5, Penalty: 8, PenaltyInfamy: 0, OfferWeight: 1},
			{InfamyRequired: 9, MinDistance: 6, MaxDistance: 0, Payment: 30, RP: 8, Deadline: 6, Penalty: 12, PenaltyInfamy: 2, OfferWeight: 1},
		},
		// 4's Nodes/MinEdges/MaxEdges were raised from {25, 36, 40} by
		// issue #229: #203's exit demonstration measured confrontations
		// per match at 13.35-13.38 (Drifter, both root seeds), clearing
		// R9/§22's ">12" action threshold. 28 nodes brings it back to
		// 11.7-11.9, comfortably inside the 4-12 target band — see
		// docs/exit-demos/229-node-count-raise.md.
		//
		// 5's confrontation rate (19.1, same measurement) does not have a
		// matching fix here. D37 swept node count from 28 to 52 at 5
		// players and found R9's threshold does clear — at 46 nodes — but
		// only well past the point where §22's two observation-coverage
		// rows (8 and 19) have left their own target bands: a map raised
		// that far leaves the Board's own data too thin to deduce from,
		// which is the guardrail on R9's remedy (D38). R9's own leading
		// indicator is a different pair — §22 rows 15 and 16, whether
		// players still open the Board at all — and no bot sweep can
		// produce it. The row is deferred to M5.5 rather than bought at
		// that price; see
		// docs/decisions/D37-five-player-confrontation-load.md and
		// docs/decisions/D38-board-going-unused-indicator.md.
		MapByPlayers: map[int]MapSpec{
			2: {Nodes: 15, MinEdges: 21, MaxEdges: 23},
			3: {Nodes: 22, MinEdges: 31, MaxEdges: 35},
			4: {Nodes: 28, MinEdges: 41, MaxEdges: 45},
			5: {Nodes: 28, MinEdges: 40, MaxEdges: 45},
		},
		MaxGenAttempts: 1000,
		Scavenging:     ScavengingTable{CashRoll: 4, CashAmount: 3, RevealRoll: 6},
		Pressure:       PressureConfig{Threshold: 2, CashPenalty: 5, InfamyPenalty: 1},
		// Suppress is the zero value: an ordinary match suppresses nothing.
	}
}

// Validate reports whether c can produce a legal match for the given player
// count. It checks deck arithmetic and map-table coverage rather than
// trusting the caller — both GDD §16.2 and RFC §6.2 call out the simulation
// harness as the likeliest place to sweep straight past an unservable
// Rounds value if match creation is the only thing that checks it.
func (c Config) Validate(players int) error {
	if c.Rounds < 1 {
		return fmt.Errorf("game: Rounds must be at least 1, got %d", c.Rounds)
	}

	// GDD §14.2: global events run rounds 4 through Rounds, one card per
	// round, drawn from a pool sized to cover exactly eventCardsDrawn of
	// them.
	eventRounds := max(c.Rounds-3, 0)
	if eventRounds > eventCardsDrawn {
		return fmt.Errorf("game: Rounds %d needs %d event rounds (4-%d), but only %d event cards are drawn for a match (GDD §14.2, §16.2)",
			c.Rounds, eventRounds, c.Rounds, eventCardsDrawn)
	}

	// GDD §14.3: sector incidents run rounds 3 through Rounds, one card per
	// round, drawn from a pool sized to cover exactly incidentCardsDrawn of
	// them without a reshuffle.
	incidentRounds := max(c.Rounds-2, 0)
	if incidentRounds > incidentCardsDrawn {
		return fmt.Errorf("game: Rounds %d needs %d incident rounds (3-%d), but only %d incident cards are drawn for a match (GDD §14.3, §16.2)",
			c.Rounds, incidentRounds, c.Rounds, incidentCardsDrawn)
	}

	if _, ok := c.PostCapByPlayers[players]; !ok {
		return fmt.Errorf("game: no PostCapByPlayers entry for %d players (GDD §10.3)", players)
	}

	if _, ok := c.MapByPlayers[players]; !ok {
		return fmt.Errorf("game: no MapByPlayers entry for %d players (GDD §6.1)", players)
	}

	if c.MaxGenAttempts < 1 {
		return fmt.Errorf("game: MaxGenAttempts must be at least 1, got %d (GDD §6.1)", c.MaxGenAttempts)
	}

	// A negative StartingBalance has no reading under GDD §5 and breaks the
	// non-negative Player.Balance invariant every Cr$-moving rule in
	// internal/rules trusts implicitly — Currency Slide's own "-25%,
	// rounded down" (GDD §14.2) would *increase* a negative balance instead
	// of decreasing it, since integer division truncates toward zero.
	if c.StartingBalance < 0 {
		return fmt.Errorf("game: StartingBalance must be non-negative, got %d (GDD §5)", c.StartingBalance)
	}

	const maxInt = int(^uint(0) >> 1)
	totalWeight := 0
	for i, tier := range c.Contracts {
		if tier.OfferWeight < 1 {
			return fmt.Errorf("game: Contracts[%d].OfferWeight must be positive, got %d (GDD §8.1, D6)", i, tier.OfferWeight)
		}
		// weightedTierDraw (rules/contracts.go) sums the eligible tiers'
		// OfferWeight into RNG.Next's n argument — reject here, at the
		// config boundary, rather than let that sum silently overflow into
		// a negative or wrapped total mid-match.
		if tier.OfferWeight > maxInt-totalWeight {
			return fmt.Errorf("game: Contracts total OfferWeight overflows int (GDD §8.1, D6)")
		}
		totalWeight += tier.OfferWeight
	}

	return nil
}
