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
			got := sessionTimeoutStatements(tt.cfg)
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
