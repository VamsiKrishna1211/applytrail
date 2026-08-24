---
title: Configuration
---

# Configuration

Every setting is an environment variable with a documented default that works
for local development, so `go run ./cmd/applytrail-server` succeeds against a
stock `make up` database with no `.env` file present.

Invalid values are rejected at startup, all of them at once, with the variable
named. A deployment that starts is a deployment that is configured.

## Core

| Variable | Default | Notes |
|---|---|---|
| `APPLYTRAIL_DATABASE_URL` | local compose DSN | Any Postgres connection string. The one setting you almost certainly need. |
| `APPLYTRAIL_ENV` | `development` | `development` or `production`. |
| `APPLYTRAIL_ADDR` | `:8080` | Listen address. Ignored when `PORT` is set. |
| `PORT` | — | Wins over `APPLYTRAIL_ADDR`. Cloud Run injects it; nothing else needs it. |
| `APPLYTRAIL_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

The default DSN is
`postgres://applytrail:applytrail@localhost:5432/applytrail?sslmode=disable`,
matching `deploy/docker-compose.yml`. Those credentials are that compose file's
local throwaways and are not a secret.

## Connection pool

| Variable | Default | Bounds |
|---|---|---|
| `APPLYTRAIL_DB_MAX_CONNS` | `8` | `>= 1`, clamped at 1000 |
| `APPLYTRAIL_DB_MIN_CONNS` | `0` | `0 <= min <= max` |

A minimum of zero is deliberate. On a scale-to-zero platform an idle instance
holding open connections is pure cost, and on a serverless Postgres with a
connection cap it is the thing most likely to exhaust it. Raise it only if cold
connection setup shows up in your latency.

## Dedupe thresholds

These govern the fuzzy tier only. The four exact tiers above it are not tunable
— they either match or they do not.

| Variable | Default | Meaning |
|---|---|---|
| `APPLYTRAIL_FUZZY_MATCH_THRESHOLD` | `0.87` | At or above this similarity, a candidate is a `duplicate`. |
| `APPLYTRAIL_FUZZY_REVIEW_THRESHOLD` | `0.75` | Between this and the match threshold, the verdict is `needs_review`. |

Both must be within `(0, 1]`, and review must not exceed match.

The gap between them is where the service admits it does not know. Narrowing it
does not make the engine more accurate; it just moves borderline cases from
"you decide" into "the service decided". Widening it sends more work to the
caller. See [Deduplication](DEDUPE.md#the-fuzzy-tier) for what actually gets
compared.

## Rate limiting

| Variable | Default | Meaning |
|---|---|---|
| `APPLYTRAIL_RATE_LIMIT_RPM` | `120` | Sustained requests per minute, per credential. |
| `APPLYTRAIL_RATE_LIMIT_BURST` | `60` | Tokens available at once. |

A token bucket per API key, held in process. Several replicas therefore
multiply the effective budget and a cold start resets it — correct for the
single-instance deployment this is built for, and the reason `ratelimit.Limiter`
is a port rather than a concrete type.

Authentication *failures* are governed separately by a smaller service-wide
budget, because the per-credential limiter cannot see a request that has no
valid credential. That budget is not env-tunable: it is a correctness setting,
not a knob, and a deployment that raises it is only widening a guessing window.

## Telemetry

| Variable | Default | Meaning |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty)* | Empty disables export entirely. No endpoint, no cost. |
| `OTEL_SERVICE_NAME` | `applytrail` | Resource name on exported spans and metrics. |

Traces name spans after the matched route pattern, so
`/v1/applications/{id}` is one span name rather than one per application.

## Not configurable, on purpose

HTTP timeouts and the shutdown grace period are compiled in:

| Setting | Value |
|---|---|
| Read header timeout | 5s |
| Read timeout | 15s |
| Write timeout | 30s |
| Idle timeout | 60s |
| Shutdown timeout | 20s |

These are correctness settings rather than tuning knobs. A request body is
capped at 4 MiB, which comfortably holds the largest realistic payload — a few
hundred candidates with descriptions — while bounding per-request memory.

## Loading it

The `Makefile` sources a `.env` in the repository root if one exists, and
exports it to every recipe. Keep real credentials there; `.gitignore` and
`.dockerignore` both exclude it, and `.dockerignore` is an allowlist so a new
secret file is excluded by default rather than by remembering to add it.

```bash
# .env
APPLYTRAIL_DATABASE_URL=postgres://user:pass@host/db?sslmode=verify-full
```

`sslmode=verify-full` validates the certificate chain and hostname. `require`
only encrypts, and accepts whatever certificate it is handed. Use `verify-full`
against any managed provider that supports it.

For the CLI, `--database-url` overrides the environment on any command.
