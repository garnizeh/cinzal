# D55 — Nothing pins the project's timezone handling to UTC: session, process, or display

**Status:** decided
**Blocks:** the `pgx/v5` pool ([#310](https://github.com/garnizeh/cinzal/issues/310)) and migration 0001 ([#312](https://github.com/garnizeh/cinzal/issues/312))
**Decided:** 2026-08-27
**Issue:** [#359](https://github.com/garnizeh/cinzal/issues/359)

## The question

RFC §7.2 makes every timestamp column `TIMESTAMPTZ`, which normalizes *storage*
to UTC — a property of the Postgres type, not a decision this document needs
to make. What is undecided is everything on either side of storage: the
**session timezone** a `pgx` connection negotiates, the **process timezone**
the Go binary runs under, and what a human ever sees formatted from either.
Nothing in the RFC, the GDD, or CONTRIBUTING states that any of this is UTC
end to end. §6.3 bans `time.Now()` inside `internal/rules`, but for replay
determinism — a different concern from display or logging correctness — and
it is the only place the codebase currently talks about time at all.

## Why it is open

The issue frames three live risks: a `pgx` session's timezone default
differing across a developer's laptop, CI (D46), and production; the Go
process's `time.Now()` following whatever `TZ` it inherits; and `slog`'s
default handler timestamping in local time. All three are real *inputs*.
Whether they produce an actual correctness gap in *this* codebase, rather
than a generic warning that would apply to any Postgres-backed Go service,
needs checking against what `pgx` v5 and Go's `time` package actually do —
not assumed from the general shape of the problem.

**Reading pgx's own decode path settles the first risk mechanically, not by
argument.** `pgx` v5 defaults to the extended query protocol with binary
format for known types, including `timestamptz`. On the wire, Postgres's
binary `timestamptz` format is `microsecSinceY2K` — an absolute instant
computed from the column's own UTC-normalized storage — with **no session
timezone applied**; timezone only shapes `timestamptz`'s *text* rendering
(`to_char`, a bare `SELECT` in `psql`, `date_trunc`, `EXTRACT` of a
calendar field, `::date`). `pgtype/timestamptz.go`'s binary decode path is:

```go
tim := time.Unix(
    microsecFromUnixEpochToY2K/1_000_000+microsecSinceY2K/1_000_000,
    (microsecFromUnixEpochToY2K%1_000_000*1_000)+(microsecSinceY2K%1_000_000*1000),
).UTC()
if plan.location != nil {
    tim = time.Date(tim.Year(), tim.Month(), tim.Day(), tim.Hour(), tim.Minute(), tim.Second(), tim.Nanosecond(), plan.location)
}
```

`plan.location` comes from `TimestamptzCodec.ScanLocation`, which is `nil`
unless an application explicitly sets it — and its own doc comment states
explicitly it **does not change the instant**, only re-labels it, unlike
`TimestampCodec.ScanLocation` (the *timezone-less* `timestamp` type's
codec, which this schema never uses and whose `ScanLocation` genuinely does
change the value). **The default, unconfigured path already returns a
UTC-labeled `time.Time` carrying the correct instant, regardless of what
timezone the session negotiated.** The issue's own claim — "the `time.Time`
Go receives is labeled with three different offsets" under three different
session timezones — does not hold for this schema's read path today: no
query anywhere in RFC §7.2 or §8.1 uses `date_trunc`, `EXTRACT` of a
calendar field, `to_char`, or `::date` (confirmed by grep across the RFC and
every decided `Dnn` document — zero matches), so no session-timezone-
dependent server-side computation exists in this codebase to get wrong.
D20's `EXTRACT(EPOCH FROM (clock_timestamp() - rate_limits.updated_at))` is
timezone-independent for the identical reason: epoch seconds are absolute.

**The second risk is real, but narrower than "wrong instant."** Go's
`time.Time` comparison methods — `Before`, `After`, `Equal`, `Sub` — compare
the absolute instant each value represents, never its `Location`. §8's
`Tick()` sample, `expired := time.Now().After(match.DeadlineAt)`, compares a
Go-side `time.Now()` (whatever `Location` the process's `TZ` gives it)
against a `time.Time` read from Postgres via `pgx` (always UTC-labeled, per
the finding above) — and produces the correct answer regardless of the
process's `TZ`, because `.After()` never looks at either operand's
`Location`. Process timezone cannot silently break a deadline comparison
the way the issue's framing implies; §8.1's own authoritative check (`now()
>= deadline_at`, evaluated inside Postgres) does not depend on the Go
process's clock at all in any case. **Where process timezone actually
leaks into observable behaviour is formatting: the moment a `time.Time` is
rendered to text** — a log line, a debug string, an eventual email body
composed outside SQL — its `Location` determines what a reader sees, even
though every comparison and every persisted value stayed correct underneath.
This is the concrete form of the issue's own "ambient time" concern, scoped
to exactly the surface where it bites.

**The third risk, `slog`'s default handler, is exactly as stated.**
`log/slog`'s handlers timestamp each record with `time.Now()` at the call
site and emit it through the handler's own formatting, in whatever
`Location` that `time.Now()` carries, unless `HandlerOptions.ReplaceAttr`
intercepts the time attribute. RFC §17 states JSON-to-stdout logging and
says nothing about the timestamp's zone — a real, unstated gap.

## Options

**Session pin — A: leave the server's session default untouched.** Costs
nothing today, per the finding above: no query in this schema is session-
timezone-sensitive, so no observed value is wrong. Cost is latent, not
absent: the day a future migration adds a `date_trunc`/`::date`/`to_char`
expression (a daily digest window, GDD's deferred `daily_digest` email
preference, an analytics rollup), its correctness would silently depend on
whatever each environment's Postgres `timezone` GUC happens to default to —
exactly the drift D46 already distrusts between a laptop, CI's container,
and production. It also leaves `psql`-based manual inspection during
development rendering in the connecting operator's own local default,
which is a debugging-friction cost paid continuously, not a one-time one.

**Session pin — B: pin every `pgx` connection's session timezone to UTC.**
One field, `pgconn.Config.RuntimeParams["timezone"] = "UTC"` (exposed
through `pgxpool.ParseConfig`'s returned `*pgxpool.Config`), sent in the
connection's own startup packet — no extra round trip, unlike an
`AfterConnect` hook issuing `SET TIME ZONE 'UTC'` as a separate statement
per new connection. Costs one line at pool construction, closes the latent
gap before it can open, and makes `psql $DATABASE_URL` during development
show the same values the application computes with, at no cost given
Option A's finding that nothing currently depends on the *unpinned* default
either.

**Process timezone — A: require `TZ=UTC` in every environment.** Matches a
common convention (a Dockerfile `ENV TZ=UTC`) and needs no code change
anywhere `time.Now()` is called. Cost: it is exactly the kind of ambient,
environment-supplied guarantee this project has already twice preferred not
to depend on for correctness — D46 rejected two mechanisms for the same
underlying reason (a CI service container and a local `docker compose` file
are "two descriptions of the same intended database that can drift"), and
§6.3's own rule 3 treats ambient time as a hazard *because* it is ambient,
not because `time.Now()` itself is expensive to call. A developer running
`go test` locally with no `TZ` set, or a future deploy target whose base
image sets its own default, silently reintroduces exactly the process-level
drift the issue raises — and nothing would catch it, since (per the finding
above) most call sites tolerate a non-UTC `Location` without producing a
wrong *value*, only a wrong *label* the moment something is formatted.

**Process timezone — B: `.UTC()` discipline at every point a `time.Time` is
formatted or logged, with no reliance on process `TZ`.** Costs a convention
that has to be held at every call site that renders a time value to text —
`slog` call sites, and any future human-readable string built from a
`time.Time` outside SQL (an email body composed in Go rather than by a
template that already receives a `TIMESTAMPTZ`-sourced, UTC-labeled value).
Nothing about comparisons, persistence, or `pgx` reads needs it, per the
findings above — the discipline is scoped to exactly the surface where
`Location` is observable, not applied everywhere out of caution. It is
checkable in code review the same way `internal/rules`' "resolution never
ranges over a map" rule already is — no mechanical gate exists for that
rule either, and CONTRIBUTING.md accepts it as a stated discipline, not
every project rule needing a linter to be real.

## Decision

**Session: Option B, pinned via `pgconn.Config.RuntimeParams["timezone"] =
"UTC"`** on the `pgxpool.Config` #310's pool construction returns from
`pgxpool.ParseConfig(databaseURL)`, before `pgxpool.NewWithConfig` is
called — sent in every connection's own startup packet, not a per-connection
`AfterConnect` round trip. This closes the latent gap identified above
before migration 0001 gives it a real column to matter against, and it
costs nothing given that today's queries don't need it — it is bought now
because the alternative is a silent dependency on each environment's
Postgres default the day a `date_trunc`/`::date`/daily-digest-shaped query
is added later, and pinning then would need auditing every existing query
for a timezone assumption nobody wrote down. **This does not change what
any current query returns** — see Reasoning for why `pgx`'s binary decode
already made the session timezone irrelevant to every `TIMESTAMPTZ` value
this schema reads today.

**Process: Option B, `.UTC()` discipline, with no code relying on `TZ`.**
Every `time.Time` value that is formatted, logged, or otherwise rendered to
text calls `.UTC()` first. This applies to `slog` output (below) and to any
future code that composes a human-readable time string in Go rather than
handing a `TIMESTAMPTZ`-sourced value straight to a template. It does
**not** apply to comparisons (`Before`/`After`/`Sub` against a
`pgx`-sourced or `time.Now()`-produced value) or to values passed to `pgx`
for a parameterized query — both are `Location`-independent already, per
the finding above, and adding `.UTC()` there would be a no-op restating a
guarantee Go and `pgx` already hold. **The production Docker image still
sets `ENV TZ=UTC`** (§18) as a harmless backstop — cheap, and it means a
process that *did* skip the discipline somewhere fails safe rather than
silently mislabeling in production specifically — but no code path is
allowed to depend on it being set, and CI/local dev run with no such
guarantee, which is the environment the discipline has to hold in anyway.

**`slog`: `HandlerOptions.ReplaceAttr` forces the timestamp to UTC.**

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        if a.Key == slog.TimeKey {
            a.Value = slog.TimeValue(a.Value.Time().UTC())
        }
        return a
    },
})
```

One handler, constructed once at `cmd/server`'s startup; every `slog` call
site downstream is unaffected and needs no change.

**Informal project timestamps (a decision document's `Decided:` field, and
the like) are out of scope, not merely low-stakes.** They are bare calendar
dates (`YYYY-MM-DD`), not `TIMESTAMPTZ` instants — a timezone question does
not apply to a value with no time component, the same way it does not apply
to `Node.Name` or any other timezone-free field. GitHub's own PR/issue
timestamps are already UTC per GitHub's platform guarantee and are not this
project's to pin.

**Player-facing time display is a separate, later question this decision
does not answer.** GDD's round deadlines are round-based; RFC §8's
wall-clock `deadline_at` is the lifecycle sweeper's own internal
comparison value, and whatever M5's board or M6's email shows a player from
it — UTC as written, or converted to a viewer's local time — is a UX
decision with no bearing on any mechanism decided here: every value stays
correctly UTC-normalized in storage and correctly instant-accurate through
`pgx` regardless of how M5/M6 later choose to *display* it.

## Reasoning

**Why the session pin is decided even though nothing today needs it.**
Option A's cost is real but deferred — it is exactly the "surfaces months
later" shape §6.3 and RFC §6.4's RNG consumption table both already warn
about for a different kind of ambient hazard: a gap that costs nothing
until the day a specific new query needs the thing nobody pinned, at which
point the fix requires auditing every prior query for an assumption nobody
recorded making. `RuntimeParams["timezone"] = "UTC"` is one field, decided
once, at the exact point in the build order (#310, before migration 0001's
first wall-clock columns) where paying for it is cheapest and where
leaving it undecided would otherwise let #310 and #312 each guess
independently.

**Why the process-timezone answer doesn't take the cheaper-looking
`TZ=UTC`-only option.** The issue itself supplies the reason to distrust an
environment variable as the *sole* guarantee: "a developer's laptop, a CI
runner, and a container image are not guaranteed to agree" is the same
argument D46 already accepted once, about a different ambient variable
(which Postgres a test talks to), and rejected the environment-description
route both times it was offered (a CI `services:` block, a `docker compose`
file) in favor of one mechanism that can't drift because there's only one
description of it. `TZ=UTC` in a Dockerfile is one description that a local
`go test` invocation, a different CI image, or a future deploy target's own
base image can each independently fail to share — the drift is structural,
not hypothetical. `.UTC()` at the point of formatting is the one-mechanism
answer: it holds in every one of those environments identically, because it
depends on none of them.

**Why this isn't "add `.UTC()` everywhere" — the scope is load-bearing.**
The pgx decode finding and Go's `Location`-independent comparison semantics
together mean most of the codebase's eventual time-handling code needs no
change and no discipline at all: `Tick()`'s deadline check, every `pgx`
parameter binding, every stored `TIMESTAMPTZ` round-trip are correct
regardless of process `TZ`, today and after this decision. Stating the
discipline as "everywhere" would be advice to review for a hazard that
mostly doesn't exist in this codebase's actual data flow, and would dilute
the one place — text rendering — where a reviewer's attention should
concentrate. This mirrors §6.3's own shape: the rule against ranging over a
map in `resolution` isn't "never range over a map anywhere in the
codebase," it's scoped to the one place iteration order is observable in
output.

**Why `RuntimeParams` over `AfterConnect`.** Both reach the same session
state. `RuntimeParams` is carried in the connection's own startup packet —
the same handshake that already negotiates `search_path` or
`application_name` — costing nothing beyond what establishing the
connection already pays. `AfterConnect` would add one more statement's
round trip to every new pool connection, forever, to set a value that
never needs to vary per connection. Given `pgxpool`'s pool sizing (§18: "two
app instances behind a load balancer is the ceiling this design needs"),
the difference is immaterial at this scale, but there is no reason to pay
even that when the cheaper mechanism exists and does the identical thing.

## Consequences

- **RFC §7.5** gains one paragraph: the pool's `RuntimeParams["timezone"] =
  "UTC"` pin, and the finding that `pgx` v5's binary-protocol `timestamptz`
  decode is already session-timezone-independent — so #310's pool
  construction has a concrete line to write, and #312's migration needs no
  timezone-specific handling beyond the `TIMESTAMPTZ` type it already uses.
- **RFC §17** gains the `ReplaceAttr` snippet above, so `cmd/server`'s
  `slog` setup has one exact shape to build rather than an unstated gap.
- **RFC §18** gains `ENV TZ=UTC` in the Docker image description, named
  explicitly as a backstop, not a dependency — no new required env var
  joins the `DATABASE_URL`/`MAIL_PROVIDER_KEY`/`BASE_URL`/`SESSION_KEY`/
  `TRUSTED_PROXY_HOPS` list, since correctness never branches on whether
  it's set.
- **#310's pgx pool** can now be built against a concrete contract: pin
  `RuntimeParams` before `NewWithConfig`, no `AfterConnect` hook needed for
  this purpose.
- **#312's migration 0001** needs no schema change from this decision —
  `TIMESTAMPTZ` was already the right type (RFC §7.2, D17/D18/D19/D20 all
  already use it); this decision only fixes what surrounds the column, not
  the column itself.
- **A future daily-digest-shaped query** (GDD's deferred `daily_digest`
  email preference, D19) that needs `date_trunc`/`::date` gets a session
  timezone that is already pinned, rather than inheriting whatever each
  environment's Postgres defaults to at the time it's written.
- **No GDD text change** — this is a persistence-and-observability
  decision with no game-rule content; GDD's round-based deadlines are
  untouched. Companion doc stays at its current version; RFC moves to the
  next revision.
- **Reversible at low cost.** The session pin is one config field; the
  `.UTC()` discipline is a code-review convention with no schema or wire-
  format dependency. Superseding either is a documents-and-one-line-of-code
  change, not a migration.
