-- +goose Up
CREATE TABLE broken_table (id INTEGER REFERENCES table_that_does_not_exist(id));

-- +goose Down
DROP TABLE broken_table;
