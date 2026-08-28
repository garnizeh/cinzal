-- +goose Up

-- Generic per-key rate limiting (D20). scope + key is the whole identity;
-- auth is the only caller in v1 (auth_email / auth_ip, RFC-001 Sec.12.3),
-- but nothing here is auth-specific -- a future scope reuses this table and
-- the same check-and-consume statement (internal/store/ratelimit.go) with a
-- new (capacity, rate) pair, no migration required.
--
-- Continuous-refill token bucket, not a fixed window or a sliding window:
-- one row per key, refilled lazily on read, so write/cleanup cost is
-- constant per key regardless of how hard an attacker floods it -- see
-- D20's Reasoning for why that beats both alternatives here.
--
-- Not a derived projection -- nothing in orders determines a bucket's
-- state, and cmd/replay --rebuild (RFC-001 Sec.7.1/7.2) never touches it,
-- the same authoritative-with-no-order-log-equivalent status invite_links
-- and board_notes have in migration 00002, for the same underlying reason:
-- a rate-limit decision leaves no trace in the game's own order log.
CREATE TABLE rate_limits (
  scope TEXT NOT NULL,             -- 'auth_email' | 'auth_ip' in v1
  key TEXT NOT NULL,               -- an email address, or an IP key (DeriveIPKey, D20's trusted-hop rule)
  tokens DOUBLE PRECISION NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL, -- written with clock_timestamp(), never now() -- see ratelimit.go
  PRIMARY KEY (scope, key)
);
COMMENT ON TABLE rate_limits IS
  'Generic token-bucket rate limiter (D20). Authoritative state with no '
  'order-log equivalent -- never a derived projection, out of cmd/replay '
  '--rebuild''s scope, the same category as invite_links/board_notes.';

-- Cleanup sweep only (D20): the check-and-consume statement is an indexed
-- point lookup on the primary key (scope, key) and never scans this index.
CREATE INDEX rate_limits_updated_at_idx ON rate_limits (updated_at);

-- +goose Down

DROP TABLE rate_limits;
