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
	InfamyRequired int

	// MinDistance and MaxDistance bound the shortest-path distance between
	// origin and destination. MaxDistance is 0 for Tier IV's "6+", meaning
	// no upper bound — real distances are never 0.
	MinDistance int
	MaxDistance int

	Payment int // Cr$ on delivery
	RP      int // Reputation on delivery

	Deadline int // rounds until the contract expires, from acceptance

	Penalty int // Cr$ paid on a missed deadline

	// PenaltyInfamy is additional Infamy lost on a missed deadline, on top
	// of Penalty. 0 for every tier except IV, which also costs −2 Infamy.
	PenaltyInfamy int
}

// MapSpec is one row of GDD §6.1's node/edge table. Config.MapByPlayers
// keys this by player count.
type MapSpec struct {
	Nodes    int
	MinEdges int
	MaxEdges int
}

// ScavengingTable is GDD §9.1's reward table for entering a Hidden node,
// rolled on 1D6. A roll below CashRoll finds nothing; a roll in
// [CashRoll, RevealRoll) finds CashAmount; a roll >= RevealRoll reveals
// every node adjacent to the one just entered as Known.
type ScavengingTable struct {
	CashRoll   int
	CashAmount int
	RevealRoll int
}

// PressureConfig is GDD §14.4's check: once per round, every Legend-tier
// player rolls 1D6, and a result <= Threshold costs them CashPenalty and
// InfamyPenalty. There is no separate suppression flag for Pressure — see
// SubsystemSuppression.
type PressureConfig struct {
	Threshold     int
	CashPenalty   int
	InfamyPenalty int
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
	Leases      bool
	Incidents   bool
	Events      bool
	InfamyTiers bool
	Items       bool
}

// Config is every dial the GDD calls tunable, plus the validation that
// stops a config from producing an unservable match (RFC §6.2). It is
// serialised into the match at creation and never read from global state:
// rebalancing never corrupts an in-flight match, and a replay months later
// runs under the rules the match was created with (RFC §6.2).
type Config struct {
	// Rounds is the match length. It is validated against deck arithmetic
	// by Validate, not merely configured — GDD §16.2 works the case for why
	// 20 rounds is unserviceable against the pools sized for 15.
	Rounds int

	// StepsByTier is the step-allowance base before modifiers (GDD §9.1,
	// §9.1a). Index 0 is TierNobody, index 3 is TierLegend.
	StepsByTier [4]int

	// CooldownByTier is rounds between contract offers (GDD §8.2). Same
	// indexing as StepsByTier.
	CooldownByTier [4]int

	// PostCapByPlayers is the post cap per player, keyed by player count
	// (GDD §10.3).
	PostCapByPlayers map[int]int

	// LeaseCostPerBlock is Cr$ per lease block (GDD §10.4). GDD §10.4 calls
	// this "the single most sensitive dial in the game" — M2 sweeps it, so
	// it must never be read as a constant.
	LeaseCostPerBlock int

	// LeaseBlockRounds is rounds held per lease block purchased (GDD §10.4).
	LeaseBlockRounds int

	// ShakedownCost is Cr$ an Evasive loser pays to keep their cargo (GDD
	// §15).
	ShakedownCost int

	// LedgerCost is Cr$ to buy every player's exact balance as of the end
	// of the previous round (GDD §5.1).
	LedgerCost int

	// GateFee is Cr$ charged on every delivery (GDD §6.2).
	GateFee int

	// StartingBalance is Cr$ every player starts a match with (GDD §5).
	StartingBalance int

	// Contracts is GDD §8.3's contract table. Index 0 is Tier I, index 3 is
	// Tier IV.
	Contracts [4]ContractTier

	// MapByPlayers is GDD §6.1's node/edge table, keyed by player count.
	MapByPlayers map[int]MapSpec

	// MaxGenAttempts bounds rules/gen's rejection-and-retry loop (GDD §6.1:
	// "The generator rejects and retries until all hold."). GDD §6.1 states
	// the algorithm, not its termination bound — D9's decision log is
	// explicit that the retry count is an implementation detail for issue
	// #59, not a rules question, but that there must be one and that
	// exhausting it is a returned error, never a partial graph or an
	// infinite loop. A seed that cannot produce a legal map within this
	// many attempts is a bug worth surfacing at match creation.
	MaxGenAttempts int

	// Scavenging is the Hidden-node find table (GDD §9.1).
	Scavenging ScavengingTable

	// Pressure is the Legend-tier end-of-round check (GDD §14.4).
	Pressure PressureConfig

	// Suppress holds the solo-scenario subsystem "off" flags (D11). Zero
	// value for an ordinary match.
	Suppress SubsystemSuppression
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
			{InfamyRequired: 0, MinDistance: 3, MaxDistance: 4, Payment: 8, RP: 2, Deadline: 4, Penalty: 3, PenaltyInfamy: 0},
			{InfamyRequired: 3, MinDistance: 4, MaxDistance: 6, Payment: 14, RP: 3, Deadline: 5, Penalty: 5, PenaltyInfamy: 0},
			{InfamyRequired: 6, MinDistance: 5, MaxDistance: 8, Payment: 20, RP: 5, Deadline: 5, Penalty: 8, PenaltyInfamy: 0},
			{InfamyRequired: 9, MinDistance: 6, MaxDistance: 0, Payment: 30, RP: 8, Deadline: 6, Penalty: 12, PenaltyInfamy: 2},
		},
		MapByPlayers: map[int]MapSpec{
			2: {Nodes: 15, MinEdges: 21, MaxEdges: 23},
			3: {Nodes: 22, MinEdges: 31, MaxEdges: 35},
			4: {Nodes: 25, MinEdges: 36, MaxEdges: 40},
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

	return nil
}
