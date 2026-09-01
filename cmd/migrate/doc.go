// Command migrate applies internal/store's embedded goose migrations to a
// database, reporting how many it applied.
//
//	migrate --db $DATABASE_URL
//
// This is `make db-migrate`'s implementation (issue #326) — the local-dev
// counterpart to RFC-001 §18's "start: migrate (advisory lock) → serve" boot
// step, standing on its own because cmd/server, the eventual owner of that
// boot sequence, is still doc.go (RFC §21's build order defers it to M5).
// It wraps store.Migrate() rather than shelling out to the bare goose CLI
// against internal/store/migrations directly: store.Migrate() is already
// the one implementation of "how a migration gets applied" — the advisory
// lock, the real goose.Provider — that production boot, storetest's setup,
// and every Concurrency test already share, and a second, parallel path
// through the same .sql files would be exactly the kind of duplicate
// implementation RFC §7.4 argues against elsewhere (cmd/replay reuses
// internal/match/fold.FoldThrough rather than re-deriving state).
//
// Fails closed on the case a plain "ok, nothing to do" would hide: if the
// database had no migrations applied before this run (goose_db_version
// absent or empty — a genuinely fresh database) and this run still applied
// none, that can only mean internal/store/migrations itself had nothing to
// apply, which is a bug, not a legitimate no-op — see run.go's
// evaluateMigration. Re-running against an already-migrated database
// (something applied before, nothing new to apply) is the ordinary
// idempotent case and exits 0.
package main
