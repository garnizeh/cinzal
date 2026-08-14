package rules

import "testing"

// TestFinalScoreCombinesAllFiveSources is GDD §16's table, worked by hand:
// prior RP (delivered contracts and anything else earned over the match),
// 2 live posts in 2 different sectors (each won outright — the fixture's
// only player has no rival to tie or beat), Cr$17 (truncating to 4 RP, not
// 4.25), and one active undelivered contract.
func TestFinalScoreCombinesAllFiveSources(t *testing.T) {
	s := sectorMajorityFixture(3, 1)
	s.Graph.Nodes[0].Post = &Post{Owner: 0} // Old Docks: seat 0 sole owner
	s.Graph.Nodes[3].Post = &Post{Owner: 0} // Iron Low: a second live post
	s.Players[0].RP = 6
	s.Players[0].Balance = 17
	s.Players[0].Contracts = []Contract{{ID: 0}}

	got := FinalScore(s)
	if len(got) != 1 {
		t.Fatalf("FinalScore() returned %d breakdowns, want 1", len(got))
	}
	b := got[0]

	want := FinalScoreBreakdown{
		Seat:       0,
		PriorRP:    6,
		PostsRP:    4,  // 2 live posts * 2 RP
		MajorityRP: 6,  // Old Docks and Iron Low, both won outright
		CashRP:     4,  // 17 / 4, truncating
		PenaltyRP:  -2, // one active undelivered contract
		Total:      18,
	}
	if b != want {
		t.Fatalf("FinalScore()[0] = %+v, want %+v", b, want)
	}
}

// TestFinalScoreSectorMajorityTieScoresNobody is GDD §16 verbatim, in the
// final-scoring path rather than SectorMajority directly: a sector split
// evenly between two seats contributes MajorityRP 0 to both.
func TestFinalScoreSectorMajorityTieScoresNobody(t *testing.T) {
	s := sectorMajorityFixture(2, 2)
	s.Graph.Nodes[0].Post = &Post{Owner: 0}
	s.Graph.Nodes[1].Post = &Post{Owner: 1}

	got := FinalScore(s)
	for _, b := range got {
		if b.MajorityRP != 0 {
			t.Errorf("seat %d: MajorityRP = %d, want 0 (tied sector scores nobody)", b.Seat, b.MajorityRP)
		}
	}
}

// TestFinalScorePenaltyFiresOnAnAcceptedButNeverPickedUpContract is issue
// #76's own acceptance criterion, verbatim: "including one held but never
// picked up" — membership in Contracts is the trigger, not Cargo state.
func TestFinalScorePenaltyFiresOnAnAcceptedButNeverPickedUpContract(t *testing.T) {
	s := sectorMajorityFixture(3, 1)
	s.Players[0].Contracts = []Contract{{ID: 0}}
	s.Players[0].Cargo = nil // never picked up

	got := FinalScore(s)
	if got[0].PenaltyRP != activeContractPenaltyRP {
		t.Fatalf("PenaltyRP = %d, want %d", got[0].PenaltyRP, activeContractPenaltyRP)
	}
}

// TestFinalScorePenaltyScalesWithContractCount checks the penalty is
// per-contract, not a flat one-time deduction — GDD §16 says "each".
func TestFinalScorePenaltyScalesWithContractCount(t *testing.T) {
	s := sectorMajorityFixture(3, 1)
	s.Players[0].Contracts = []Contract{{ID: 0}, {ID: 1}}

	got := FinalScore(s)
	if want := 2 * activeContractPenaltyRP; got[0].PenaltyRP != want {
		t.Fatalf("PenaltyRP = %d, want %d (two active contracts)", got[0].PenaltyRP, want)
	}
}

// TestFinalScoreCashConversionTruncates is GDD §16 verbatim: "1 per Cr$4,
// rounded down" — integer division, not the nearest RP.
func TestFinalScoreCashConversionTruncates(t *testing.T) {
	cases := []struct {
		balance int
		wantRP  int
	}{
		{balance: 0, wantRP: 0},
		{balance: 3, wantRP: 0},
		{balance: 4, wantRP: 1},
		{balance: 17, wantRP: 4},
		{balance: 19, wantRP: 4},
		{balance: 20, wantRP: 5},
	}
	for _, tt := range cases {
		s := sectorMajorityFixture(3, 1)
		s.Players[0].Balance = tt.balance

		got := FinalScore(s)
		if got[0].CashRP != tt.wantRP {
			t.Errorf("balance=%d: CashRP = %d, want %d", tt.balance, got[0].CashRP, tt.wantRP)
		}
	}
}

// TestFinalScoreRanksTotalDescendingWithTiebreak is the end-to-end path
// through FinalScore itself (rankFinalScores' own unit tests in
// ordering_test.go cover the mechanics in isolation): the returned slice
// is match placement, not seat order, and a Total tie falls through to
// GDD §16's tiebreak chain.
func TestFinalScoreRanksTotalDescendingWithTiebreak(t *testing.T) {
	s := sectorMajorityFixture(3, 2)
	s.Players[0].RP = 10
	s.Players[0].ContractsDelivered = 1
	s.Players[1].RP = 10
	s.Players[1].ContractsDelivered = 4 // same Total, wins the tiebreak

	got := FinalScore(s)
	if len(got) != 2 {
		t.Fatalf("FinalScore() returned %d breakdowns, want 2", len(got))
	}
	if got[0].Seat != 1 || got[1].Seat != 0 {
		t.Fatalf("FinalScore() order = %+v, want seat 1 first (tiebreak: most delivered)", got)
	}
	if got[0].Total != got[1].Total {
		t.Fatalf("FinalScore() Totals = %d, %d, want equal (test premise)", got[0].Total, got[1].Total)
	}
}

// TestFinalScoreHigherTotalOutranksTiebreakLevels checks Total RP is
// strictly the primary key — a seat behind on every GDD §16 tiebreak level
// still places first on a higher Total.
func TestFinalScoreHigherTotalOutranksTiebreakLevels(t *testing.T) {
	s := sectorMajorityFixture(3, 2)
	s.Players[0].RP = 30
	s.Players[0].ContractsDelivered = 0
	s.Players[0].Infamy = 0
	s.Players[0].Balance = 0
	s.Players[1].RP = 5
	s.Players[1].ContractsDelivered = 9
	s.Players[1].Infamy = 9
	s.Players[1].Balance = 99

	got := FinalScore(s)
	if got[0].Total <= got[1].Total {
		t.Fatalf("FinalScore() Totals = %d, %d, want seat 0 strictly ahead (test premise)", got[0].Total, got[1].Total)
	}
	if got[0].Seat != 0 {
		t.Fatalf("FinalScore() order = %+v, want seat 0 first (higher Total)", got)
	}
}
