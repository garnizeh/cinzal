-- +goose Up

-- game.RoundNumber is 1-indexed (GDD §4, internal/game/ids.go's own doc
-- comment); nothing in 00001's orders table stopped round from being 0 or
-- negative. checkNoRoundGap (internal/store/orderlog/orderlog.go) and
-- AppendOrder (internal/store/orders.go) both reject round < 1 in Go now,
-- but this is the storage layer's own belt: a row that reaches the table by
-- any other path (a future direct insert, a repair script) still cannot
-- carry a round the rest of the system treats as impossible.
--
-- Added NOT VALID (a CodeRabbit review finding on PR #393): a normally-
-- validated ADD CONSTRAINT CHECK takes an ACCESS EXCLUSIVE lock for the
-- full scan it does to verify every existing row, blocking concurrent
-- reads and writes on orders for the scan's duration -- unacceptable on a
-- table this milestone expects to grow without bound. NOT VALID skips that
-- scan and only takes the lock for the instant needed to record the
-- constraint's existence; every row inserted or updated from this point on
-- is still checked immediately, so the floor is live right away. The
-- existing-row scan is deferred to 00005_orders_round_positive_validate.sql,
-- which runs it under ShareUpdateExclusiveLock instead -- a lock that
-- coexists with concurrent DML on the table.
ALTER TABLE orders ADD CONSTRAINT orders_round_positive CHECK (round >= 1) NOT VALID;

-- +goose Down

ALTER TABLE orders DROP CONSTRAINT orders_round_positive;
