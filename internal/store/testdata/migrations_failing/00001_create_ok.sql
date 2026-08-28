-- +goose Up
CREATE TABLE ok_table (id INTEGER PRIMARY KEY);

-- +goose Down
DROP TABLE ok_table;
