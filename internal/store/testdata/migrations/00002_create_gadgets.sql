-- +goose Up
CREATE TABLE gadgets (id INTEGER PRIMARY KEY);

-- +goose Down
DROP TABLE gadgets;
