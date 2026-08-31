// Command replay folds a recorded match log and dumps state at a given round.
//
//	replay --db $DATABASE_URL --match <id> [--round N] [--seat N]
//	replay --bundle match.json [--round N] [--seat N]
//	replay --db $DATABASE_URL --match <id> --export-bundle match.json
//	replay --db $DATABASE_URL --match <id> --rebuild [--include-active]
//	replay --db $DATABASE_URL --all --rebuild [--include-active]
//
// It runs the same fold as the server — internal/match/fold.FoldThrough,
// the exact per-round Resolve loop internal/match's own tick (M4) will use,
// not a second implementation — with a null effect sink, so replaying a
// match never re-sends a historical notification (RFC-001 §7.4).
//
// Two input paths (issue #322). --match reads a finished match straight out
// of the database. --bundle reads RFC §15.4's own replay bundle instead —
// {seed, config, orderLog}, downloadable by a match's players and "the
// perfect bug report: attach it to an issue and cmd/replay reproduces the
// exact match" — so a bug can be reproduced on a machine with no database
// at all. --export-bundle writes that same bundle from a database match,
// straight from its rows with no re-encoding step, so the two input paths
// agree by construction.
//
// Two dump shapes. The default prints the full rules.MatchState — a
// developer's view, and deliberately not behind //go:build debug: D49
// (docs/decisions/D49-fold-package-boundary.md), decided while scoping this
// same issue, already settles that cmd/replay needs the raw MatchState "its
// default dump prints," and RFC §15.1's build-tag argument is about an
// accidental runtime god view reachable from a live server — a CLI reading
// a database it already holds credentials for discloses nothing a build
// tag would meaningfully withhold. Consistency with that discipline is kept
// by the import-graph mechanism instead: internal/match/fold is on
// scripts/check-fog-boundary.sh's own FORBIDDEN list, so nothing this
// package returns can reach internal/render or internal/web by accident,
// which is the property //go:build debug exists to guarantee elsewhere.
// --seat N prints rules.ProjectView(state, seat, cfg) instead — one seat's
// fog-filtered game.PlayerView, exactly what a player could be shown, and
// the shape a reproduced bug report actually needs.
//
// --round N dumps state as it stood after round N resolved, for any N up to
// the match's own last round — not always the terminal state Fold returns.
// See internal/match/fold.FoldThrough's own doc for why this needs its own
// parameter rather than folding a shortened cfg: Resolve, Legal and the
// incident/market/add-on rules all branch on the match's real cfg.Rounds
// for final-round behaviour, and a truncated cfg would fire those branches
// early, silently disagreeing with what the live match actually did at
// round N.
//
// --rebuild (issue #323) deletes and regenerates events and match_summary
// for --match (or every eligible match, with --all) from a fresh fold of
// the match's own order log, through the log's own highest round rather
// than always cfg.Rounds — an active match rebuilt with --include-active
// has orders only for the rounds it has actually played, and fold.Fold's
// own cfg.Rounds bound would reject that log as missing every later round.
// RFC-001 §7.2: "there is a cmd/replay --rebuild that regenerates them,"
// and, per the issue's own title, both of these two projections, not a
// third: match_players.
// last_seen_round is a derived projection too (RFC §7.2), but its own
// rebuild is out of this issue's scope: the roadmap's own M3 deliverable
// line names it, but neither #323 nor its exit demonstration (#330) does,
// and no other M3 issue claims it either — a gap between the roadmap's
// prose and the issue breakdown, filed separately as #409 rather than
// invented here.
//
// The rebuild is one transaction per match (store.RebuildProjections):
// delete both tables, write the fresh content, commit — a failure at any
// point leaves the match's original rows untouched, never a half-cleared
// projection. It writes nothing else: no outbox row, no orders row, no
// matches row — the fold it runs is the same null-sink fold as every other
// mode here (RFC §7.4), so a --rebuild over every finished match, ever, can
// never re-send a single historical notification.
//
// --all is restricted to matches whose status is "finished" unless
// --include-active is also given, which additionally allows "active"
// matches — named or listed by --all, the guard is the same either way,
// since rebuilding an active match races the round tick regardless of how
// it was reached (issue #323).
package main
