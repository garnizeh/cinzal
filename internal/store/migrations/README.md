# Migrations

`internal/store/migrate.go` (#311) is the machinery that embeds and applies
whatever lands in this directory. `00001_base_schema.sql` (#312) is RFC-001
§7.2's schema outside the D16–D20/D53 additions; those land in `00002` and
`00003` (#313, #314), both blocked on this one.

This file exists so `//go:embed migrations` always has a non-empty,
non-dotfile directory to embed even before the first real migration
existed — `go:embed` is a compile-time error over an empty directory, and
dotfiles (`.gitkeep`) are excluded from a directory embed by default.
`goose`'s own migration scan ignores any file that doesn't match its
`NNN_name.sql`/`.go` naming pattern, so this file stays inert to it.
