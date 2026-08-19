package telemetry

import (
	"errors"
	"fmt"
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// allStances, allActions and allItems are every declared value of their
// respective enums, in the order internal/game declares them — the closed
// key sets RoundActions populates every entry of, never only the values a
// given match happened to produce. A map keyed by a subset would silently
// drop GDD §22's own worked example ("the current suspects are Surveil and
// Vanish") the one time it matters: an action nobody ever chose still
// needs its own zero-valued Rate, or a reader cannot tell "never chosen,
// dead weight" from "not tracked."
var (
	allStances = []game.Stance{game.StanceAggressive, game.StanceNeutral, game.StanceEvasive}
	allActions = []game.ActionKind{
		game.ActionPickup, game.ActionDeliver, game.ActionStakePost, game.ActionDeal,
		game.ActionVanish, game.ActionSurveil, game.ActionNothing,
	}
	allItems = []game.ItemID{
		game.ItemShiv, game.ItemMuscle, game.ItemPoliceBand, game.ItemCirculationPermit,
		game.ItemTornMap, game.ItemDecoy, game.ItemBoltHole, game.ItemGuardContact,
	}
)

// RoundActionSummary is GDD §22's per-round and per-action metric set,
// computed once by RoundActions from one match's order log. It is a
// separate type from MatchSummary, not more fields on it, because it is a
// different shape read by a different audience (issue #198): MatchSummary
// is twenty numbers with bands and actions, read once per configuration by
// whoever decides whether a dial moves; these are distributions, read
// across a whole sweep, whose thresholds ask whether a *mechanic* — a
// stance, an action, an item — is pulling its weight at all.
//
// # The bot-play caveat
//
// RFC §16.4: "bot play is not human play." Every field below carries GDD
// §22's own recommended remedy ("cut or buffed," "isn't a real choice"),
// and that remedy is exactly where bot data can point the wrong way: an
// action a bot never picks may be an action it does not understand, not
// one that is dead weight for a human player. These fields ship as
// instrumentation for M5.5 to read against human data, not as M2 findings
// on their own — reading a verdict off them directly, the way D35's
// per-match bands are read, is misusing them. The one exception issue #198
// names is the gap between them: an action Drifter (uniform-random) picks
// and Operator (a bot that is trying) does not is a stronger signal about
// dead weight than either tier's frequency alone, and needs no human
// baseline to be worth watching — see ActionFrequencyGap.
//
// # Rows this package does not answer
//
// Two per-round rows are human-timing metrics with no meaning against
// bots — median/90th-percentile time to submit an order, and the count of
// players who let the timer expire — both M4's round-lifecycle scope, not
// M2's; declaring a zero for either here would read as "bots never time
// out," which is true but not the measurement GDD §22 means. Post-match
// recall (per-action) is an M5.5 playtest question, the same shape as
// MatchSummary's own row 15/18 omissions (see this package's doc.go):
// nothing computes it, because nothing headless could. Card impact on
// final standings (per-action, "a card that never changes an outcome is a
// card that only costs reading time") is the closest analogue to
// MatchSummary's own row 13: GDD §22 gives no operational definition of
// "swing," and answering it precisely needs the same per-round MatchState
// diffing D33 declined to build for row 13, cross-referenced against the
// seed-derivable card identity liveConvergenceRounds/liveIncidentRounds
// already compute. It ships open here for the same reason row 13 shipped
// open in #197, not silently approximated.
type RoundActionSummary struct {
	// StanceDistribution is GDD §22 per-round: "Distribution of stance
	// choices. If Evasive exceeds 60%, the stance choice isn't a real
	// choice." One entry per game.Stance, every entry's N the same
	// player-round population (len(s.Players) * cfg.Rounds): every legal
	// order declares exactly one stance (game.StanceOrder's zero value is
	// reserved invalid — GDD §9.3 is not optional), so "choices" reads as
	// every order submitted, not merely orders that reached a
	// confrontation.
	StanceDistribution map[game.Stance]Rate

	// LedgerPurchaseRate is GDD §22 per-round: "Ledger purchase rate. If
	// under 10%, bands aren't creating real uncertainty; if over 70%, Cr$
	// 3 is too cheap." The Ledger (GDD §5.1: Cr$ 3, "learn every player's
	// exact balance as of the end of last round") is a private order
	// add-on, Order.AddOns.BuyLedger — GDD §5.1 itself calls the purchase
	// private, and no EventKind exists for it, so this reads the order log
	// directly, the same source LedgerPurchasesByRound reduces
	// differently. N is player-rounds, the same population as
	// StanceDistribution.
	LedgerPurchaseRate Rate

	// LedgerPurchasesByRound is GDD §22 per-round: "Ledger purchases by
	// round number. A spike in the last three rounds means the staleness
	// rule isn't doing its job." One Rate per round, index 0 is round 1,
	// len(LedgerPurchasesByRound) == cfg.Rounds; each entry's N is
	// len(s.Players) — this round's own purchases against this round's own
	// player count, not LedgerPurchaseRate's match-wide population. GDD
	// §5.1's own final-round blackout means the last entry is
	// structurally 0/N in a legal match; RoundActions does not check
	// in-game legality any more than Match does (see fixture_test.go's own
	// note), so a caller feeding it an illegal log would see whatever the
	// log actually says, not a rejection.
	LedgerPurchasesByRound []Rate

	// ActionFrequency is GDD §22 per-action: "Action selection frequency.
	// Any action chosen in under 3% of turns is dead weight and should be
	// cut or buffed — the current suspects are Surveil and Vanish." One
	// entry per game.ActionKind, N is player-rounds for every entry,
	// including game.ActionNothing: GDD §9.2/§15.0 makes "no action" a
	// real, always-legal choice (also the degradation fallback), so
	// "turns" is read as every order's own declared action, not only the
	// orders that acted.
	ActionFrequency map[game.ActionKind]Rate

	// ItemPurchaseFrequency is GDD §22 per-action: "Item purchase
	// frequency by item. Same threshold." "Same threshold" is read here as
	// the same population, too: N is player-rounds, identical to
	// ActionFrequency, not narrowed to orders that chose game.ActionDeal —
	// an item bought in under 3% of all turns is exactly as dead as an
	// underused action, and GDD §22 asks that question, not "3% of
	// purchases." One entry per game.ItemID, meaningful only on an order
	// whose Action.Kind is game.ActionDeal (game.ActionOrder's own doc
	// comment).
	ItemPurchaseFrequency map[game.ItemID]Rate
}

// RoundActions computes GDD §22's per-round and per-action metric set from
// one match's order log — s and cfg supply the player and round counts
// every field's denominator needs, the same three-input shape Match uses,
// but RoundActions needs no event stream: every row it answers is order-log
// shaped (D33 never audited these rows, since it scoped itself to the
// twenty per-match rows; this package's own doc comment above records why
// the order log alone answers all five here).
//
// # Fails closed
//
// Issue #198's own acceptance criterion: "a frequency over a zero
// denominator is an error, not 0%. An action chosen 0 times out of 0 turns
// clears §22's 3% cut line and would read as dead weight." RoundActions
// enforces this structurally rather than row by row: cfg.Rounds, the
// player count and a completed match (s.Round reaching cfg.Rounds) are
// checked up front, exactly as Match checks them, which makes
// player-rounds — every field's denominator here — provably positive for
// the rest of this function. There is no per-row Rate{} escape hatch the
// way Match's own rows have for a legitimately empty population (D35):
// every order in a completed match declares a real stance and a real
// action, so none of these five populations can be empty short of the
// structural failures already checked.
//
// The same posture applies to what the fold accepts, not only the
// preconditions: a round key outside 1..cfg.Rounds, or a Stance/
// ActionKind/ItemID this package does not enumerate in allStances/
// allActions/allItems, is rejected rather than silently counted into one
// field's total and dropped from another's — a caller-supplied log with
// an out-of-range round previously left LedgerPurchaseRate and
// LedgerPurchasesByRound disagreeing about the same purchases, the exact
// "looks like an answer but isn't" failure this package's own doc comment
// warns against elsewhere.
func RoundActions(s rules.MatchState, log rules.OrderLog, cfg game.Config) (RoundActionSummary, error) {
	if cfg.Rounds <= 0 {
		return RoundActionSummary{}, errors.New("telemetry: RoundActions: cfg.Rounds is zero")
	}
	if s.Round < game.RoundNumber(cfg.Rounds) {
		return RoundActionSummary{}, fmt.Errorf("telemetry: RoundActions: match reached round %d, cfg.Rounds is %d — the match did not finish", s.Round, cfg.Rounds)
	}
	if len(s.Players) == 0 {
		return RoundActionSummary{}, errors.New("telemetry: RoundActions: MatchState has no players")
	}
	if len(log) == 0 {
		return RoundActionSummary{}, errors.New("telemetry: RoundActions: OrderLog has no entries — no order was ever submitted")
	}

	playerRounds := len(s.Players) * cfg.Rounds

	stanceCounts := make(map[game.Stance]int, len(allStances))
	actionCounts := make(map[game.ActionKind]int, len(allActions))
	itemCounts := make(map[game.ItemID]int, len(allItems))
	ledgerTotal := 0
	ledgerByRound := make([]int, cfg.Rounds)

	for round, orders := range log {
		idx := int(round) - 1
		if idx < 0 || idx >= cfg.Rounds {
			return RoundActionSummary{}, fmt.Errorf("telemetry: RoundActions: OrderLog has round %d, outside 1-%d", round, cfg.Rounds)
		}
		for seat, order := range orders {
			if !slices.Contains(allStances, order.Stance.Stance) {
				return RoundActionSummary{}, fmt.Errorf("telemetry: RoundActions: round %d seat %d declares an invalid Stance %v", round, seat, order.Stance.Stance)
			}
			if !slices.Contains(allActions, order.Action.Kind) {
				return RoundActionSummary{}, fmt.Errorf("telemetry: RoundActions: round %d seat %d declares an invalid Action %v", round, seat, order.Action.Kind)
			}
			if order.Action.Kind == game.ActionDeal && !slices.Contains(allItems, order.Action.Item) {
				return RoundActionSummary{}, fmt.Errorf("telemetry: RoundActions: round %d seat %d declares Deal with an invalid Item %v", round, seat, order.Action.Item)
			}

			stanceCounts[order.Stance.Stance]++
			actionCounts[order.Action.Kind]++
			if order.Action.Kind == game.ActionDeal {
				itemCounts[order.Action.Item]++
			}
			if order.AddOns.BuyLedger {
				ledgerTotal++
				ledgerByRound[idx]++
			}
		}
	}

	stances := make(map[game.Stance]Rate, len(allStances))
	for _, st := range allStances {
		stances[st] = Rate{Value: float64(stanceCounts[st]) / float64(playerRounds), N: playerRounds}
	}

	actions := make(map[game.ActionKind]Rate, len(allActions))
	for _, a := range allActions {
		actions[a] = Rate{Value: float64(actionCounts[a]) / float64(playerRounds), N: playerRounds}
	}

	items := make(map[game.ItemID]Rate, len(allItems))
	for _, it := range allItems {
		items[it] = Rate{Value: float64(itemCounts[it]) / float64(playerRounds), N: playerRounds}
	}

	byRound := make([]Rate, cfg.Rounds)
	for i, n := range ledgerByRound {
		byRound[i] = Rate{Value: float64(n) / float64(len(s.Players)), N: len(s.Players)}
	}

	return RoundActionSummary{
		StanceDistribution:     stances,
		LedgerPurchaseRate:     Rate{Value: float64(ledgerTotal) / float64(playerRounds), N: playerRounds},
		LedgerPurchasesByRound: byRound,
		ActionFrequency:        actions,
		ItemPurchaseFrequency:  items,
	}, nil
}

// ActionFrequencyGap computes, for every game.ActionKind, how much more
// often b chose it than a — issue #198's own acceptance criterion made
// callable instead of something "the reader computes by opening two
// CSVs": "An action that Drifter picks 8% of the time and Operator picks
// 0% of the time is an action a competent player rejects — which is a
// stronger signal about dead weight than either number alone, and it is
// only available because the harness runs both tiers." a and b are
// ordinarily two RoundActionSummary.ActionFrequency maps already pooled
// across a whole sweep — one bot tier each (D35's per-configuration
// reduction, done by the caller: RFC §16.4 is explicit that "the
// reduction happens once per configuration," not inside this package) —
// but the arithmetic reads the same at the scale of one match's own
// ActionFrequency. This package names neither Drifter nor Operator: D34
// fixed internal/telemetry's import boundary at internal/rules, and
// internal/bots must stay out of it, so the caller supplies both maps and
// labels which is which — cmd/simulate calling
// ActionFrequencyGap(drifter.ActionFrequency, operator.ActionFrequency) is
// the expected shape. A key absent from a or b reads as Go's zero Rate
// (Value 0), the same as never having chosen that action at all — the
// gap it produces is just the other map's own value, not a missing entry
// or a panic.
func ActionFrequencyGap(a, b map[game.ActionKind]Rate) map[game.ActionKind]float64 {
	gap := make(map[game.ActionKind]float64, len(allActions))
	for _, k := range allActions {
		gap[k] = b[k].Value - a[k].Value
	}
	return gap
}
