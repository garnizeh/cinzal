# Migrations

`internal/store/migrate.go` (#311) is the machinery that embeds and applies
whatever lands in this directory. `00001_base_schema.sql` (#312) is RFC-001
§7.2's schema outside the D16–D20/D53 additions. `00002_recap_invites_notes_prefs.sql`
(#313) carries D16's Recap cursor, D17's invite links, D18's board notes,
D19's email preferences, and D53's `users.email_suppressed_at` — the two of
those that are deliberate exceptions to §7.1 (`invite_links`, `board_notes`)
say so in their own `COMMENT ON TABLE`. D20's `rate_limits` table lands in
`00003` (#314), a separate migration since it is the one addition that is
not just storage — the check-and-consume query it needs is scoped there,
not here.

This file exists so `//go:embed migrations` always has a non-empty,
non-dotfile directory to embed even before the first real migration
existed — `go:embed` is a compile-time error over an empty directory, and
dotfiles (`.gitkeep`) are excluded from a directory embed by default.
`goose`'s own migration scan ignores any file that doesn't match its
`NNN_name.sql`/`.go` naming pattern, so this file stays inert to it.
