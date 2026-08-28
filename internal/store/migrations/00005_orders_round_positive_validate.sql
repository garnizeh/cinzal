-- +goose Up

-- Second half of the NOT VALID / VALIDATE split (a CodeRabbit review
-- finding on PR #393, see 00004_orders_round_positive.sql's own comment).
-- 00004 added orders_round_positive as NOT VALID: live at once for new
-- writes, but Postgres has not yet confirmed every pre-existing row
-- satisfies it. VALIDATE CONSTRAINT does that scan now, in its own
-- migration so it runs as its own transaction/lock scope rather than
-- inside 00004's.
--
-- VALIDATE CONSTRAINT only takes a ShareUpdateExclusiveLock, which
-- conflicts with other DDL (another VALIDATE, an index build) but not with
-- ordinary concurrent SELECT/INSERT/UPDATE/DELETE -- the property that
-- makes the two-step pattern worth doing at all. Once this runs,
-- orders_round_positive is indistinguishable from a constraint added the
-- ordinary way; nothing about the constraint's name or predicate changes
-- from what 00004 declared.
ALTER TABLE orders VALIDATE CONSTRAINT orders_round_positive;

-- +goose Down

-- Nothing to undo: Postgres has no "un-validate a constraint" operation,
-- and VALIDATE CONSTRAINT itself changes only Postgres's internal
-- pg_constraint.convalidated bookkeeping, not any row or schema object.
-- Rolling back 00004 (which drops orders_round_positive entirely) is what
-- actually reverses this migration's effect; there is nothing left for
-- this Down to do once that happens.
