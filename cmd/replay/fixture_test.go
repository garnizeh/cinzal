package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
)

// testFixture returns a small, fully valid 2-player match: DefaultConfig,
// a fixed seed, and a complete order log of idle orders (no route, no
// action, Neutral stance) for every one of cfg.Rounds rounds — the same
// shape internal/match/fold's own fold_test.go fixtures use. Idle orders
// keep every seat at its starting position for the whole match, which is
// exactly what the --seat fog-negative test (run_test.go) needs: with no
// movement, most of the map stays outside each seat's own discovered set.
func testFixture() (cfg game.Config, seed [32]byte, players int, log rules.OrderLog) {
	cfg = game.DefaultConfig()
	seed = [32]byte{9, 9, 9}
	players = 2

	idle := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	log = rules.OrderLog{}
	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := 0; seat < players; seat++ {
			o := idle
			o.Round = game.RoundNumber(round)
			orders[game.SeatID(seat)] = o
		}
		log[game.RoundNumber(round)] = orders
	}

	return cfg, seed, players, log
}

// writeTestBundle assembles testFixture's own (cfg, seed, players, log) into
// a Bundle and writes it to a fresh file under t.TempDir(), returning the
// path — the --bundle input every non-integration cmd/replay test drives
// against, with no database involved.
func writeTestBundle(t *testing.T) string {
	t.Helper()

	cfg, seed, players, log := testFixture()
	_ = players // player count is derived by readBundle itself from the log

	configEnvelope, err := store.EncodeConfig(cfg)
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}

	var orderLog []BundleOrder
	for round := game.RoundNumber(1); round <= game.RoundNumber(cfg.Rounds); round++ {
		orders := log[round]
		for seat := game.SeatID(0); int(seat) < len(orders); seat++ {
			payload, err := json.Marshal(orders[seat])
			if err != nil {
				t.Fatalf("marshal order (round %d, seat %d): %v", round, seat, err)
			}
			orderLog = append(orderLog, BundleOrder{Round: round, Seat: seat, Payload: payload})
		}
	}

	b := Bundle{Seed: seed[:], Config: configEnvelope, Players: players, OrderLog: orderLog}

	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := writeBundle(path, b); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}
	return path
}

// mustReadFile reads path or fails the test.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
