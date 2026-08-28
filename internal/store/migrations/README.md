# Migrations

Empty on purpose. `internal/store/migrate.go` (#311) is the machinery that
embeds and applies whatever lands in this directory; the first `.sql` file —
RFC-001 §7.2's schema plus the D16–D19 additions — is #312's own deliverable,
not this one's.

This file exists only so `//go:embed migrations` has a non-empty,
non-dotfile directory to embed — `go:embed` is a compile-time error over an
empty directory, and dotfiles (`.gitkeep`) are excluded from a directory
embed by default. `goose`'s own migration scan ignores any file that doesn't
match its `NNN_name.sql`/`.go` naming pattern, so this file is inert to it.
