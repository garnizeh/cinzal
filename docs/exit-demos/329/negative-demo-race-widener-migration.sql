Added as internal/store/migrations/00006_demo_329_temp_race_widener.sql,
run against, then deleted in the same turn — never committed. See
329-boot-race.md's Method section for why the real 00001-00005 migrations
alone did not produce an observable failure without the lock.

-- +goose Up
-- TEMPORARY, issue #329's negative exit demonstration only — never committed.
-- The real production migrations (00001-00005) apply fast enough that two
-- unlocked processes rarely overlap on the same wall clock; this widens the
-- window so the underlying catalog-level conflict is actually observed
-- rather than missed by luck.
SELECT pg_sleep(0.5);
CREATE TABLE demo_329_race_widget (id serial primary key);

-- +goose Down
DROP TABLE demo_329_race_widget;
