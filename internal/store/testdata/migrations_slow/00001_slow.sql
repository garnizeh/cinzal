-- +goose Up
SELECT pg_sleep(2);
CREATE TABLE slow_table (id INTEGER PRIMARY KEY);

-- +goose Down
DROP TABLE slow_table;
