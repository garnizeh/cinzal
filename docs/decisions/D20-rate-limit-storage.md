# D20 — Rate-limit state has no home: the Postgres shape, the IP key, and what happens when the limiter fails

**Status:** decided
**Blocks:** the M3 rate-limit migration, and M5's auth handlers, which cannot enforce §12.2's limits without somewhere to keep the counters
**Decided:** 2026-08-26
**Issue:** [#306](https://github.com/garnizeh/cinzal/issues/306)

## The question

RFC §12.2 states two hard limits — **3 requests per email per 15 min, 20 per IP per hour** — and nowhere to keep them. An in-process counter is wrong the moment there are two app instances, which §18 names as the deployment ceiling this design is built for: a limit enforced in memory is silently a limit of 6 and 40 in production. The roadmap already rules out Redis (§4) and recommends Postgres, so the open part is the shape:

1. **Fixed window, sliding window, or token bucket** — a fixed window lets double the limit through at a window seam; a sliding window needs a row per request; a token bucket is one row per key and one `UPDATE`.
2. **What is the key**, and what does "per IP" mean behind a load balancer — `X-Forwarded-For` is attacker-controlled unless the trusted-hop count is pinned, and IPv6 needs a prefix rule, since limiting per `/128` limits nothing.
3. **What happens when the limiter itself fails** — a timed-out check must not silently open the door, and the repo's own fail-closed posture has a real cost here: a database blip can lock everyone out of login.

## Why it is open

**Enumeration and rate limiting are the same control.** §12.2 also requires `/auth/request`'s response to be identical for known and unknown emails. A limiter that returns `429` for a rate-limited real account and `200` for an unknown one hands back exactly the distinction §12.2 removed — the limiter's response has to be identical too, which is easy to miss when it's written as generic middleware.

**Cleanup is not optional and is not free.** Rows accumulate on the auth path forever unless something deletes them, and the deployment topology is "one binary + Postgres" (§18) — no cron, no scheduler beyond what the binary itself runs.

**The write lands on the login hot path**, and the number that matters is its cost *under flood*, not at rest — the roadmap's "low-traffic by nature" is true and is also not the argument an attacker is making.

## Options

**1. Algorithm.**

- **Fixed window** (`COUNT(*) WHERE window_start = current_bucket`, one row per key per window). Cheapest to reason about, but lets a client send its full quota at 14:59 and again at 15:00 — double the stated limit across the seam, for the same reason the in-memory counter is wrong.
- **Sliding window** (one row per request, `COUNT(*)` over a trailing interval). Exact at the boundary, but its write volume — and its cleanup cost — scale with request count, which is exactly the number an attacker controls. A flood that is supposed to be made cheap to reject instead gets more expensive to store the harder it pushes.
- **Token bucket** (one row per key, refilled continuously, one `UPDATE`/`INSERT ON CONFLICT`). No fixed clock seam to double through — the burst it allows is bounded to the key's own bucket, not to a shared wall-clock boundary every key resets on at once — and its per-request cost is constant regardless of flood size, the row reused, never multiplied. It is not exact either: burst capacity stacked on continuous refill still bounds the worst case in any one rolling window to roughly double the stated number, just as a fixed window does — see Reasoning for why that shared bound is still the right tradeoff here.

**2. IP key.**

- **Trust `X-Forwarded-For` as given.** The leftmost entry is whatever the client sent; an attacker rotates it per request and the limit never engages. Not a rate limit — a rate limit with a documented bypass.
- **Pin a trusted-hop count** and read the entry that many positions from the right, ignoring everything left of it. Costs one piece of deployment configuration (how many proxies sit in front of the app) in exchange for a key nothing client-supplied can select.

**3. Failure policy.**

- **Fail-open** (limiter error → allow). Never locks a real user out, but converts any transient Postgres hiccup into a rate-limiting bypass window, on the exact path (§19: OTP brute force) the limiter exists to close.
- **Fail-closed** (limiter error → deny). Matches the repo's established gate posture, but the issue's own framing is right that this needs a stated reason, not just an appeal to precedent, since a runtime path failing shut has a live-user cost a CI gate never has.

## Decision

**A single generic table**, not auth-specific, storing a **token bucket** per key:

```sql
-- generic per-key rate limiting (D20). scope + key is the whole identity;
-- auth is the only caller in v1, but nothing here is auth-specific.
rate_limits(
  scope TEXT NOT NULL,             -- 'auth_email' | 'auth_ip' in v1
  key TEXT NOT NULL,               -- an email address, or an IP key per the derivation below
  tokens DOUBLE PRECISION NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (scope, key)
);
CREATE INDEX rate_limits_updated_at_idx ON rate_limits (updated_at);  -- cleanup sweep only
```

Two buckets in v1, both drawn on by **both** `/auth/request` and `/auth/verify` — one shared attempt budget per email and per IP across the whole authentication surface, matching §19's single "OTP brute force" threat row, which already cites §12.2 for both endpoints together rather than naming two separate limit pairs:

| Scope | Capacity | Refill |
|---|---|---|
| `auth_email` | 3 | 1 token / 300s (3 per 900s) |
| `auth_ip` | 20 | 1 token / 180s (20 per 3600s) |

**Worst case: bounded to roughly double the stated number, not exact — stated plainly rather than left implicit.** A bucket found full and drained in one instant, then drained again as fast as it refills, admits at most `capacity + capacity` — 6 for `auth_email`, 40 for `auth_ip` — within any single rolling window equal to the refill period, not the stated 3 or 20. This is not a corner case nobody would hit; a patient attacker who always spends a token the instant it's available gets exactly this, indefinitely. See Reasoning for why that bound is accepted rather than chased away by a more exact algorithm.

**Algorithm: continuous-refill token bucket**, checked and consumed in one atomic statement (`$1`=scope, `$2`=key, `$3`=capacity, `$4`=refill rate in tokens/second, matching the positional style §8.1 already uses). It uses `clock_timestamp()`, not `now()`: `now()` is pinned to the enclosing transaction's *start*, and this check runs inside the same transaction as the rest of the handler (below) — a transaction held open by lock contention would write a backdated `updated_at`, letting a later check compute more elapsed time than actually passed and over-credit the refill. `clock_timestamp()` is evaluated fresh, so the value written is when the row is actually touched:

```sql
INSERT INTO rate_limits (scope, key, tokens, updated_at)
VALUES ($1, $2, $3 - 1, clock_timestamp())
ON CONFLICT (scope, key) DO UPDATE
  SET tokens = LEAST($3, rate_limits.tokens
                 + EXTRACT(EPOCH FROM (clock_timestamp() - rate_limits.updated_at)) * $4) - 1,
      updated_at = clock_timestamp()
  WHERE LEAST($3, rate_limits.tokens
                 + EXTRACT(EPOCH FROM (clock_timestamp() - rate_limits.updated_at)) * $4) >= 1
RETURNING tokens;
```

Zero rows returned means limited. Capacity and refill rate are Go constants in `internal/auth`, one pair per scope, passed as `$3`/`$4` — not `game.Config` fields, despite CONTRIBUTING's "a tunable number is a `Config` field" rule: `game.Config` is specifically the GDD's gameplay-balance dial surface (`internal/game/config.go`); these are RFC-fixed security thresholds with their own spec anchor (§12.2), not a design knob anyone tunes per match.

Both buckets are checked inside the same transaction as the rest of the handler — but a zero-row `RETURNING` is not itself an error to Postgres, and does not roll anything back on its own; `UPDATE`/`INSERT ... ON CONFLICT` affecting no rows is a normal, successful outcome, and the transaction stays open and would commit whatever else ran in it. The handler's own code has to check the row count from each consume call and, on zero, return an error from the transaction closure — the same `m.tx(ctx, func(q *store.Queries) error { … })` idiom §7.4/§8's `Tick()` already uses, where a non-nil return is what triggers the actual `ROLLBACK`. Nothing new: the existing wrapper mechanism, applied here, not a second one.

**IP key derivation.** A new env var, `TRUSTED_PROXY_HOPS` (alongside `DATABASE_URL`/`MAIL_PROVIDER_KEY`/`BASE_URL`/`SESSION_KEY`, §18), default `1` — matching §18's own "two app instances behind a load balancer" as the ceiling this design needs, i.e. exactly one trusted hop in the common case. The key is the `X-Forwarded-For` entry `TRUSTED_PROXY_HOPS` positions from the **right**; everything left of that point is client-supplied and ignored, so no header content an attacker controls can pick their own bucket. This needs **at least `TRUSTED_PROXY_HOPS` entries**, not `TRUSTED_PROXY_HOPS + 1` — a single trusted hop that appends to (rather than replaces) the header produces exactly one entry when the client sent none at all, which is the ordinary case for NGINX's `$proxy_add_x_forwarded_for` and HAProxy's `option forwardfor`, and is already the real client's address, not something to be suspicious of. Requiring one more than that would reject the common single-hop case and fall back to the connection's remote address — the load balancer's own — silently funnelling every client behind it into one shared bucket. Fewer than `TRUSTED_PROXY_HOPS` entries (a misconfiguration, or a direct connection bypassing the expected proxy chain) is what triggers the remote-address fallback instead: a stricter failure, not a bypass. IPv4 keys on the exact address (`/32`); **IPv6 keys on the `/64` prefix** — the usual single-customer allocation size, so an attacker who requests a fresh address inside their own delegated block doesn't get a fresh bucket for it. Limiting per `/128` limits nothing, since nothing stops that rotation.

**Failure policy: fail-closed.** A `rate_limits` check that errors — timeout, connection failure, anything — is treated identically to a returned zero rows: limited, no code issued, no mail enqueued.

**Cleanup.** An idle bucket sitting at full capacity is indistinguishable from a bucket that was never created — the consume statement's own `LEAST($3, …)` clamp recomputes the same result either way — so deleting it loses no state. An in-process ticker, the same shape as §8's deadline sweeper, runs every 10 minutes:

```sql
DELETE FROM rate_limits WHERE updated_at < now() - INTERVAL '1 hour';
```

One hour is `auth_ip`'s own full-refill time, the longer of the two configured in v1, not an arbitrary round number — by then either bucket has certainly refilled to capacity regardless of where it stood when the interval started, so the delete is exactly equivalent to leaving the row alone. That equivalence only holds because the retention is at least as long as every active scope's own full-refill time; a scope added later with a slower refill (say, a token every 10 minutes instead of 3) would still get swept at the one-hour mark, deleting a bucket still well short of capacity and silently handing it a fresh full budget on the next request — the sweep has to widen to the new maximum across all active scopes, not stay pinned to `auth_ip`'s. No separate connection pool the way §8.3 gives the sweeper its own: this is a single indexed-range delete, not a per-round contended lock, and `rate_limits_updated_at_idx` is what keeps it cheap.

**Response shape: identical, reusing the existing enumeration rule.** §12.2 already requires `/auth/request`'s response to be the same for known and unknown emails. A limiter error, a rate-limited real account, a rate-limited unknown account, and an unrate-limited unknown account all take that same path and produce that same response — the fail-closed policy above adds no new branch, because the branch it would have needed already exists for a different reason. `/auth/verify` carries no such constraint (the caller already holds a code to get this far), so its rate-limited or fail-closed response can say so plainly — "too many attempts, try again later" — rather than disguise itself.

**Scope: auth-only by design, table shape kept generic.** No other RFC-stated surface has a rate limit today — order submission and invite redemption are both unlimited per spec. `(scope, key)` costs nothing to leave general: a future scope reuses the same table and the same consume statement with one new `(capacity, rate)` pair in the caller, no migration. The cleanup sweep is the one piece that isn't a free reuse — its retention has to cover *every* active scope's own full-refill time (currently `auth_ip`'s one hour), so adding a scope with a slower refill means widening that one constant, not adding a second sweep. It is not built now because nothing cites a number to build it against — inventing one would be product policy with no spec anchor, the same discipline D35 and this project's decision-log convention already hold to elsewhere.

## Reasoning

**Why token bucket over the other two, given it shares fixed window's worst-case bound.** Both a fixed window and a continuous token bucket can be driven to admit roughly double the stated number within one window by an attacker who banks a full budget and spends it the instant it's available — that symmetry is real, and no framing of "continuous refill" changes it; the two are not equivalent in practice, though, for two reasons. First, *reachability*: a fixed window's worst case is trivial to trigger — any client whose 3rd and 4th request happen to straddle a clock-aligned quarter-hour boundary hits it by accident, with no adversarial timing at all, because every key resets on the same shared clock. A token bucket's window starts at that key's own first use, not a shared clock tick, so hitting the worst case requires an attacker to deliberately withhold requests until a specific key's bucket refills and then drain it the instant it does — a targeted, deliberate pattern, not something an ordinary retrying user stumbles into. Second, *cost symmetry with the option this rules out*: a sliding window (one row per request) is exact, but its row count, write volume, and cleanup burden scale with request count, which is exactly the variable an attacker controls — a limiter is supposed to make a flood cheap to reject, and a sliding window makes a flood more expensive to store the harder it's pushed. A token bucket's cost is one row and one upsert per key, forever, independent of how hard any one key is hit. The residual worst-case bound (§ above) is accepted rather than chased away by a sliding-window-style algorithm because §12.2's own defense-in-depth already bounds the damage: OTP codes still need 5 correct guesses to crack regardless of how many are issued (§12.2's own "max 5 attempts per code, then burn it"), so doubling the request-issuance ceiling doubles the worst-case mail-send cost an attacker can impose — six emails per 15 minutes to one address, forty per hour to one IP — without meaningfully weakening brute-force resistance, which is the threat §19 names this control against.

**Why the trusted-hop count is configuration, not a hardcoded assumption.** RFC §18 leaves the exact platform open — "Fly.io, Railway, or a VM with systemd all work" — each of which puts a different number of hops between the client and the app (a platform edge proxy, an operator's own load balancer, sometimes both). Hardcoding "read the first entry" or "read the last entry" bakes in a specific topology the RFC deliberately doesn't commit to. `TRUSTED_PROXY_HOPS` costs one more env var, matching the four §18 already lists, and turns "how many hops do I trust" into an operational fact set once per deployment rather than a code change.

**Why `/64` and not `/128` or `/56`.** The issue's own framing is the test: "limiting per `/128` limits nothing" for an attacker who can request a new IPv6 address inside a delegation they already control, which residential and hosting ISPs alike typically hand out in `/64` blocks per customer. `/64` is the smallest prefix that still forces an attacker to acquire a genuinely new allocation — not just a new address inside one they already have — to escape a bucket, without being so coarse (`/56`, `/48`) that it risks bucketing unrelated customers of the same upstream ISP together on shared infrastructure that hands out finer-grained blocks.

**Why fail-closed doesn't cost what it looks like it costs.** The issue frames this as "the repo's own fail-closed rule applied to a runtime path... and it has a cost: a database blip locks everyone out of login." That's true of the rule in isolation, but not of this specific application: `/auth/request` already has to write `auth_codes` (§12) — a bcrypt hash and an expiry — to do anything useful at all. If Postgres is unreachable or timing out badly enough to fail a `rate_limits` upsert, it is unreachable or timing out badly enough to fail the `auth_codes` insert three lines later regardless of what the limiter decided. Fail-closed on the limiter doesn't introduce a new single point of failure on the login path; it fails at the point that was already there, slightly earlier. The one case this doesn't cover — the `rate_limits` row specifically contended (not Postgres broadly down) — is exactly the flood scenario the limiter exists to handle, and denying during contention on one key is the correct behavior, not a false failure: contention on `(scope, key)` is isolated to that one email or IP, so it costs nothing to callers not being flooded.

**Why the response doesn't need new machinery for enumeration-safety.** §12.2's identical-response requirement was written for the account-existence question, not the rate-limit question, but the two collapse into the same mechanism for free: whatever code path already renders the same "check your email" body for a real and a fake address is the same code path a rate-limited or fail-closed request falls into, since none of those three cases proceeds to send mail. There is no second response shape to design or keep in sync with the first.

**Why the same buckets cover both `/auth/request` and `/auth/verify`.** RFC §12.2's code block lists "rate limit per email and per IP" under `/auth/verify` separately from its "max 5 attempts per code" line — the two are different mechanisms at different scopes (one per-code, one per-identity), and nothing in the RFC states two separate numeric pairs for the two endpoints. §19's threat table also names one row, "OTP brute force," citing §12.2 for the whole authentication surface rather than splitting it. Reading the limits as shared avoids inventing a second unstated pair of numbers, and it's the more defensible posture regardless: an attacker hammering `/auth/verify` with a stolen or guessed code is exactly as much the threat §12.2 exists to close as one hammering `/auth/request`, and giving them a separate budget per endpoint would double their effective attempt rate against one email or IP.

## Consequences

- **RFC §7.2's schema** gains `rate_limits(scope, key, tokens, updated_at)`, primary key `(scope, key)`, plus its `updated_at` index — a fifth table alongside `invite_links`/`board_notes`/`match_players`'s new columns in the "not derived from `orders`" category, though for a different reason: it isn't authoritative game state at all, and carries nothing `cmd/replay --rebuild` would ever need to touch.
- **RFC §12 gains a new §12.3** stating the algorithm, the two buckets' capacity/refill numbers, the consume statement, the IP key derivation and `TRUSTED_PROXY_HOPS`, the fail-closed policy, and the cleanup sweep.
- **RFC §18's env list** gains `TRUSTED_PROXY_HOPS`.
- **RFC §19's security table** gains a "Rate limiting" row with the concrete mechanism; the existing "OTP brute force" row is unchanged in substance, now backed by something instead of a bare citation.
- **M3's schema migration** gains `rate_limits` directly, no longer part of an undifferentiated "D20 rate-limit table" placeholder in the roadmap.
- **M5's auth handlers** can now be built against a real contract: check both buckets inside the request transaction before any `auth_codes`/outbox write, treat a check error identically to a denial, and reuse `/auth/request`'s existing identical-response path for every non-`200`-worthy outcome. Choosing the actual statement/lock timeout the check runs under, and wiring a metric for limiter-check failures, is M5's to do alongside the rest of the auth handler's connection settings (§17 already tracks the analogous sweeper-side numbers — lock-timeout count, pool saturation) — not specified here, the same boundary D17 drew around its own join-landing markup.
- **Reversible at low cost.** A future decision to change the algorithm, the numbers, or add a scope is a superseding decision, not a rewrite: `(scope, key)` already has the granularity any of those changes would need, and only the consume statement's constants or its `WHERE`/refill expression would move.
