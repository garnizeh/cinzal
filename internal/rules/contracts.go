package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// ContractOffer is one of the (0-3) contracts presented at Phase 2, before
// acceptance (GDD §8.1). It carries an origin/destination pair and a tier,
// but no ID and no deadline — both are meaningless before acceptance: GDD
// §8.3 states the deadline as "rounds until it expires, from acceptance",
// and ContractID (game/ids.go) is scoped to a seat's own held slots.
// AcceptOffer turns one of these into a real Contract.
type ContractOffer struct {
	Origin      game.NodeID
	Destination game.NodeID
	Tier        int // indexes game.Config.Contracts, 0-3 (GDD §8.3)
}

// pairKey identifies one origin/destination pair, for D7's without-
// replacement rule: once a pair fills a slot, it is removed from every
// tier's candidate pool, not just the tier it filled from — the tier bands
// overlap by design (a distance-6 pair is valid for II, III and IV at
// once), so leaving a used pair in another tier's pool would let one job
// fill two or three slots as the same pair re-offered under different tier
// labels.
type pairKey struct {
	Origin, Destination game.NodeID
}

// eligibleTiers returns every Config.Contracts index a seat at infamy may be
// offered, ascending. Infamy thresholds are cumulative (0/3/6/9 for
// I/II/III/IV — GDD §8.3), so this is never empty: Tier I's InfamyRequired
// is 0. cfg.Contracts is a fixed-size array, not a map, so ranging it is not
// the §6.3 map-iteration hazard — index order is already 0..3.
func eligibleTiers(infamy int, cfg game.Config) []int {
	var eligible []int
	for i, tier := range cfg.Contracts {
		if tier.InfamyRequired <= infamy {
			eligible = append(eligible, i)
		}
	}
	return eligible
}

// weightedTierDraw picks one eligible tier index via a single RNG.Next call,
// weighted by cfg.Contracts[t].OfferWeight (GDD §8.1, D6): the draw's domain
// is the sum of eligible weights, and the tier whose cumulative range
// contains the draw is returned. Config.Validate rejects a non-positive
// OfferWeight at construction, so the eligible set's total weight is always
// positive here.
func weightedTierDraw(rng *RNG, purpose Purpose, eligible []int, cfg game.Config) int {
	total := 0
	for _, t := range eligible {
		total += cfg.Contracts[t].OfferWeight
	}

	draw := rng.Next(purpose, total)
	for _, t := range eligible {
		w := cfg.Contracts[t].OfferWeight
		if draw < w {
			return t
		}
		draw -= w
	}
	panic("rules: weightedTierDraw: draw exceeded the eligible tiers' total weight")
}

// contractCandidates returns every (Known Warehouse origin, any Border
// destination) pair, at tier's distance band (GDD §8.3), on the currently
// navigable graph (GDD §9.1a item 0), minus used. MaxDistance == 0 means "no
// upper bound" (Tier IV's "6+").
//
// s.Graph.Nodes is NodeID-ascending indexed (Node's own invariant:
// Nodes[i].ID == NodeID(i)), and both loops below walk it in that order, so
// the result is already sorted ascending (Origin, then Destination) without
// a separate sort step — the deterministic order PartialFisherYates'
// candidates contract requires (RFC §6.3, §6.4).
func contractCandidates(s MatchState, seat game.SeatID, tier int, cfg game.Config, used map[pairKey]bool) []ContractOffer {
	band := cfg.Contracts[tier]
	fog := s.Players[seat].Fog

	var candidates []ContractOffer
	for _, origin := range s.Graph.Nodes {
		if origin.Type != game.NodeWarehouse || fog[origin.ID] < game.FogKnown {
			continue
		}

		dist := s.Graph.distances(origin.ID)
		for _, dest := range s.Graph.Nodes {
			if dest.Type != game.NodeBorder {
				continue
			}

			d := dist[dest.ID]
			if d < 0 || d < band.MinDistance || (band.MaxDistance != 0 && d > band.MaxDistance) {
				continue
			}
			if used[pairKey{origin.ID, dest.ID}] {
				continue
			}

			candidates = append(candidates, ContractOffer{Origin: origin.ID, Destination: dest.ID, Tier: tier})
		}
	}
	return candidates
}

// cascade fills one offer slot under D7's fallback ladder: try target's own
// pool, then target-1, down to Tier I, never above target and never by
// widening a band. On the first non-empty pool it draws one pair via
// PartialFisherYates (the same mandated selection primitive every other
// bounded pick in this package uses — rng_shuffle.go) at
// PurposeContractOfferPick, marks it used, and returns it. ok is false only
// when every tier from target down to 0 was empty — the slot is dropped
// (D7), costing no draw for any tier that was never reached and no draw for
// an empty tier that was tried (contractCandidates returning nil consumes
// nothing; only a non-empty pool's single pick does).
func cascade(s MatchState, seat game.SeatID, target int, cfg game.Config, used map[pairKey]bool, rng *RNG) (ContractOffer, bool) {
	for t := target; t >= 0; t-- {
		candidates := contractCandidates(s, seat, t, cfg, used)
		if len(candidates) == 0 {
			continue
		}

		picked := PartialFisherYates(rng, PurposeContractOfferPick, candidates, 1)[0]
		used[pairKey{picked.Origin, picked.Destination}] = true
		return picked, true
	}
	return ContractOffer{}, false
}

// RoundsToNextOffer reports how many rounds until seat's next Phase 2 offer
// attempt, under GDD §8.2's Contact Cooldown — the "Next offer: round N"
// number the HUD always displays (PlayerView.SelfState.RoundsToNextOffer,
// #75's Project reads this). 0 means due now.
//
// p.LastOfferRound == 0 (its Go zero value — RoundNumber is 1-indexed, so 0
// unambiguously means "never offered") reports due immediately: this is
// what makes the opening offer unconditional rather than waiting out a full
// cooldown period nobody has started yet, closing GDD §8.1's v1.8 deadlock.
//
// This does not account for D7's held-offer extensions (2 active contracts,
// or an empty pool) — those can only be discovered by actually attempting
// generation (GenerateOffer). A held offer's true delivery round can be
// later than what this reports, never earlier — "always displayed, never a
// surprise" (GDD §8.2) is about the cooldown number itself, and D7's own
// text is explicit that a held offer's HUD continues to show the ordinary
// display rather than a distinct promise.
func RoundsToNextOffer(s MatchState, seat game.SeatID, cfg game.Config) int {
	p := s.Players[seat]
	if p.LastOfferRound == 0 {
		return 0
	}

	cooldown := cfg.CooldownByTier[infamyTierIndex(p.Infamy, cfg)]
	remaining := int(p.LastOfferRound) + cooldown - int(s.Round)
	return max(0, remaining)
}

// offerDue reports whether seat's Contact Cooldown has elapsed as of s.Round
// (GDD §8.2, §11.1's "Infamy at Phase 2" — s is trusted to be exactly that
// moment's state, the caller's obligation, same as every other
// entry-snapshot consumer in this package).
func offerDue(s MatchState, seat game.SeatID, cfg game.Config) bool {
	return RoundsToNextOffer(s, seat, cfg) == 0
}

// GenerateOffer runs seat's complete Phase 2 (GDD §8.1, §8.2; D6, D7)
// against s, which must be exactly the state as of Phase 2 for the round it
// evaluates — the caller's obligation, matching every other entry-snapshot
// consumer in this package (RFC §6.6).
//
// delivered is false, and offer is nil, with no RNG draw at all, when the
// cooldown has not elapsed (offerDue) or seat already holds 2 active
// contracts (GDD §8.2's full-slots hold) — nothing to attempt, nothing to
// account for.
//
// Otherwise it runs D6's tier-mix assignment — slot 0 targets seat's single
// highest eligible tier outright (no draw); slots 1 and 2 are each an
// independent weighted draw over the eligible set, purpose
// contract.offer.tier, always drawn even when only one tier is eligible —
// then D7's per-slot fallback cascade in slot order 0, 1, 2 against one
// shared without-replacement pool, so the guaranteed slot gets first claim.
// Exactly 2 contract.offer.tier draws are consumed whenever this point is
// reached; 0 to 3 contract.offer.pick draws follow, one per filled slot.
//
// delivered is also false, offer nil, when every slot's cascade came up
// empty (D7's empty-pool hold) — this can only be known by trying, so the 2
// tier draws are still consumed even though nothing is delivered. Otherwise
// offer holds 1-3 contracts and delivered is true; fewer than 3 means the
// pool ran short (D7 disclosure is the caller's/render's concern, not this
// function's).
func GenerateOffer(s MatchState, seat game.SeatID, cfg game.Config, rng *RNG) (offer []ContractOffer, delivered bool) {
	p := s.Players[seat]

	if !offerDue(s, seat, cfg) || len(p.Contracts) >= 2 {
		return nil, false
	}

	eligible := eligibleTiers(p.Infamy, cfg)
	highest := eligible[len(eligible)-1]

	var targets [3]int
	targets[0] = highest
	targets[1] = weightedTierDraw(rng, PurposeContractOfferTier, eligible, cfg)
	targets[2] = weightedTierDraw(rng, PurposeContractOfferTier, eligible, cfg)

	used := map[pairKey]bool{}
	var filled []ContractOffer
	for _, target := range targets {
		if c, ok := cascade(s, seat, target, cfg, used, rng); ok {
			filled = append(filled, c)
		}
	}

	if len(filled) == 0 {
		return nil, false
	}
	return filled, true
}

// prepareNextRound runs D29's Phase 2 (contract offer) and Phase 3
// (market refresh) a round ahead — s.Round standing in for "the round
// about to begin is s.Round+1", the same convention nextUnstableSector
// (incidents.go) already uses for the Headline. It has exactly two call
// sites: the tail of Resolve, where s (next) is the fully-closed state for
// the round just folded in, and initial()'s round-1 bootstrap, where s is
// round 0's freshly-seated state — sharing this one function means a
// reader learns a single mechanism rather than two, and every consumer of
// state = fold(Resolve, initial(seed, cfg), orderLog) gets both phases
// correct automatically (D29's Reasoning).
//
// Contract offers run first, seat-ascending (RFC §6.5's RNG-batch
// default): GenerateOffer's own offerDue check is what decides whether a
// seat draws at all, so a seat whose Contact Cooldown hasn't elapsed costs
// zero RNG draws and leaves PendingOffer untouched (nil, or whatever an
// unrelated earlier round already delivered — never possible in practice,
// since a delivered offer is always answered, never left pending, before
// this runs again for the same seat).
//
// Market stock refreshes second, gated on MarketRefreshDue(s.Round+1,
// cfg) — the *upcoming* round, not s.Round itself: at Resolve's tail
// s.Round is the round just closed, and at initial()'s bootstrap s.Round
// is 0, so "+1" is what makes both call sites ask the same question,
// "is the round about to begin a refresh round."
func prepareNextRound(s *MatchState, cfg game.Config, r *RNG) {
	for _, seat := range bySeat(*s) {
		offer, delivered := GenerateOffer(*s, seat, cfg, r)
		if delivered {
			s.Players[seat].PendingOffer = offer
		}
	}

	if MarketRefreshDue(s.Round+1, cfg) {
		*s = RefreshMarkets(*s, r)
	}
}

// nextContractID assigns a slot-index ID for a new contract, given the
// contracts a seat already holds (0 or 1 of them — GenerateOffer never
// delivers when a seat is already at the 2-contract cap, so AcceptOffer is
// never called against a full hand). ContractID is scoped to "one seat's own
// two slots" (game/ids.go) — 0 and 1 are literally those two slots, reused
// once their previous contract discards. That reuse is safe: GDD §8.4
// always drops any CarriedCargo referencing a contract in the same Upkeep
// step that discards the contract itself, so no stale reference can outlive
// the slot ID it named.
func nextContractID(existing []Contract) game.ContractID {
	if len(existing) == 0 {
		return 0
	}
	if existing[0].ID == 0 {
		return 1
	}
	return 0
}

// AcceptOffer commits seat to one contract from a delivered offer,
// converting it from the pre-acceptance ContractOffer into a real Contract
// (ID assigned, ExpiresRound computed from round + this tier's deadline —
// GDD §8.3), and restarts the Contact Cooldown from round (GDD §8.2: "the
// cooldown starts the round you accept or decline an offer").
//
// s.Players is cloned rather than mutated in place, so a caller holding the
// pre-call MatchState (a prior round's fold result, a test fixture) is never
// aliased into this call's result.
func AcceptOffer(s MatchState, seat game.SeatID, offer ContractOffer, cfg game.Config, round game.RoundNumber) MatchState {
	players := slices.Clone(s.Players)
	p := players[seat]

	contract := Contract{
		ID:           nextContractID(p.Contracts),
		Origin:       offer.Origin,
		Destination:  offer.Destination,
		Tier:         offer.Tier,
		ExpiresRound: round + game.RoundNumber(cfg.Contracts[offer.Tier].Deadline),
	}
	p.Contracts = append(slices.Clone(p.Contracts), contract)
	p.LastOfferRound = round

	players[seat] = p
	s.Players = players
	return s
}

// DeclineOffer restarts seat's Contact Cooldown from round without adding a
// contract (GDD §8.2: "Declining does not reset the cooldown — it restarts
// it" — the identical LastOfferRound write AcceptOffer makes, just with
// nothing appended to Contracts).
func DeclineOffer(s MatchState, seat game.SeatID, round game.RoundNumber) MatchState {
	players := slices.Clone(s.Players)
	p := players[seat]
	p.LastOfferRound = round
	players[seat] = p

	s.Players = players
	return s
}

// applyContractChoices is Resolve's head-of-round step (D29), run
// immediately after resetRoundFlags and before validate — before, not
// after, because a seat already standing at the newly-accepted contract's
// origin can legally Pickup in the very same round, and validate's
// legality check needs p.Contracts to already reflect the acceptance.
//
// A seat whose PendingOffer is nil is skipped entirely: ContractChoice is
// not read, AcceptOffer/DeclineOffer are not called, and LastOfferRound is
// left untouched — there is nothing to accept or decline, and calling
// DeclineOffer anyway would incorrectly restart a cooldown that was never
// due. Only when PendingOffer is non-nil does this read
// orders[seat].ContractChoice: nil, or an index outside 0..len(offer)-1,
// is GDD §15.0's illegal-payload category and degrades to declining that
// offer — never a partial or best-effort accept. A seat absent from
// orders entirely (GDD §18's absence default) reads as the same nil
// ContractChoice, so an absent player never commits to a job it didn't
// choose, for free.
//
// No RNG draw either way: applying a choice, or leaving one unmade, is a
// deterministic state mutation, zero index cost, exactly like
// resetRoundFlags itself.
func applyContractChoices(s *MatchState, orders map[game.SeatID]game.Order, cfg game.Config, round game.RoundNumber) {
	for _, seat := range bySeat(*s) {
		offer := s.Players[seat].PendingOffer
		if offer == nil {
			continue
		}

		choice := orders[seat].ContractChoice
		if choice != nil && *choice >= 0 && *choice < len(offer) {
			*s = AcceptOffer(*s, seat, offer[*choice], cfg, round)
		} else {
			*s = DeclineOffer(*s, seat, round)
		}
		s.Players[seat].PendingOffer = nil
	}
}
