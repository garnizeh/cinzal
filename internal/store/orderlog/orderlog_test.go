package orderlog

import (
	"fmt"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
)

// This file unit-tests Decode directly — the decode/group/gap-check logic
// Load wraps around a real database call — against hand-built rows, with no
// Postgres involved. The full pipeline (AppendOrder -> Load against real
// Postgres, including the resubmission-upsert and human-flips-source
// acceptance criteria) is internal/store's own
// //go:build integration suite (issue #317's D46-governed layer).

const matchID = game.MatchID("11111111-1111-7111-8111-111111111111")

func row(round game.RoundNumber, seat game.SeatID, payload string) store.Order {
	return store.Order{
		MatchID: matchID,
		Round:   round,
		Seat:    seat,
		Payload: []byte(payload),
		Source:  "human",
	}
}

// minimalOrder is a syntactically complete Order payload — every field
// D44/D47 gave a wire name, all at their omitted/zero/nil ordinary values.
const minimalOrder = `{"round":1,"route":[]}`

// minimalOrderForRound builds a syntactically complete Order payload whose
// own Round field is r, for fixtures that need several distinct rounds
// (each row's payload must agree with the row's own round now that
// Decode rejects a mismatch — see TestDecodeRejectsRoundMismatch
// below) but don't need a named constant per round.
func minimalOrderForRound(r game.RoundNumber) string {
	return fmt.Sprintf(`{"round":%d,"route":[]}`, r)
}

func TestDecodeGroupsByRoundAndSeat(t *testing.T) {
	rows := []store.Order{
		row(1, 0, minimalOrder),
		row(1, 1, minimalOrder),
		row(2, 0, minimalOrderForRound(2)),
	}

	log, err := Decode(matchID, rows)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(log) != 2 {
		t.Fatalf("len(log) = %d, want 2 rounds", len(log))
	}
	if len(log[1]) != 2 {
		t.Fatalf("len(log[1]) = %d, want 2 seats", len(log[1]))
	}
	if len(log[2]) != 1 {
		t.Fatalf("len(log[2]) = %d, want 1 seat", len(log[2]))
	}
	if _, ok := log[1][0]; !ok {
		t.Error("log[1][0] missing")
	}
	if _, ok := log[1][1]; !ok {
		t.Error("log[1][1] missing")
	}
}

func TestDecodeEmptyRowsIsEmptyLogNoError(t *testing.T) {
	log, err := Decode(matchID, nil)
	if err != nil {
		t.Fatalf("Decode(nil): %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("len(log) = %d, want 0 (a fresh match has no gap)", len(log))
	}
}

// TestDecodeRejectsRoundGap is issue #317's own acceptance criterion:
// "OrderLog returns rounds in a form that folds in order; a match with a
// gap in its rounds (round 3 missing) is an error, not a silently short
// fold."
func TestDecodeRejectsRoundGap(t *testing.T) {
	rows := []store.Order{
		row(1, 0, minimalOrder),
		row(2, 0, minimalOrderForRound(2)),
		// round 3 entirely missing
		row(4, 0, minimalOrderForRound(4)),
	}

	_, err := Decode(matchID, rows)
	if err == nil {
		t.Fatal("Decode with round 3 missing returned nil error, want a gap error")
	}
}

func TestDecodeSingleRoundNoGap(t *testing.T) {
	rows := []store.Order{row(1, 0, minimalOrder)}
	if _, err := Decode(matchID, rows); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

// TestDecodeRejectsRoundZero is a CodeRabbit review finding on PR #393
// (issue #317): checkNoRoundGap's gap scan only walks [1, maxRound], so a
// round-0 row sitting alongside an otherwise gapless 1..maxRound run would
// pass unnoticed if round < 1 were not rejected outright. game.RoundNumber
// is 1-indexed (GDD §4); round 0 is never valid, gap or no gap.
func TestDecodeRejectsRoundZero(t *testing.T) {
	rows := []store.Order{
		row(0, 0, minimalOrderForRound(0)),
		row(1, 0, minimalOrder),
		row(2, 0, minimalOrderForRound(2)),
	}

	_, err := Decode(matchID, rows)
	if err == nil {
		t.Fatal("Decode with a round-0 row returned nil error, want a rejection (rounds are 1-indexed)")
	}
}

// TestDecodeFieldsCorrectly proves the decoded Order actually
// carries the payload's real values through, not just that no error
// occurred.
func TestDecodeFieldsCorrectly(t *testing.T) {
	const payload = `{"round":1,"route":[1,2,3],"action":{"kind":"deliver"},"stance":{"stance":"neutral","stake":0}}`
	rows := []store.Order{row(1, 2, payload)}

	log, err := Decode(matchID, rows)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	got := log[1][2]
	want := game.Order{
		Round:  1,
		Route:  []game.NodeID{1, 2, 3},
		Action: game.ActionOrder{Kind: game.ActionDeliver},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	if !got.Equal(want) {
		t.Fatalf("decoded order = %+v, want %+v", got, want)
	}
}

// TestDecodeRejectsUnknownField is D44's corruption guard for orders:
// DisallowUnknownFields catches a stray key rather than silently dropping
// it.
func TestDecodeRejectsUnknownField(t *testing.T) {
	rows := []store.Order{row(1, 0, `{"round":1,"route":[],"totally_unknown_field":true}`)}
	if _, err := Decode(matchID, rows); err == nil {
		t.Fatal("Decode with an unknown JSON field returned nil error, want a decode error")
	}
}

// TestDecodeRejectsMalformedPayload asserts a genuinely corrupt payload
// (not valid JSON at all) fails rather than decoding to a zero Order.
func TestDecodeRejectsMalformedPayload(t *testing.T) {
	rows := []store.Order{row(1, 0, `{not json`)}
	if _, err := Decode(matchID, rows); err == nil {
		t.Fatal("Decode with malformed JSON returned nil error, want a decode error")
	}
}

// TestDecodeRejectsTrailingData is a CodeRabbit review finding on PR
// #393: json.Decoder.Decode returns as soon as it has read one complete
// JSON value and never checks whether the stream holds more after it, so a
// payload of a valid order object followed by a second top-level JSON
// value must not decode successfully with the trailing value silently
// dropped.
func TestDecodeRejectsTrailingData(t *testing.T) {
	rows := []store.Order{row(1, 0, `{"round":1,"route":[]}{"unexpected":"trailing value"}`)}
	if _, err := Decode(matchID, rows); err == nil {
		t.Fatal("Decode with trailing JSON after the order returned nil error, want a rejection")
	}
}

// TestDecodeRejectsRoundMismatch is a CodeRabbit review finding on PR
// #393: Decode decoded a payload's own Round field but never checked it
// against the row it was actually stored under, so a payload claiming
// round 1 while its row was written under round 2 would decode cleanly and
// land in log[2] carrying a value that says it's for round 1 —
// TestDecodeGroupsByRoundAndSeat above had exactly this shape by
// accident before this test (and that fixture) were added.
func TestDecodeRejectsRoundMismatch(t *testing.T) {
	rows := []store.Order{
		// row stored under round 2, payload claims round 1.
		row(2, 0, minimalOrder),
	}

	_, err := Decode(matchID, rows)
	if err == nil {
		t.Fatal("Decode with a payload round that disagrees with its row's round returned nil error, want a rejection")
	}
}

// TestDecodeRejectsDuplicateRoundSeat is a CodeRabbit review finding on PR
// #404: two rows claiming the same (round, seat) used to overwrite the
// earlier payload silently, in map-assignment order, with nothing recording
// that a duplicate existed — reachable both from a corrupted database read
// and from cmd/replay's offline --bundle path, which reshapes its own rows
// into []store.Order and calls this same Decode. A duplicate is rejected
// the same way TestDecodeRejectsRoundMismatch's mismatch is: loudly, before
// either payload's data reaches a fold.
func TestDecodeRejectsDuplicateRoundSeat(t *testing.T) {
	rows := []store.Order{
		row(1, 0, minimalOrder),
		row(1, 0, minimalOrder), // duplicate (round 1, seat 0)
	}

	_, err := Decode(matchID, rows)
	if err == nil {
		t.Fatal("Decode with a duplicate (round, seat) row returned nil error, want a rejection")
	}
}

// TestDecodeFreshOrderPerRow guards D47's own precondition: an absent key
// in one row must never read as a prior row's value. Row for seat 0 sets a
// non-zero Action.Kind; row for seat 1 (decoded after it, same round) omits
// it entirely and must decode to the reserved-invalid zero, not seat 0's
// Deal.
func TestDecodeFreshOrderPerRow(t *testing.T) {
	rows := []store.Order{
		row(1, 0, `{"round":1,"route":[],"action":{"kind":"deal","item":"shiv"}}`),
		row(1, 1, `{"round":1,"route":[]}`),
	}

	log, err := Decode(matchID, rows)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if log[1][1].Action.Kind != 0 {
		t.Fatalf("seat 1's Action.Kind = %v, want the reserved-invalid zero (no stale value from seat 0's row)", log[1][1].Action.Kind)
	}
	if log[1][0].Action.Kind != game.ActionDeal {
		t.Fatalf("seat 0's Action.Kind = %v, want ActionDeal", log[1][0].Action.Kind)
	}
}
