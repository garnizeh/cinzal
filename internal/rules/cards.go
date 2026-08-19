package rules

import "github.com/garnizeh/cinzal/internal/game"

// EventTag is one of GDD §14.2's three event tags — damage, convergence,
// opportunity. A card may carry more than one (Fence's Windfall and Dead
// Runner are both [C][O]), so EventCard.Tags is a slice, not a single value.
type EventTag uint8

const (
	_ EventTag = iota

	TagDamage      // [D] (GDD §14.2)
	TagConvergence // [C] (GDD §14.2)
	TagOpportunity // [O] (GDD §14.2)
)

// EventCard is one row of the global event deck's catalog (GDD §14.2):
// identity, category and tags only — the effect itself is issue #72's scope.
// Splitting the catalogue from the effects means the deck arithmetic here
// and the Headline's DeckCounts can be tested before a single card does
// anything.
type EventCard struct {
	ID       EventCardID
	Name     string
	Category game.EventCategory
	Tags     []EventTag
}

// The 24 event card identities (GDD §14.2). Declaration order is the
// catalog's canonical order — Police, Economy, Underworld, City, each in the
// GDD's own table order — the same fixed-order convention D9 and D10 already
// use for their own tie-breaks: something has to fix a stable order, and
// reusing the GDD's own table order costs nothing to justify.
const (
	_ EventCardID = iota

	EventRaid
	EventCurfew
	EventGatesClosed
	EventPayrollDay
	EventDragnet
	EventShiftChange

	EventCurrencySlide
	EventMarketSurge
	EventPermitAuction
	EventRetainer
	EventFencesWindfall
	EventShippingBoom

	EventInformants
	EventAmnesty
	EventNewBoss
	EventOldFavour
	EventDeadRunner
	EventBounty

	EventBlackout
	EventBridgeDown
	EventRain
	EventDockersStrike
	EventFestival
	EventScaffolding
)

// allEvents is the global event deck's full 24-card catalog (GDD §14.2),
// sorted by canonical order — see the EventCardID block above. buildEventDeck
// passes this directly to ShuffleConstrained, never a copy: ShuffleConstrained
// only ranges its pool argument to group by category, never mutates it.
var allEvents = []EventCard{
	{EventRaid, "Raid", game.CategoryPolice, []EventTag{TagDamage}},
	{EventCurfew, "Curfew", game.CategoryPolice, []EventTag{TagDamage}},
	{EventGatesClosed, "Gates Closed", game.CategoryPolice, []EventTag{TagDamage}},
	{EventPayrollDay, "Payroll Day", game.CategoryPolice, []EventTag{TagDamage}},
	{EventDragnet, "Dragnet", game.CategoryPolice, []EventTag{TagConvergence}},
	{EventShiftChange, "Shift Change", game.CategoryPolice, []EventTag{TagOpportunity}},

	{EventCurrencySlide, "Currency Slide", game.CategoryEconomy, []EventTag{TagDamage}},
	{EventMarketSurge, "Market Surge", game.CategoryEconomy, []EventTag{TagOpportunity}},
	{EventPermitAuction, "Permit Auction", game.CategoryEconomy, []EventTag{TagOpportunity}},
	{EventRetainer, "Retainer", game.CategoryEconomy, []EventTag{TagOpportunity}},
	{EventFencesWindfall, "Fence's Windfall", game.CategoryEconomy, []EventTag{TagConvergence, TagOpportunity}},
	{EventShippingBoom, "Shipping Boom", game.CategoryEconomy, []EventTag{TagConvergence}},

	{EventInformants, "Informants", game.CategoryUnderworld, []EventTag{TagDamage}},
	{EventAmnesty, "Amnesty", game.CategoryUnderworld, []EventTag{TagOpportunity}},
	{EventNewBoss, "New Boss", game.CategoryUnderworld, []EventTag{TagOpportunity}},
	{EventOldFavour, "Old Favour", game.CategoryUnderworld, []EventTag{TagOpportunity}},
	{EventDeadRunner, "Dead Runner", game.CategoryUnderworld, []EventTag{TagConvergence, TagOpportunity}},
	{EventBounty, "Bounty", game.CategoryUnderworld, []EventTag{TagConvergence}},

	{EventBlackout, "Blackout", game.CategoryCity, []EventTag{TagDamage}},
	{EventBridgeDown, "Bridge Down", game.CategoryCity, []EventTag{TagDamage}},
	{EventRain, "Rain", game.CategoryCity, []EventTag{TagDamage}},
	{EventDockersStrike, "Dockers' Strike", game.CategoryCity, []EventTag{TagDamage}},
	{EventFestival, "Festival", game.CategoryCity, []EventTag{TagConvergence}},
	{EventScaffolding, "Scaffolding", game.CategoryCity, []EventTag{TagOpportunity}},
}

// IncidentCard is one row of the sector incident deck's catalog (GDD §14.3):
// identity and hazard/boon only — the effect itself is issue #73's scope.
type IncidentCard struct {
	ID     IncidentCardID
	Name   string
	Hazard bool // false means boon
}

// The 16 sector incident identities (GDD §14.3). Declaration order is
// canonical: hazards in the GDD's own table order, then boons in the GDD's
// own table order — the same convention EventCardID's block uses.
const (
	_ IncidentCardID = iota

	IncidentFlood
	IncidentSnatchJob
	IncidentGuardSweep
	IncidentTorched
	IncidentTurfWar
	IncidentStreetsBlocked
	IncidentGasLeak
	IncidentRiot
	IncidentSinkhole
	IncidentShakedown
	IncidentInformantRing

	IncidentSpilledLoad
	IncidentLocalInformant
	IncidentDistractedGuard
	IncidentOpenDoors
	IncidentWordOfWork
)

// allIncidents is the sector incident deck's full 16-card catalog (GDD
// §14.3), sorted by canonical order — see the IncidentCardID block above.
var allIncidents = []IncidentCard{
	{IncidentFlood, "Flood", true},
	{IncidentSnatchJob, "Snatch Job", true},
	{IncidentGuardSweep, "Guard Sweep", true},
	{IncidentTorched, "Torched", true},
	{IncidentTurfWar, "Turf War", true},
	{IncidentStreetsBlocked, "Streets Blocked", true},
	{IncidentGasLeak, "Gas Leak", true},
	{IncidentRiot, "Riot", true},
	{IncidentSinkhole, "Sinkhole", true},
	{IncidentShakedown, "Shakedown", true},
	{IncidentInformantRing, "Informant Ring", true},

	{IncidentSpilledLoad, "Spilled Load", false},
	{IncidentLocalInformant, "Local Informant", false},
	{IncidentDistractedGuard, "Distracted Guard", false},
	{IncidentOpenDoors, "Open Doors", false},
	{IncidentWordOfWork, "Word of Work", false},
}

// EventCardTags returns id's GDD §14.2 tags, or nil if id names no card in
// the catalog. internal/telemetry's own read of the seed-derivable card
// catalog (D33 rows 13/14): "which card was live this round" is already
// reconstructible from MatchState.Graph.EventDeck alone, so this is the one
// accessor telemetry needs to tell a Convergence-tagged round from any
// other, without a second, duplicated copy of allEvents living outside this
// package.
func EventCardTags(id EventCardID) []EventTag {
	for _, c := range allEvents {
		if c.ID == id {
			return c.Tags
		}
	}
	return nil
}

// incidentCategory maps IncidentCard.Hazard onto ShuffleConstrained's int
// category, per D03's quota: 9 hazards, 4 boons.
const (
	incidentCategoryHazard = 0
	incidentCategoryBoon   = 1
)

// buildEventDeck builds the 12-of-24 match event deck (GDD §14.2): exactly 3
// per category, then fully shuffled into round order (RFC §6.4).
func buildEventDeck(rng *RNG) []EventCardID {
	quota := map[int]int{
		int(game.CategoryPolice):     3,
		int(game.CategoryEconomy):    3,
		int(game.CategoryUnderworld): 3,
		int(game.CategoryCity):       3,
	}
	selected := ShuffleConstrained(rng, PurposeDeckEventSelect, PurposeDeckEventOrder, allEvents,
		func(c EventCard) int { return int(c.Category) }, quota)

	deck := make([]EventCardID, len(selected))
	for i, c := range selected {
		deck[i] = c.ID
	}
	return deck
}

// buildIncidentDeck builds the 13-of-16 match incident deck (GDD §14.3):
// exactly 9 hazards and 4 boons, then fully shuffled into round order (RFC
// §6.4).
func buildIncidentDeck(rng *RNG) []IncidentCardID {
	quota := map[int]int{
		incidentCategoryHazard: 9,
		incidentCategoryBoon:   4,
	}
	selected := ShuffleConstrained(rng, PurposeDeckIncidentSelect, PurposeDeckIncidentOrder, allIncidents,
		func(c IncidentCard) int {
			if c.Hazard {
				return incidentCategoryHazard
			}
			return incidentCategoryBoon
		}, quota)

	deck := make([]IncidentCardID, len(selected))
	for i, c := range selected {
		deck[i] = c.ID
	}
	return deck
}
