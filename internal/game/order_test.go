package game

import "testing"

func baseOrder() Order {
	sector := SectorIronLow
	choice := 1
	return Order{
		Round: 5,
		Route: []NodeID{1, 2, 3},
		PushingOn: PushingOn{
			Steps: 2,
			Bias:  &sector,
		},
		Action: ActionOrder{Kind: ActionDeal, Item: ItemShiv},
		Stance: StanceOrder{Stance: StanceAggressive, Stake: 4},
		Items: []ItemDiscard{
			{Item: ItemPoliceBand, Target: 7},
		},
		AddOns: AddOns{
			BuyLedger:   true,
			RenewPost:   9,
			RenewBlocks: 2,
		},
		AbandonCargo:   true,
		ContractChoice: &choice,
	}
}

func TestOrderEqualIdentical(t *testing.T) {
	a := baseOrder()
	b := baseOrder()

	if !a.Equal(b) {
		t.Fatalf("expected identical orders to compare equal:\na = %+v\nb = %+v", a, b)
	}
	if !b.Equal(a) {
		t.Fatalf("Equal is not symmetric")
	}
}

func TestOrderEqualNilBiasBothSides(t *testing.T) {
	a := baseOrder()
	a.PushingOn.Bias = nil
	b := baseOrder()
	b.PushingOn.Bias = nil

	if !a.Equal(b) {
		t.Fatalf("expected orders with both nil Bias to compare equal")
	}
}

func TestOrderEqualDetectsEveryFieldDifference(t *testing.T) {
	otherSector := SectorNorthVale
	otherChoice := 2

	mutations := map[string]func(o *Order){
		"ContractChoice nil":   func(o *Order) { o.ContractChoice = nil },
		"ContractChoice value": func(o *Order) { o.ContractChoice = &otherChoice },
		"Round":                func(o *Order) { o.Round++ },
		"Route length":         func(o *Order) { o.Route = append(o.Route, 4) },
		"Route contents":       func(o *Order) { o.Route[0] = 99 },
		"PushingOn.Steps":      func(o *Order) { o.PushingOn.Steps = 0 },
		"PushingOn.Bias nil":   func(o *Order) { o.PushingOn.Bias = nil },
		"PushingOn.Bias value": func(o *Order) { o.PushingOn.Bias = &otherSector },
		"Action.Kind":          func(o *Order) { o.Action.Kind = ActionNothing },
		"Action.Item":          func(o *Order) { o.Action.Item = ItemMuscle },
		"Stance.Stance":        func(o *Order) { o.Stance.Stance = StanceEvasive },
		"Stance.Stake":         func(o *Order) { o.Stance.Stake = 0 },
		"Items length":         func(o *Order) { o.Items = append(o.Items, ItemDiscard{Item: ItemDecoy, Target: 1}) },
		"Items contents":       func(o *Order) { o.Items[0].Target = 42 },
		"AddOns.BuyLedger":     func(o *Order) { o.AddOns.BuyLedger = false },
		"AddOns.RenewPost":     func(o *Order) { o.AddOns.RenewPost = 0 },
		"AddOns.RenewBlocks":   func(o *Order) { o.AddOns.RenewBlocks = 0 },
		"AbandonCargo":         func(o *Order) { o.AbandonCargo = false },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			a := baseOrder()
			b := baseOrder()
			mutate(&b)

			if a.Equal(b) {
				t.Fatalf("expected orders differing in %s to compare unequal", name)
			}
		})
	}
}

func TestOrderEqualDoesNotAliasRouteSlice(t *testing.T) {
	a := baseOrder()
	b := baseOrder()

	if !a.Equal(b) {
		t.Fatalf("precondition failed: expected a and b to start equal")
	}

	// Mutating one order's backing slice must not silently flip the other's
	// comparison — Equal must read current contents, not a shared identity.
	b.Route[0] = 123
	if a.Equal(b) {
		t.Fatalf("expected mutation of b.Route to be visible to Equal")
	}
}

func TestOrderEqualContractChoiceNilBothSides(t *testing.T) {
	a := baseOrder()
	a.ContractChoice = nil
	b := baseOrder()
	b.ContractChoice = nil

	if !a.Equal(b) {
		t.Fatalf("expected orders with both nil ContractChoice to compare equal")
	}
}

func TestOrderEqualContractChoiceNilVsNonNil(t *testing.T) {
	choice := 0
	withChoice := baseOrder()
	withChoice.ContractChoice = &choice
	withoutChoice := baseOrder()
	withoutChoice.ContractChoice = nil

	if withChoice.Equal(withoutChoice) {
		t.Fatalf("expected a declared ContractChoice to differ from none")
	}
	if withoutChoice.Equal(withChoice) {
		t.Fatalf("Equal is not symmetric for nil vs non-nil ContractChoice")
	}
}

func TestPushingOnEqualNilVsNonNil(t *testing.T) {
	sector := SectorMistHeights
	withBias := PushingOn{Steps: 1, Bias: &sector}
	withoutBias := PushingOn{Steps: 1, Bias: nil}

	if withBias.Equal(withoutBias) {
		t.Fatalf("expected a declared bias to differ from no bias")
	}
	if withoutBias.Equal(withBias) {
		t.Fatalf("Equal is not symmetric for nil vs non-nil Bias")
	}
}
