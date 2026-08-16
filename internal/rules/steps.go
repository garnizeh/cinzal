package rules

import "github.com/garnizeh/cinzal/internal/game"

// infamyTierIndex maps an Infamy score (0-10) to Config.StepsByTier's index
// (0-3), built on TierOf (infamy.go) rather than its own copy of the
// boundary table. It is deliberately not game.InfamyTier's own ordinal:
// that enum reserves 0 as an invalid sentinel (its constants start at 1),
// while Config.StepsByTier is documented and indexed 0=Nobody..3=Legend —
// using int(game.TierNobody) here would silently read the wrong array slot.
//
// Under cfg.Suppress.InfamyTiers (D11), this always returns Nobody's index:
// the single early return that pins every ladder-derived effect — step
// base, combat bonus, Contact Cooldown — to the Nobody row regardless of
// the seat's actual Infamy. GDD §11's Gains and Losses table is unmodified
// by this: Infamy itself keeps accumulating normally, only this lookup is
// gated.
func infamyTierIndex(infamy int, cfg game.Config) int {
	if cfg.Suppress.InfamyTiers {
		return int(game.TierNobody) - 1
	}
	return int(TierOf(infamy)) - 1
}

// Steps computes this round's step allowance, GDD §9.1a's exact order of
// operations: every modifier sums against the Infamy-tier base, the floor of
// 1 is applied exactly once at the end, and only then does a hard cap (GDD
// §14.3's Streets Blocked) apply.
//
// v.You.StepModifiers must be the entry-snapshot-frozen values (RFC §6.6) —
// Steps never reads v.You.StepAllowance, and it trusts whatever Infamy and
// StepModifiers the caller put on v. A view built from live, mid-round state
// rather than the entry snapshot will silently produce the wrong answer;
// that discipline is the caller's (see internal/rules/steps_test.go's
// entry-snapshot case).
func Steps(v game.PlayerView, cfg game.Config) int {
	steps := cfg.StepsByTier[infamyTierIndex(v.You.Infamy, cfg)]

	m := v.You.StepModifiers
	if m.Flagged {
		steps--
	}
	if m.Curfew {
		steps--
	}
	if m.EvasiveStepPenalty {
		steps--
	}
	if m.DistractedGuard {
		steps++
	}
	if m.Scaffolding {
		steps++
	}
	if m.Retainer {
		steps += 2
	}

	steps = max(1, steps)

	if m.StreetsBlocked {
		steps = min(steps, 1)
	}

	return steps
}
