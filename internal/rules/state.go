package rules

import (
	"maps"

	"github.com/garnizeh/cinzal/internal/game"
)

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

	// Market is this Black Market's 3 currently rolled items, nil for every
	// other node type (GDD §12). RefreshMarkets (market.go) is the only
	// writer; it replaces this wholesale every refresh, never merges into
	// it — a stale item lingering across a refresh is exactly the kind of
	// silent bug RFC §6.6 warns about.
	Market []game.ItemID

	// SinkholeRounds is rounds remaining before this node is passable
	// again, 0 meaning passable right now (GDD §14.3: "One random node in
	// the sector is impassable for 3 rounds."). Upkeep step 3 decrements
	// it; at zero the node needs no announcement — "it's read off the map
	// like any other passable node" (GDD "Upkeep").
	SinkholeRounds int

	// FenceWindfallActive is Fence's Windfall's standing offer (GDD §14.2):
	// true from the round the card fires until whichever cargo-carrying
	// seat ends a round here first claims it — "First arrival only." Only
	// ever true on a Black Market node. Cleared by the claim, never by
	// Upkeep — unlike the NextRoundModifiers block below, this has no
	// round-bounded lifetime of its own; it simply waits.
	FenceWindfallActive bool
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

	// SpilledLoad marks a loose crate as issue #73's Spilled Load
	// incident's own (GDD §14.3: Cr$ 10, 2 RP) rather than Dead Runner's
	// (GDD §14.2: Cr$ 12, 3 RP) — carried through pickup onto
	// game.CarriedCargo.SpilledLoad (view.go) so resolveOneDelivery
	// (deliveries.go) can tell the two flat payouts apart. Meaningful
	// only when Bound is false.
	SpilledLoad bool
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

	// Ledger is every seat's exact balance as of the end of the previous
	// round (the entry snapshot), set by resolveAddons when this seat buys
	// the Ledger add-on this round (GDD §5.1, §9.5), nil otherwise. Not one
	// of RFC §6.6's original eight cross-round fields — the RFC's own text
	// there argues against a *stored* "previous round's balances" field,
	// which this is not: it is this seat's private purchase *result*, and
	// it exists because Project (#75) reads this MatchState, not Resolve's
	// local entry snapshot, which is discarded when Resolve returns.
	// Cleared at the same top-of-Resolve point as Flagged and
	// EvasiveStepPenalty (resetRoundFlags, resolve.go): "next round only,
	// consumed once."
	Ledger []game.LedgerEntry

	// MarketSurgeActive is Market Surge's per-seat standing bonus (GDD
	// §14.2): "Each player's next delivery pays +50%." Unlike Ledger, this
	// is not "next round only" — it survives however many rounds pass
	// until this seat's next Deliver actually resolves, at which point
	// resolveOneDelivery (deliveries.go) applies the bonus and clears it.
	MarketSurgeActive bool

	// DistractedGuard and StreetsBlocked are issue #73's two sector-
	// incident step-allowance modifiers (GDD §14.3: Distracted Guard "+1
	// step next round", Streets Blocked "capped at 1 step next round") —
	// same lifecycle as Flagged and EvasiveStepPenalty above: consumed
	// from the entry snapshot (SeatSnapshot, below) by legalView
	// (validate.go) into game.StepModifiers, reset by resetRoundFlags at
	// the top of Resolve (resolve.go), and set fresh by incident()
	// (incidents.go) at Phase 7 for whichever seats end this round in the
	// flagged sector. Lifetime: next round only.
	DistractedGuard bool
	StreetsBlocked  bool

	// LocalInformant is the Local Informant boon's own standing grant
	// (GDD §14.3: "sight of every node within 2 steps of wherever they
	// end their route next round") — set by incident() (incidents.go) at
	// Phase 7, same "survives until consumed" lifecycle as
	// MarketSurgeActive above, not resetRoundFlags's "next round only":
	// it is read and cleared by seatSight/distributeTrail (trail.go) the
	// moment next round's writeTrail actually uses it, which happens
	// strictly after resetRoundFlags would already have cleared a
	// same-round-only field.
	LocalInformant bool
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

	// NextRound holds the four global-event cards whose GDD text reads
	// "next round" rather than "this round" (GDD §14.2: Scaffolding,
	// Retainer, Blackout, Dockers' Strike). globalEvent (events.go) sets
	// these when the corresponding card fires; the round that follows
	// reads them as ordinary live state — validate/legalView for
	// Scaffolding and Retainer's step bonuses, Legal for Dockers' Strike,
	// writeTrail for Blackout — before that round's own movement can
	// change what they're evaluated against. Upkeep step 4 (D5) clears
	// them; that step is issue #74's stub, not this one's, so a flag set
	// here persists until #74 lands (matches the precedent trail.go's
	// rainActive/Blackout comment already documents).
	//
	// Warning for #74: an unconditional "clear s.NextRound" at the end of
	// Upkeep would delete a flag globalEvent() just set moments earlier in
	// the very same Resolve() call, before round N+1 ever gets to read it —
	// the identical hazard D5's own Reasoning section already worked
	// through for Flagged/EvasiveStepPenalty ("cannot tell round N's
	// already-consumed value apart from a value Phase 5 just wrote for
	// round N+1"). D5's decided step 4 text — "clear whichever fired for
	// this round" — already points at the right fix (clear only a
	// modifier that was active entering this round, not one merely
	// present when Upkeep runs), but that distinction needs an explicit
	// mechanism, not just careful phrasing, or #74 reintroduces the same
	// bug this field's own consumers were built to avoid.
	NextRound NextRoundModifiers

	// UnstableSector is the sector flagged Unstable for round Round+1 —
	// the round about to begin, not the one just folded in (GDD §14.1:
	// the Headline announces it "at the start of each round, before
	// orders", which is before that round's own Resolve call ever runs,
	// so the value has to already be sitting here when the round before
	// it returns). nil before round 3's value is drawn (GDD §14.3:
	// "nothing in rounds 1-2") or once the match has no further round to
	// announce it for. initial() (initial.go) draws round 3's value at
	// Setup; incident() (incidents.go), at Phase 7, resolves the CURRENT
	// round's incident against whatever this field already held entering
	// Resolve, then overwrites it with the NEXT round's draw — excluding
	// the sector just used, which is the entire mechanism behind "the
	// same sector cannot be flagged two rounds running."
	UnstableSector *game.Sector
}

// NextRoundModifiers is MatchState.NextRound's shape — see that field's own
// comment. Scaffolding is a *game.Sector rather than a bool because its
// bonus is sector-scoped (GDD §14.2: "every player inside it"); nil means no
// Scaffolding is pending.
type NextRoundModifiers struct {
	Scaffolding   *game.Sector
	Retainer      bool
	Blackout      bool
	DockersStrike bool
}

// clone returns a deep copy of m — only Scaffolding needs reallocating; the
// three bools copy by value with the struct itself. See MatchState.clone.
func (m NextRoundModifiers) clone() NextRoundModifiers {
	if m.Scaffolding != nil {
		sector := *m.Scaffolding
		m.Scaffolding = &sector
	}
	return m
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

	// DistractedGuard and StreetsBlocked are issue #73's own two
	// entry-snapshot-sourced step-allowance modifiers (GDD §14.3),
	// same shape as Flagged/EvasiveStepPenalty above — read from here by
	// legalView, never from the live Player field of the same name.
	DistractedGuard bool
	StreetsBlocked  bool
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
			DistractedGuard:    p.DistractedGuard,
			StreetsBlocked:     p.StreetsBlocked,
		}
	}
	return EntrySnapshot{Seats: seats}
}

// clone returns a deep copy of s: every slice and pointer field, at every
// level, is a fresh allocation. Resolve's own acceptance criterion (#67) is
// "Resolve never mutates its argument; the returned state is new and the
// input is byte-identical afterwards" — a plain `next := s` copies the
// struct but leaves every slice and pointer aliased to the original, so a
// write anywhere inside Resolve (Graph.Nodes[i].Post, a Player's Contracts,
// the Fog slice, ...) would reach back into the caller's s. clone is called
// exactly once, at the top of Resolve, before anything else touches state.
func (s MatchState) clone() MatchState {
	var players []Player
	if s.Players != nil {
		players = make([]Player, len(s.Players))
		for i, p := range s.Players {
			players[i] = p.clone()
		}
	}
	var unstableSector *game.Sector
	if s.UnstableSector != nil {
		sector := *s.UnstableSector
		unstableSector = &sector
	}
	return MatchState{
		Round:          s.Round,
		Graph:          s.Graph.clone(),
		Players:        players,
		NextRound:      s.NextRound.clone(),
		UnstableSector: unstableSector,
	}
}

// clone returns a deep copy of g. See MatchState.clone.
func (g Graph) clone() Graph {
	var nodes []Node
	if g.Nodes != nil {
		nodes = make([]Node, len(g.Nodes))
		for i, n := range g.Nodes {
			nodes[i] = n.clone()
		}
	}

	var cargo []Cargo
	if g.Cargo != nil {
		cargo = make([]Cargo, len(g.Cargo))
		copy(cargo, g.Cargo)
	}

	var eventDeck []EventCardID
	if g.EventDeck != nil {
		eventDeck = make([]EventCardID, len(g.EventDeck))
		copy(eventDeck, g.EventDeck)
	}

	var incidentDeck []IncidentCardID
	if g.IncidentDeck != nil {
		incidentDeck = make([]IncidentCardID, len(g.IncidentDeck))
		copy(incidentDeck, g.IncidentDeck)
	}

	return Graph{
		Nodes:        nodes,
		Cargo:        cargo,
		EventDeck:    eventDeck,
		IncidentDeck: incidentDeck,
	}
}

// clone returns a deep copy of n — Edges and Market copied, Post
// reallocated so the clone owns its own lease record. See MatchState.clone.
func (n Node) clone() Node {
	clone := n

	if n.Edges != nil {
		clone.Edges = make([]game.NodeID, len(n.Edges))
		copy(clone.Edges, n.Edges)
	}

	if n.Post != nil {
		post := *n.Post
		clone.Post = &post
	}

	if n.Market != nil {
		clone.Market = make([]game.ItemID, len(n.Market))
		copy(clone.Market, n.Market)
	}

	return clone
}

// clone returns a deep copy of p — every slice, the carried-cargo pointer,
// and the archive's own maps and slice are all fresh. See MatchState.clone.
func (p Player) clone() Player {
	clone := p

	if p.Cargo != nil {
		cargo := *p.Cargo
		clone.Cargo = &cargo
	}

	if p.Contracts != nil {
		clone.Contracts = make([]Contract, len(p.Contracts))
		copy(clone.Contracts, p.Contracts)
	}

	if p.Items != nil {
		clone.Items = make([]game.ItemID, len(p.Items))
		copy(clone.Items, p.Items)
	}

	if p.Posts != nil {
		clone.Posts = make([]game.NodeID, len(p.Posts))
		copy(clone.Posts, p.Posts)
	}

	if p.Fog != nil {
		clone.Fog = make([]game.FogState, len(p.Fog))
		copy(clone.Fog, p.Fog)
	}

	if p.Ledger != nil {
		clone.Ledger = make([]game.LedgerEntry, len(p.Ledger))
		copy(clone.Ledger, p.Ledger)
	}

	clone.Archive = cloneArchive(p.Archive)

	return clone
}

// cloneArchive returns a deep copy of a. RoundSet is an immutable value
// type (rng_purpose.go-style named integer), so the two maps only need
// fresh backing storage, not per-value cloning — but TrailEntry inside
// StampedTrailEntry carries three pointer fields (Actor, Target, Item,
// game/view.go), which do need reallocating or two seats' archives could
// end up sharing one *SeatID a later, unrelated mutation reaches through.
func cloneArchive(a game.SeatArchive) game.SeatArchive {
	clone := game.SeatArchive{}

	if a.Sight != nil {
		clone.Sight = make(map[game.NodeID]game.RoundSet, len(a.Sight))
		maps.Copy(clone.Sight, a.Sight)
	}

	if a.Obscured != nil {
		clone.Obscured = make(map[game.NodeID]game.RoundSet, len(a.Obscured))
		maps.Copy(clone.Obscured, a.Obscured)
	}

	if a.Trail != nil {
		clone.Trail = make([]game.StampedTrailEntry, len(a.Trail))
		for i, e := range a.Trail {
			clone.Trail[i] = e
			if e.Actor != nil {
				actor := *e.Actor
				clone.Trail[i].Actor = &actor
			}
			if e.Target != nil {
				target := *e.Target
				clone.Trail[i].Target = &target
			}
			if e.Item != nil {
				item := *e.Item
				clone.Trail[i].Item = &item
			}
		}
	}

	return clone
}
