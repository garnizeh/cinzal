// Package store holds sqlc-generated queries, the repositories around them, and
// the goose migrations, embedded with embed.FS and run at process start behind
// an advisory lock.
//
// The order log is the only source of truth. Match state is derived by folding
// it and is never stored: state = fold(Resolve, initial(seed, cfg), orderLog).
// The events and match_summary tables are rebuildable projections kept for
// cheap reads, and nothing may treat them as authority (RFC-001 §7.1–7.3).
//
// This matters operationally as well as architecturally: lose the order log and
// matches cannot be reconstructed, because there is no state table to fall back
// on (RFC-001 §18).
package store
