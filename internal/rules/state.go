package rules

import "github.com/garnizeh/cinzal/internal/game"

// Node is one node of the match's graph, exactly as it stands mid-match —
// topology, sector, layout, and the state Bridge Down and Sinkhole change,
// permanently or temporarily (GDD §6, §14.2, §14.3). Nodes[i].ID ==
// game.NodeID(i) always, matching RFC §6.4 and §6.5's own NodeID-ascending
// convention, so Graph never needs a map or a separate sort key to look one
// up or to iterate the whole graph in a stable order.
type Node struct {
	ID     game.NodeID
	Name   string
	Type   game.NodeType
	Sector game.Sector

	// X, Y are canvas coordinates, generated once at setup and never
	// recomputed per viewer (D10, RFC §6.4).
	X, Y int

	// Edges is this node's adjacency list, ascending game.NodeID. It is the
	// live, navigable graph: Bridge Down (GDD §14.2, "One random edge is
	// destroyed permanently. The map is now different.") removes an entry
	// from both endpoints and nothing restores it. Pushing On's priority
	// ladder (GDD §9.1) and every other distance computation reads this,
	// never the static setup graph — "or the ladder will cheerfully steer
	// a player into a hole."
	Edges []game.NodeID

	// Post is the lease occupying this node, nil if unowned (GDD §10.1,
	// §10.4).
	Post *Post

	// SinkholeRounds is rounds remaining before this node is passable
	// again, 0 meaning passable right now (GDD §14.3: "One random node in
	// the sector is impassable for 3 rounds."). Upkeep step 3 decrements
	// it; at zero the node needs no announcement — "it's read off the map
	// like any other passable node" (GDD "Upkeep").
	SinkholeRounds int
}

// Post is one seat's lease on a node (GDD §10.1, §10.4).
type Post struct {
	Owner game.SeatID

	// RoundsRemaining is decremented by Upkeep step 2. At zero the lease
	// expires and "The corner went quiet" fires publicly — identically
	// whether it expired on its own or was surrendered to Debt (GDD §13,
	// "Upkeep": "the same trace whether the lease expired on its own or
	// was surrendered for debt. The district never learns which.").
	RoundsRemaining int
}

// Cargo is one piece of cargo sitting on the map (GDD §8.4, §15). Bound
// cargo fell there from a lost confrontation and keeps the origin/
// destination pair of the contract it was picked up against — "still bound
// to its original origin/destination pair (§8.4) — it does not become a
// loose crate" — collectible only by a seat holding a contract with that
// same pair. A loose crate (Dead Runner, Spilled Load) has neither: Origin
// and Destination are the zero NodeID and any seat may collect it.
type Cargo struct {
	Node  game.NodeID
	Bound bool

	Origin      game.NodeID // meaningful only when Bound
	Destination game.NodeID // meaningful only when Bound
}

// EventCardID identifies one of the global event deck's 24 cards (GDD
// §14.2). The concrete catalog — Currency Slide, Bridge Down, and the rest —
// is issue #72's scope; this package needs only a stable identifier now, to
// build and pop the deck order RFC §6.4 describes.
type EventCardID uint8

// IncidentCardID identifies one of the sector incident deck's 16 cards (GDD
// §14.3), including Sinkhole. The concrete catalog is issue #73's scope; see
// EventCardID.
type IncidentCardID uint8

// Graph is the map: topology and layout fixed at setup (GDD §6, D10), the
// permanent and temporary mutations Bridge Down and Sinkhole apply to it,
// every piece of cargo currently on the ground, and the two card decks'
// remaining order.
type Graph struct {
	// Nodes is indexed by NodeID — see Node's own comment.
	Nodes []Node

	// Cargo is every piece of cargo currently on the map. Appended in
	// resolution order, which is already a deterministic sequence, but
	// that is not itself a sort key: a caller that needs to iterate this
	// for anything order-sensitive must impose its own explicit one first
	// (RFC §6.3).
	Cargo []Cargo

	// EventDeck and IncidentDeck are the remaining deck order, built once
	// in initial(seed, cfg) and never reshuffled (RFC §6.4). Index 0 is
	// the next card: Phase 1's Headline peeks it without popping; Phases 6
	// and 7 pop it.
	EventDeck    []EventCardID
	IncidentDeck []IncidentCardID
}

// Contract is one contract instance held by a seat — per-player, never a
// shared or match-global object (GDD §8.4): "each player draws their own
// offer of three and keeps their own contract, with its own deadline and its
// own Deadline Pause flag."
type Contract struct {
	ID game.ContractID

	Origin      game.NodeID // a Warehouse, Known before the offer (GDD §8.1)
	Destination game.NodeID // a Border, Rumoured on acceptance (GDD §8.1)

	Tier int // indexes game.Config.Contracts, 0-3 (GDD §8.3)

	// ExpiresRound is the absolute round this contract's deadline falls
	// on, decremented by Upkeep step 1 (GDD "Upkeep").
	ExpiresRound game.RoundNumber

	// DeadlinePauseUsed is GDD §8.4's once-per-contract flag: losing a
	// confrontation while carrying this contract's cargo extends its
	// deadline by 1 round, once. It lives on the contract instance, never
	// on the seat, so two seats racing for the same dropped crate never
	// share or inherit it — "B is fulfilling B's contract, on B's
	// untouched deadline, with B's own Pause still available."
	DeadlinePauseUsed bool
}

// Player is one seat's complete state, exact and never fog-filtered — fog
// filtering happens exactly once, at Project, the only function that reads
// this and produces something render or web may hold (RFC §3).
type Player struct {
	Seat game.SeatID

	Balance  int
	Infamy   int
	RP       int
	Position game.NodeID

	Cargo *game.CarriedCargo // nil when carrying nothing (GDD §5, §8.4)

	Contracts []Contract // up to 2, per-player instances (GDD §8.1, §8.4)

	Items []game.ItemID // hand, up to 3 (GDD §12)

	Posts []game.NodeID // nodes this seat holds a lease on, ascending NodeID

	// Fog is this seat's discovery state for every node — Fog[i] is this
	// seat's game.FogState for game.NodeID(i) (GDD §7.1's four states).
	// Indexed exactly like Graph.Nodes, so no map is ever needed here
	// either.
	Fog []game.FogState

	// Archive is this seat's observation history: sight, obscured rounds,
	// and every trail entry it has ever received (RFC §9.2). It is the
	// Board's data source and never crosses into any other seat's view.
	Archive game.SeatArchive

	// The eight cross-round counters RFC §6.6 enumerates. Each field's
	// comment states its consumer and its lifetime, matching that table.

	// LastEndNode is where this seat's route ended last round, read by
	// Loitering's 1-step radius test (GDD §9.1). Lifetime: previous round.
	LastEndNode game.NodeID

	// LoiteringStreak counts consecutive loitering rounds, escalating
	// silent → trace at 2 → global at 3+ (GDD §9.1). Lifetime: until
	// broken.
	LoiteringStreak int

	// LooseCrateHeldRounds counts consecutive rounds this seat has ended
	// holding an unbound loose crate, escalating to a global announcement
	// at 2+ (GDD §8.4). Lifetime: while carried.
	LooseCrateHeldRounds int

	// Flagged is Debt's −1 step penalty (GDD §13) — a boolean, never
	// stacks. Consumed from the entry snapshot by the step-allowance
	// formula, not cleared by Upkeep (RFC §6.6; GDD "Upkeep": "Flagged
	// and the Evasive step penalty are not cleared here at all."). The
	// live field here is reset at the same top-of-Resolve point the
	// snapshot is taken, before anything in the round can write to it
	// again — that reset is Resolve's job (#67), not this type's.
	// Lifetime: next round only.
	Flagged bool

	// EvasiveStepPenalty is the −1 step penalty after an Evasive loss (GDD
	// §15). Same consumption and reset rule as Flagged. Lifetime: next
	// round only.
	EvasiveStepPenalty bool

	// LastOfferRound is the round this seat's Contact Cooldown last reset
	// — written once per offer or decline, read as a difference against
	// the current round (GDD §8.2). Needs no per-round mutation of its
	// own (GDD "Upkeep": "LastOfferRound needs no mutation... written once
	// and read as a difference"). Lifetime: until the next offer.
	LastOfferRound game.RoundNumber

	// ConsecutiveDefaults counts consecutive rounds this seat's order was
	// not a real human submission, feeding Autopilot engagement (RFC
	// §8.2). Lifetime: until a real submission.
	ConsecutiveDefaults int
}

// MatchState is the match's authoritative state: the graph and every seat,
// as of the round just folded in. It has no name outside this package —
// everything a player is entitled to see leaves through Project as a
// game.PlayerView, and there is no second path (RFC §3). It is derived by
// the fold and never stored: state = fold(Resolve, initial(seed, cfg),
// orderLog) (RFC §7.1).
type MatchState struct {
	// Round is the round this state was produced by — the last round
	// Resolve folded in, 0 before the first call. Contract deadlines,
	// LastOfferRound, and the Ledger's one-round staleness (GDD §5.1) are
	// all absolute-round comparisons that need this to mean anything.
	Round game.RoundNumber

	Graph Graph

	// Players is indexed by SeatID — Players[i].Seat == game.SeatID(i)
	// always (RFC §6.5's seat-index default ordering), so no map and no
	// separate sort key are ever needed to look one up or to iterate every
	// seat in a stable, fair order.
	Players []Player
}

// SeatSnapshot is one seat's frozen end-of-previous-round truth (RFC §6.6):
// exactly the fields EntrySnapshot's three consumers need, and nothing else
// — this is deliberately not a copy of Player. See EntrySnapshot.
type SeatSnapshot struct {
	Seat game.SeatID

	// Balance is the Ledger's one-round-stale figure (GDD §5.1).
	Balance int

	// Infamy is the step-allowance formula's tier base (GDD §9.1a) and
	// what the Legend broadcast tests against (GDD §11.1).
	Infamy int

	// Position is what the Legend order-phase broadcast reveals (GDD §11,
	// RFC §9.1 row 10) — evaluated "as the order phase opens" (GDD
	// §11.1), which is this frozen value, not whatever Resolve computes
	// mid-round.
	Position game.NodeID

	// Flagged and EvasiveStepPenalty are the step-allowance formula's two
	// entry-snapshot-sourced modifiers (GDD §9.1a, RFC §6.6) — read from
	// here, never from the live, mutable Player field of the same name.
	Flagged            bool
	EvasiveStepPenalty bool
}

// EntrySnapshot is "end of the previous round" truth, frozen at the top of
// Resolve (RFC §6.6): entry := s.Snapshot(). It has three named consumers —
// the Ledger (GDD §5.1's one-round staleness by design), the step allowance
// (frozen at round start, so a mid-round Infamy swing can never shrink a
// route the server already accepted as legal), and the Legend order-phase
// broadcast (GDD §11.1) — named explicitly as a value here, rather than
// treating the Ledger as a special case that reads stale state some other
// way. RFC §6.6: "the fix is not stored state — 'end of the previous round'
// is exactly the state entering Resolve."
type EntrySnapshot struct {
	// Seats is indexed like MatchState.Players — Seats[i].Seat ==
	// game.SeatID(i).
	Seats []SeatSnapshot
}

// Snapshot freezes s's per-seat entry state: exactly the fields
// EntrySnapshot's three consumers need, copied by value so no later
// mutation of s can reach the result (RFC §6.6). It does not clear
// s.Players[*].Flagged or EvasiveStepPenalty — that reset is Resolve's own
// job (#67), performed once, at the same point it takes this snapshot,
// before any resolution step can write to either field.
func (s MatchState) Snapshot() EntrySnapshot {
	seats := make([]SeatSnapshot, len(s.Players))
	for i, p := range s.Players {
		seats[i] = SeatSnapshot{
			Seat:               p.Seat,
			Balance:            p.Balance,
			Infamy:             p.Infamy,
			Position:           p.Position,
			Flagged:            p.Flagged,
			EvasiveStepPenalty: p.EvasiveStepPenalty,
		}
	}
	return EntrySnapshot{Seats: seats}
}
