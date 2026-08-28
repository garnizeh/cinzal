-- +goose Up

-- All-matches unsubscribe (D53). Set only from the unsubscribe confirmation
-- page's human path, never the automated RFC 8058 one-click body -- that
-- request carries no field to signal anything wider than the one match its
-- token names. NULL = no global suppression; the per-match email_pref below
-- is the narrower, more common opt-out.
ALTER TABLE users ADD COLUMN email_suppressed_at TIMESTAMPTZ NULL;

-- The bearer credential for admission to one match's lobby (D17). A separate
-- table, not a matches column, so one leaked link can be revoked without
-- invalidating every other link into the same match or kicking the seats a
-- different link already admitted. Not a derived projection -- unlike
-- match_players.last_seen_round below, a revoked or expired link leaves no
-- trace in orders, so there is nothing for cmd/replay --rebuild to fold it
-- back from; it is authoritative state, out of that command's scope entirely.
CREATE TABLE invite_links (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  match_id UUID NOT NULL REFERENCES matches (id) ON DELETE RESTRICT,
  token_hash BYTEA UNIQUE NOT NULL,   -- sha256(32 random bytes from crypto/rand); the raw token is never stored
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NULL,        -- NULL = no forced expiry beyond the match's own lobby window
  revoked_at TIMESTAMPTZ NULL,        -- NULL = live; set = revoked. A flag, never a DELETE -- attribution survives.
  -- lets match_players FK to (id, match_id), not id alone -- see below.
  UNIQUE (id, match_id)
);
COMMENT ON TABLE invite_links IS
  'Authoritative state with no order-log equivalent -- never a derived '
  'projection, out of cmd/replay --rebuild''s scope. See D17.';

-- Recap cursor (D16), advanced inside the order-submission transaction
-- (RFC-001 Sec.8.1, corrected by D52) on a human seat's first submission for
-- a round, never on a GET of the board or the Recap fragment -- the same
-- prefetch hazard Sec.12.2 already rejected for OTP magic links. 0 at seat
-- creation covers both lobby-formation and mid-lobby-join seats identically,
-- since POST /m/{id}/join only ever runs before round 1 resolves.
-- Derived and rebuildable from orders, exactly like events/match_summary/
-- missed_deadlines below it -- NOT a new exception to Sec.7.1's
-- state = fold(...), unlike invite_links/board_notes above/below.
ALTER TABLE match_players ADD COLUMN last_seen_round INT NOT NULL DEFAULT 0;
COMMENT ON COLUMN match_players.last_seen_round IS
  'Derived, rebuildable read cache of the order log -- same category as '
  'missed_deadlines, events and match_summary, not an exception to Sec.7.1. '
  'See D16, corrected by D52.';

-- Which link admitted this seat (D17); NULL for the host's own seat or a bot
-- fill. Composite, not a plain FK to invite_links(id) alone: a bare
-- REFERENCES invite_links(id) only proves the link exists somewhere, not
-- that it belongs to *this* match -- the composite FK against
-- invite_links(id, match_id) makes single-match scope physically
-- unrepresentable rather than a convention every future caller must
-- remember.
ALTER TABLE match_players ADD COLUMN invite_link_id UUID NULL;
ALTER TABLE match_players
  ADD CONSTRAINT match_players_invite_link_fk
  FOREIGN KEY (invite_link_id, match_id) REFERENCES invite_links (id, match_id);

-- Per-match email volume preference (D19). Three of RFC-001 Sec.13's four
-- named levels -- daily_digest is deferred, not one of these (D19's own
-- Option B; it is a second delivery mechanism, not a predicate over a row
-- Tick() already writes). Default matches Sec.13's stated default ("only
-- when it's my turn and I haven't moved").
ALTER TABLE match_players ADD COLUMN email_pref TEXT NOT NULL DEFAULT 'turn_only'
  CHECK (email_pref IN ('every_round', 'turn_only', 'none'));

-- Per-seat one-click-unsubscribe token (D19), sha256(32 random bytes from
-- crypto/rand) -- the same shape as invite_links.token_hash, minted once at
-- seat creation, unconditionally, for every seat regardless of whether it
-- currently has a deliverable address (D53's 2A: cheaper than branching seat
-- creation on whether an email is known yet, and it means a guest who binds
-- an email later needs no second mint). No UNIQUE constraint, unlike
-- invite_links.token_hash: this token is never looked up by value alone,
-- the URL already carries (match_id, seat) and the row is found by that
-- primary key first.
ALTER TABLE match_players ADD COLUMN unsubscribe_token_hash BYTEA NOT NULL;

COMMENT ON COLUMN match_players.email_pref IS
  'D19: per-match email volume preference. daily_digest deferred -- not one of these three.';
COMMENT ON COLUMN match_players.unsubscribe_token_hash IS
  'D19: one-click-unsubscribe bearer token, sha256 digest, minted at seat '
  'creation regardless of address eligibility (D53). Directly-written state, '
  'not derived -- nothing in orders determines a preference or a token.';

-- Manual annotation: the Board's fourth tool (GDD Sec.7.5, D18). Authoritative
-- state with no order-log equivalent, the same kind of exception as
-- invite_links above -- a pin or note leaves no trace in orders, so there is
-- nothing to rebuild it from. Seat-private: every query against it is scoped
-- to (match_id, seat) with seat taken from the session, never a path or form
-- parameter, and no query the store package exposes returns another seat's
-- rows.
CREATE TABLE board_notes (
  match_id UUID NOT NULL,
  seat SMALLINT NOT NULL,
  -- the per-seat cap, enforced by the bound itself -- Postgres cannot
  -- express "at most N sibling rows" declaratively, and a trigger would be
  -- procedural enforcement bolted onto one table for one column.
  slot SMALLINT NOT NULL CHECK (slot BETWEEN 1 AND 20),
  node_id INT NULL,     -- pinned to a node (game.NodeID, no FK -- the map is generated at runtime, never a stored table); NULL = a freeform note
  round INT NOT NULL,   -- the round it was written in
  body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 500),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (match_id, seat) REFERENCES match_players (match_id, seat),
  PRIMARY KEY (match_id, seat, slot)
);
COMMENT ON TABLE board_notes IS
  'Authoritative state with no order-log equivalent -- never a derived '
  'projection, out of cmd/replay --rebuild''s scope, and never rendered to '
  'any seat but its own author. See D18.';

-- +goose Down

ALTER TABLE match_players DROP CONSTRAINT match_players_invite_link_fk;
DROP TABLE board_notes;
ALTER TABLE match_players DROP COLUMN unsubscribe_token_hash;
ALTER TABLE match_players DROP COLUMN email_pref;
ALTER TABLE match_players DROP COLUMN invite_link_id;
ALTER TABLE match_players DROP COLUMN last_seen_round;
DROP TABLE invite_links;
ALTER TABLE users DROP COLUMN email_suppressed_at;
