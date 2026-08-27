package store

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newUnconnectedStore builds a *Store the same way Open does, minus the
// Ping — pgxpool.NewWithConfig does not dial eagerly, so this exercises
// Store.Close's own behavior without needing a reachable database.
func newUnconnectedStore(t *testing.T) (*Store, error) {
	t.Helper()
	pgxCfg, err := pgxpool.ParseConfig("postgres://user:placeholder@127.0.0.1:1/db?sslmode=disable")
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), pgxCfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func TestOpenRejectsEmptyDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"empty", ""},
		{"whitespace only", "   \t  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(context.Background(), Config{DSN: tt.dsn})
			if s != nil {
				t.Errorf("Open(%q) returned a non-nil *Store", tt.dsn)
			}
			if !errors.Is(err, ErrEmptyDSN) {
				t.Fatalf("Open(%q) error = %v, want ErrEmptyDSN", tt.dsn, err)
			}
		})
	}
}

// TestOpenRejectsUnparseableDSN asserts an error distinct from ErrEmptyDSN —
// the two must not be conflated, since an operator debugging a malformed
// DATABASE_URL needs a different fix than one debugging an unset one.
func TestOpenRejectsUnparseableDSN(t *testing.T) {
	s, err := Open(context.Background(), Config{DSN: "not a dsn"})
	if s != nil {
		t.Fatal("Open returned a non-nil *Store for an unparseable DSN")
	}
	if err == nil {
		t.Fatal("Open returned a nil error for an unparseable DSN")
	}
	if errors.Is(err, ErrEmptyDSN) {
		t.Fatalf("Open reported ErrEmptyDSN for an unparseable (non-empty) DSN: %v", err)
	}
}

// TestOpenFailsClosedOnDeadPort is the acceptance criterion's fails-closed
// case: "A DSN pointing at a closed port fails Open, not the first query."
// Port 1 is a well-known unprivileged-inaccessible port that nothing on a
// test runner listens on, so the OS returns ECONNREFUSED immediately rather
// than timing out — the connect_timeout is still set as a backstop so this
// test cannot hang if that assumption is ever wrong in some environment.
//
// The assertion is on the error's *content*, not merely its non-nilness —
// the acceptance criterion is explicit that "context deadline exceeded"
// would pass a bare err != nil check while telling an operator the wrong
// thing about a bad password. A refused TCP connection must surface as a
// dial/connection error, distinguishable from both ErrEmptyDSN and a parse
// error.
func TestOpenFailsClosedOnDeadPort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, err := Open(ctx, Config{DSN: "postgres://user:placeholder@127.0.0.1:1/db?sslmode=disable&connect_timeout=2"})
	if s != nil {
		s.Close()
		t.Fatal("Open returned a non-nil *Store for a dead-port DSN")
	}
	if err == nil {
		t.Fatal("Open returned a nil error for a dead-port DSN")
	}
	if errors.Is(err, ErrEmptyDSN) {
		t.Fatalf("Open reported ErrEmptyDSN for a dead-port DSN: %v", err)
	}

	if _, ok := errors.AsType[*net.OpError](err); !ok {
		t.Fatalf("Open error chain contains no *net.OpError (want a dial/connect failure): %v", err)
	}
	if !strings.Contains(err.Error(), "store: connect:") {
		t.Fatalf("Open error = %q, want the store: connect: prefix distinguishing it from a parse/config error", err.Error())
	}
}

func TestStoreCloseIsNilSafe(t *testing.T) {
	var s *Store
	s.Close() // must not panic
}

func TestStoreCloseIsIdempotent(t *testing.T) {
	// A pool is constructed without dialing (pgxpool.NewWithConfig doesn't
	// connect eagerly), so this exercises Close's own idempotency without
	// needing a reachable database.
	s, err := newUnconnectedStore(t)
	if err != nil {
		t.Fatalf("newUnconnectedStore: %v", err)
	}
	s.Close()
	s.Close() // must not panic, must not error, safe to call again
}

func TestSessionTimeoutStatements(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{"none set", Config{}, nil},
		{
			"lock timeout only",
			Config{LockTimeout: 2 * time.Second},
			[]string{"SET lock_timeout = '2000ms'"},
		},
		{
			"all three, RFC-001 §8.3 defaults, one statement each — never joined into one string",
			Config{
				LockTimeout:                     DefaultLockTimeout,
				StatementTimeout:                DefaultStatementTimeout,
				IdleInTransactionSessionTimeout: DefaultIdleInTransactionSessionTimeout,
			},
			[]string{
				"SET lock_timeout = '2000ms'",
				"SET statement_timeout = '10000ms'",
				"SET idle_in_transaction_session_timeout = '15000ms'",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sessionTimeoutStatements(tt.cfg)
			if err != nil {
				t.Fatalf("sessionTimeoutStatements() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("sessionTimeoutStatements() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sessionTimeoutStatements()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSessionTimeoutStatementsRejectsInvalidDurations covers the case a bare
// "!= 0" check misses: a positive but sub-millisecond duration truncates to
// "0ms" under time.Duration.Milliseconds(), which *disables* the Postgres
// timeout instead of setting it — the opposite of what a non-zero Config
// field asked for. A negative duration is rejected the same way.
func TestSessionTimeoutStatementsRejectsInvalidDurations(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"negative lock timeout", Config{LockTimeout: -1 * time.Second}},
		{"sub-millisecond lock timeout", Config{LockTimeout: 500 * time.Microsecond}},
		{"negative statement timeout", Config{StatementTimeout: -1 * time.Millisecond}},
		{"sub-millisecond idle-in-transaction timeout", Config{IdleInTransactionSessionTimeout: 1 * time.Microsecond}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := sessionTimeoutStatements(tt.cfg)
			if !errors.Is(err, ErrInvalidTimeout) {
				t.Fatalf("sessionTimeoutStatements(%+v) error = %v, want ErrInvalidTimeout", tt.cfg, err)
			}
			if stmts != nil {
				t.Errorf("sessionTimeoutStatements(%+v) = %v, want nil on error", tt.cfg, stmts)
			}
		})
	}
}

// TestOpenRejectsMissingHost is the connection-string half of the
// no-fallback-DSN guarantee: a DSN that parses successfully but omits an
// explicit host resolves ambiently (PGHOST, a service file, or a platform
// default) rather than failing — verified directly against pgx v5.10.0
// during review. requireExplicitHost must reject every DSN shape that
// reaches that fallback, not just the wholly-empty one ErrEmptyDSN already
// covers.
func TestOpenRejectsMissingHost(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"hostless URL, no auth", "postgres:///cinzal"},
		{"hostless URL, with auth", "postgres://user:placeholder@/cinzal"},
		{"keyword form, even with an explicit host=", "host=myhost dbname=cinzal"},
		{"keyword form, no host at all", "dbname=cinzal"},
		{"bare service reference", "service=myservice"},
		{"service query param on an otherwise-valid URL", "postgres://user:placeholder@realhost/cinzal?service=myservice"},
		{"servicefile query param on an otherwise-valid URL", "postgres://user:placeholder@realhost/cinzal?servicefile=/tmp/svc.conf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(context.Background(), Config{DSN: tt.dsn})
			if s != nil {
				t.Errorf("Open(%q) returned a non-nil *Store", tt.dsn)
			}
			if !errors.Is(err, ErrDSNMissingHost) {
				t.Fatalf("Open(%q) error = %v, want ErrDSNMissingHost", tt.dsn, err)
			}
		})
	}
}

// TestOpenAcceptsExplicitHostRegardlessOfEnvironment is
// TestOpenRejectsMissingHost's positive counterpart: a DSN that does name a
// host must not be rejected merely because PGHOST also happens to be set to
// something else in the environment — requireExplicitHost checks the DSN's
// own text, not ambient state.
func TestOpenAcceptsExplicitHostRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("PGHOST", "env-supplied-host-should-be-irrelevant")

	tests := []string{
		"postgres://user:placeholder@realhost/cinzal",
		"postgres://user:placeholder@[::1]/cinzal",
		"postgresql://user:placeholder@realhost:5433/cinzal",
	}
	for _, dsn := range tests {
		t.Run(dsn, func(t *testing.T) {
			if err := requireExplicitHost(dsn); err != nil {
				t.Errorf("requireExplicitHost(%q) = %v, want nil", dsn, err)
			}
		})
	}
}
