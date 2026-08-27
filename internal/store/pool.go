package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmptyDSN is returned by Open when cfg.DSN is empty or all whitespace.
//
// pgxpool.ParseConfig does not error on an empty connection string — it
// falls through to Postgres's own default-host resolution (a unix socket
// directory, or "localhost"), which is exactly the fallback DATABASE_URL
// RFC-001 §18 rules out: "There is no default DSN, no localhost fallback,
// and no 'run without a database' mode." Open therefore rejects an empty
// DSN itself, before pgxpool ever sees it.
var ErrEmptyDSN = errors.New("store: DSN is empty")

// ErrDSNMissingHost is returned by Open when cfg.DSN is not a
// "postgres://" or "postgresql://" URL naming an explicit host, or when it
// names a libpq service/servicefile.
//
// Verified directly against pgx v5.10.0 (internal/store/probe_dsn_test.go
// during review, not committed): with PGHOST set in the environment,
// pgxpool.ParseConfig("postgres:///cinzal") and even
// ParseConfig("postgres://user:pass@/cinzal") both silently resolve to
// PGHOST's value — a non-empty but hostless DSN reaches the exact ambient
// fallback ErrEmptyDSN's own doc comment describes, which a bare
// "is cfg.DSN empty" check does not catch. A "service=" (or "servicefile=")
// parameter is the same fallback reached a different way: pgx reads a
// service file and can supply the host, and every other connection detail,
// from outside the DSN entirely. RFC-001 §18 never names any form but
// DATABASE_URL, so a non-URL (keyword/value) DSN is rejected outright here
// rather than hand-parsing libpq's keyword grammar to look for an explicit
// host= this project has no use for.
var ErrDSNMissingHost = errors.New("store: DSN has no explicit host")

// The RFC-001 §8.3 numbers this package makes available as named defaults,
// each quoted verbatim from its own code sample. None of these are applied
// by Open automatically — a zero-valued Config field means "let pgxpool
// decide," so a caller opts a pool into the sweeper's shape explicitly
// rather than the app pool inheriting sweeper-only settings by accident.
const (
	// DefaultAppMaxConns is RFC-001 §8.3's app pool: "MaxConns 20 — handlers".
	DefaultAppMaxConns int32 = 20

	// DefaultSweeperMaxConns is RFC-001 §8.3's sweeper pool: "MaxConns 3 — sweeper only".
	DefaultSweeperMaxConns int32 = 3

	// DefaultLockTimeout is RFC-001 §8.3's sweeper session setting:
	// "never queue behind a long submit txn".
	DefaultLockTimeout = 2 * time.Second

	// DefaultStatementTimeout is RFC-001 §8.3's sweeper session setting:
	// "a wedged tick must not hold the row lock".
	DefaultStatementTimeout = 10 * time.Second

	// DefaultIdleInTransactionSessionTimeout is RFC-001 §8.3's sweeper
	// session setting (no accompanying comment in the RFC's own sample
	// beyond the value itself).
	DefaultIdleInTransactionSessionTimeout = 15 * time.Second
)

// Config configures one pgx connection pool. RFC-001 §8.3 requires more than
// one pool — the app pool and the deadline sweeper's own pool must never
// share connections — so every field here is a Config value rather than a
// package constant, and Open may be called as many times as the caller needs
// distinct pools.
//
// A zero-valued field leaves the corresponding pgxpool.Config field unset,
// which means "let pgxpool decide" for pool sizing and connection lifetime,
// and "do not SET this session parameter" for the three timeouts. The RFC
// gives no number for MaxConnLifetime, MaxConnIdleTime or HealthCheckPeriod
// anywhere, so this package defines no named default for them; MaxConns and
// the three session timeouts do have RFC-001 §8.3 numbers, exported above.
type Config struct {
	// DSN is the only connection input (RFC-001 §18: DATABASE_URL is the
	// entire env-var surface for this package). Required; Open rejects an
	// empty or whitespace-only value with ErrEmptyDSN.
	DSN string

	MaxConns int32
	MinConns int32

	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration

	// LockTimeout, StatementTimeout and IdleInTransactionSessionTimeout are
	// RFC-001 §8.3's sweeper session settings. Each is applied via an
	// AfterConnect SET statement when non-zero, matching the RFC's own code
	// sample literally — RuntimeParams is deliberately not used here, unlike
	// the timezone pin below, because D55 states that contrast explicitly.
	LockTimeout                     time.Duration
	StatementTimeout                time.Duration
	IdleInTransactionSessionTimeout time.Duration
}

// Store owns one pgx connection pool and its lifecycle. It knows nothing
// about matches, orders, or any other game concept — repositories arrive in
// later tasks (#315 onward); this is a pool and nothing else.
type Store struct {
	pool *pgxpool.Pool
}

// Open parses cfg, verifies connectivity, and returns a ready Store. A
// *Store handed back to a caller has already talked to the database — Open
// calls Ping before returning, so a DSN pointing at a closed port fails here
// rather than at the first query.
//
// Open never falls back to a default DSN (RFC-001 §18) and may be called
// more than once with different Config values to build independent pools —
// the app pool and the deadline sweeper's own pool (RFC-001 §8.3) are two
// such calls, not two branches of one.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, ErrEmptyDSN
	}
	if err := requireExplicitHost(dsn); err != nil {
		return nil, err
	}

	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}

	if cfg.MaxConns != 0 {
		pgxCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns != 0 {
		pgxCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime != 0 {
		pgxCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime != 0 {
		pgxCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.HealthCheckPeriod != 0 {
		pgxCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	// RFC-001 §7.5 / D55: pin the session timezone to UTC in the startup
	// packet rather than an AfterConnect round trip. RuntimeParams lives on
	// the nested pgconn.Config, not on pgxpool.Config itself.
	pgxCfg.ConnConfig.RuntimeParams["timezone"] = "UTC"

	stmts, err := sessionTimeoutStatements(cfg)
	if err != nil {
		return nil, err
	}
	if len(stmts) > 0 {
		pgxCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			// One Exec per statement, not one joined string: pgx's default
			// QueryExecMode prepares the query (extended protocol), and
			// Postgres's Parse message rejects a prepared statement
			// containing more than one SQL command.
			for _, stmt := range stmts {
				if _, err := conn.Exec(ctx, stmt); err != nil {
					return fmt.Errorf("store: session setting %q: %w", stmt, err)
				}
			}
			return nil
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("store: new pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: connect: %w", err)
	}

	return &Store{pool: pool}, nil
}

// requireExplicitHost rejects a DSN that is not a "postgres://"/"postgresql://"
// URL naming an explicit host, or that names a libpq service/servicefile —
// see ErrDSNMissingHost's doc comment for why both are the same ambient
// fallback RFC-001 §18 rules out. dsn is assumed already trimmed and
// non-empty.
func requireExplicitHost(dsn string) error {
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "service=") || strings.Contains(lower, "servicefile=") {
		return ErrDSNMissingHost
	}
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return ErrDSNMissingHost
	}

	u, err := url.Parse(dsn)
	if err != nil {
		// Malformed as a URL; let pgxpool.ParseConfig produce the real,
		// more specific parse error instead of masking it here.
		return nil
	}
	if u.Hostname() == "" {
		return ErrDSNMissingHost
	}
	return nil
}

// ErrInvalidTimeout is returned by Open when a non-zero LockTimeout,
// StatementTimeout or IdleInTransactionSessionTimeout is not a whole,
// positive number of milliseconds.
//
// Two failure shapes, both caught here rather than at connection time: a
// negative duration produces an invalid Postgres setting, and — the
// non-obvious one — a positive but sub-millisecond duration such as
// 500*time.Microsecond passes a bare "!= 0" check and then silently
// truncates to 0 under time.Duration.Milliseconds(), producing
// "SET lock_timeout = '0ms'" — which *disables* the timeout instead of
// setting it, the opposite of what a non-zero Config field asked for.
var ErrInvalidTimeout = errors.New("store: timeout must be zero (disabled) or at least 1ms")

// sessionTimeoutStatements builds one RFC-001 §8.3 SET statement per
// non-zero timeout field in cfg, in a fixed order. An empty, nil-error
// result means Open installs no AfterConnect hook at all.
func sessionTimeoutStatements(cfg Config) ([]string, error) {
	settings := []struct {
		name string
		d    time.Duration
	}{
		{"lock_timeout", cfg.LockTimeout},
		{"statement_timeout", cfg.StatementTimeout},
		{"idle_in_transaction_session_timeout", cfg.IdleInTransactionSessionTimeout},
	}

	var stmts []string
	for _, s := range settings {
		if s.d == 0 {
			continue
		}
		if s.d < time.Millisecond {
			return nil, fmt.Errorf("%w: %s = %s", ErrInvalidTimeout, s.name, s.d)
		}
		stmts = append(stmts, fmt.Sprintf("SET %s = '%dms'", s.name, s.d.Milliseconds()))
	}
	return stmts, nil
}

// Close closes the pool and releases every connection. It is safe to call on
// a nil *Store and safe to call more than once — pgxpool.Pool.Close is
// itself documented idempotent, and a caller that unconditionally defers
// Close after a failed Open (which returns a nil *Store) must not panic.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// Ping verifies the pool can still reach the database.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
