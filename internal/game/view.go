package game

// PlayerView is the fog-filtered projection of match state for one seat —
// the sole payload that crosses from internal/rules to internal/render and
// internal/web (RFC §3, §9.1). internal/rules.Project is its only producer,
// and it is defined here, not there, because game is the only package
// render and web may import (D01).
type PlayerView struct {
	// Round is the round this view was rendered for (RFC §9.1).
	Round RoundNumber

	// You is this seat's own state, exact and complete — nothing about
	// your own hand, position, or balance is fog-filtered (GDD §5).
	You SelfState

	// Nodes holds every node this seat has discovered, Rumoured or better.
	// A Hidden node is an absent key, never a zero-valued entry — the
	// representation that makes the "NodeView{Type: \"\"}" leak unspellable
	// (RFC §9.1).
	Nodes map[NodeID]NodeView

	// Others is every opponent's public state: credit band, exact Infamy
	// and RP, and posts. Never an exact balance, and never a position
	// unless one of RFC §9.1's eleven authorised writers puts it in Trail
	// or Anchors instead.
	Others []OpponentView

	// Trail is this round's sight-gated log: entries at nodes this seat
	// had sight of this round (GDD §7.3; RFC §9.1 rows 1, 7, 8, and 12).
	Trail []TrailEntry

	// Anchors is the public record of named, node-fixing facts, reaching
	// every seat unconditionally regardless of sight (RFC §9.1 rows 2-6
	// and 9-11). GDD §7.5's Attribution tool works backward from exactly
	// this list.
	Anchors []Anchor

	// Headline is the pre-order broadcast of the live event category and
	// the round's Unstable Sector (GDD §14.1).
	Headline Headline

	// Deck is the Sector Incident deck's remaining hazard/boon counts,
	// always displayed (GDD §14.3).
	Deck DeckCounts

	// Archive is this seat's own observation history and nothing else —
	// seat A's archive must never reach seat B's view (RFC §9.2).
	Archive SeatArchive

	// NodeStats is the Heat Map's per-node rate, derived from Archive (RFC
	// §9.2, GDD §7.5).
	NodeStats map[NodeID]NodeStats
}

// SelfState is this seat's own state: exact and complete, because nothing
// about your own position, balance, or hand is hidden from you (GDD §5).
type SelfState struct {
	// Balance is exact Cr$ (GDD §5) — opponents see only Others[n].Band.
	Balance int

	// Infamy is 0-10, public and exact for every seat, yours included (GDD
	// §5, §11).
	Infamy int

	// RP is Reputation, public and exact for every seat (GDD §5).
	RP int

	// Position is this seat's current node. Hidden from every other seat
	// (GDD §5) — it never appears on OpponentView.
	Position NodeID

	// Cargo is nil when carrying nothing. Hidden from every other seat —
	// the pickup leaves a trace, but the cargo itself does not (GDD §5).
	Cargo *CarriedCargo

	// Contracts is up to 2 active contracts, hidden from every other seat
	// (GDD §5, §8.1, §8.4).
	Contracts []ContractInHand

	// Items is the item hand, up to 3, hidden from every other seat (GDD
	// §12).
	Items []ItemID

	// Posts is every post this seat holds. Public regardless of fog —
	// staking and lease expiry are both global announcements (GDD §5,
	// §10.1; RFC §9.1 rows 3, 4) — repeated here because it is also part
	// of your own state.
	Posts []Post

	// Ledger is nil unless this seat bought the Ledger this round, in
	// which case it holds every seat's exact balance as of the end of the
	// previous round (GDD §5.1). Kept off OpponentView and typed apart
	// from it so a live band and a one-round-stale balance can never be
	// confused.
	Ledger []LedgerEntry

	// StepAllowance is this round's step budget, computed server-side from
	// the full modifier chain (GDD §9.1a) so the HUD number can never
	// disagree with the server's (GDD §19.2, RFC §10.2).
	StepAllowance int

	// RoundsToNextOffer is the HUD's "rounds until next contract offer"
	// invariant (GDD §8.2, §19.2). Computed server-side: the hold rules on
	// a full hand or an empty pool (D7) are not a client computation (RFC
	// §10.2).
	RoundsToNextOffer int
}

// CarriedCargo is the one cargo a seat may carry (GDD §5, §8.4).
type CarriedCargo struct {
	// Bound is true for cargo picked up against a contract, collectible
	// only by a holder of a contract with the same origin and destination.
	// False for a loose crate, collectible by anyone (GDD §8.4).
	Bound bool

	// Contract is the contract this cargo is bound to. Meaningful only
	// when Bound is true.
	Contract ContractID
}

// ContractInHand is one contract this seat currently holds (GDD §8.1,
// §8.3, §8.4).
type ContractInHand struct {
	// ID identifies this contract instance, scoped to this seat's own two
	// slots (GDD §8.4).
	ID ContractID

	// Origin is the Warehouse to pick up from — Known to this seat before
	// the contract was ever offered (GDD §8.1).
	Origin NodeID

	// Destination is the Border to deliver to. Rumoured the moment this
	// contract was accepted (GDD §8.1).
	Destination NodeID

	// Tier is 0-3, indexing Config.Contracts (GDD §8.3).
	Tier int

	// ExpiresRound is the absolute round this contract's deadline falls
	// on — not a duration, unlike Config.Contracts[Tier].Deadline (GDD
	// §8.1, §8.3).
	ExpiresRound RoundNumber

	// DeadlinePauseUsed is GDD §8.4's Deadline Pause: losing a
	// confrontation while carrying this contract's cargo extends its
	// deadline by 1 round, once. The flag lives on the contract instance,
	// never on the cargo or the seat, so two seats racing for the same
	// dropped crate never share or inherit it (GDD §8.4).
	DeadlinePauseUsed bool
}

// Post is one node a seat holds a lease on, public regardless of fog (GDD
// §5, §10.1, §10.4).
type Post struct {
	// Node is the node this lease occupies (GDD §10.1).
	Node NodeID

	// RoundsRemaining is the HUD's "rounds remaining on each lease"
	// invariant (GDD §10.4, §19.2), computed server-side.
	RoundsRemaining int
}

// LedgerEntry is one seat's exact balance as of the end of the previous
// round, purchased via the Ledger add-on (GDD §5.1, §9.5). It appears only
// inside SelfState.Ledger — see the comment there for why it is not folded
// into OpponentView.
type LedgerEntry struct {
	Seat SeatID // whose balance this is

	Balance int // exact Cr$, one round stale (GDD §5.1)
}

// NodeView is one node as this seat currently knows it. Present in
// PlayerView.Nodes only from FogRumoured upward — a Hidden node has no
// entry at all (GDD §7.1, RFC §9.1).
type NodeView struct {
	// Fog is this seat's current discovery state for the node — Rumoured,
	// Known, or InSight (GDD §7.1). Never FogHidden: a Hidden node has no
	// entry in the map at all.
	Fog FogState

	// Name, Type and Sector are disclosed from Rumoured onward (GDD §7.1).
	Name   string
	Type   NodeType
	Sector Sector

	// X, Y are canvas coordinates, generated once at match setup and never
	// recomputed per viewer (D10, RFC §9.1). Disclosed from Rumoured
	// onward, exactly when the node itself is.
	X, Y int

	// Edges is nil for a Rumoured node: the client cannot plot a route
	// into or through it (GDD §7.1; RFC §9.1: "the client physically
	// cannot plot into it"). Populated from Known onward.
	Edges []NodeID

	// Post is the lease occupying this node, nil if unowned. Populated
	// from Known onward (GDD §7.1: "post owner and lease expiry").
	Post *NodePost

	// Market is the Black Market's 3 rolled items on offer. Populated
	// only when this node is InSight and NodeBlackMarket (GDD §7.1, §12).
	Market []ItemID
}

// NodePost is the lease on one node, as disclosed by that node's own fog
// state (GDD §7.1) — distinct from the Post slice on SelfState and
// OpponentView, which lists a seat's posts regardless of fog.
type NodePost struct {
	Owner SeatID // the seat holding this lease (GDD §5, §10.1)

	RoundsRemaining int // GDD §10.4
}

// OpponentView is one other seat's public state (GDD §5). Never carries a
// position or an exact balance — RFC §9.1's eleven writers are the only
// way a position reaches Trail or Anchors instead, and Band is the only
// balance-shaped field this type has room for.
type OpponentView struct {
	Seat SeatID // which opponent this is

	// Band is the credit band, never the exact balance (GDD §5.1).
	Band CreditBand

	// Infamy and RP are public and exact for every seat (GDD §5).
	Infamy int
	RP     int

	// Posts is every node this seat holds a lease on. Public regardless of
	// fog — staking and expiry are both global announcements (GDD §5,
	// §10.1; RFC §9.1 rows 3, 4).
	Posts []Post
}

// TrailEntry is one sight-gated fact from this round's trail log — RFC
// §9.1 rows 1, 7, 8, and 12, plus the never-named Fresh Tracks and the
// pre-3rd-round Loitering entry (GDD §7.3). Round is not repeated here: a
// TrailEntry lives inside PlayerView.Trail, which is always this round's
// log — see StampedTrailEntry for the archived, multi-round form.
type TrailEntry struct {
	Kind EventKind // which trail archetype this is (GDD §7.3)
	Node NodeID    // where it happened

	// Actor is the seat named, when this entry names one. Nil for Fresh
	// Tracks, which is never named, and for a Loitering entry before its
	// 3rd consecutive round, which is node-only (GDD §7.3).
	Actor *SeatID

	// Target is the second party. Populated only for EventConfrontation,
	// which always names both (GDD §7.3, RFC §9.1 row 7).
	Target *SeatID

	// Item is the item bought. Populated only for EventItemPurchased when
	// it is named — Infamy >= 6 (GDD §7.3, RFC §9.1 row 8).
	Item *ItemID
}

// Anchor is one public, globally-broadcast fact that fixes a node in a
// round — RFC §9.1 rows 2-6 and 9-11, reaching every seat unconditionally
// regardless of sight. GDD §7.5's Attribution tool works backward from
// exactly this list.
type Anchor struct {
	Kind  EventKind   // which of RFC §9.1's global writer rows this is
	Round RoundNumber // when it happened
	Node  NodeID      // where it happened

	// Actor is the seat named. Nil for the two node-only rows — Loitering,
	// 3+ consecutive rounds, and a loose crate held 2+ consecutive rounds
	// (RFC §9.1 rows 5, 6) — which fix a node without naming a player.
	Actor *SeatID

	// Tier is the contract tier delivered, 0-3 indexing Config.Contracts.
	// Populated only for EventDelivered — the delivery announcement always
	// names tier and location alongside the actor (GDD §7.3).
	Tier int
}

// Headline is the pre-order broadcast (GDD §14.1): the next global event's
// category, and the round's Unstable Sector — never which card.
type Headline struct {
	// Category is nil in rounds 1-3, before global events start (GDD
	// §14.1, §14.2).
	Category *EventCategory

	// Sector is nil in round 1-2, before sector incidents start (GDD
	// §14.1, §14.3).
	Sector *Sector
}

// DeckCounts is the Sector Incident deck's remaining composition, always
// displayed so the Unstable Sector flag is a priced gamble rather than a
// coin flip (GDD §14.3).
type DeckCounts struct {
	HazardsRemaining int // of the 9 drawn for the match (GDD §14.3)
	BoonsRemaining   int // of the 4 drawn for the match (GDD §14.3)
}
