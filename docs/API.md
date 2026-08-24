---
title: REST API
---

# REST API

Everything under `/v1` is JSON in, JSON out, authenticated with an API key. The
machine-readable contract — every schema, every field — is
[`api/openapi.yaml`](https://github.com/VamsiKrishna1211/applytrail/blob/main/api/openapi.yaml).
This page is the map: what exists, what scope it needs, and the conventions that
hold everywhere.

The REST endpoints and the [MCP tools](MCP.md) are both thin adapters over
`internal/service`. They cannot disagree about a rule, because neither of them
implements one.

## Authenticating

```bash
curl https://your-host/v1/stats -H "Authorization: Bearer at_live_..."
```

A bare token in `X-Api-Key` also works, for clients that reserve `Authorization`
for their own OAuth token. `Authorization` wins when both are present, so a
stale fallback cannot shadow an explicit value. A `Bearer `-prefixed value in
`X-Api-Key` is rejected rather than unwrapped — a misconfiguration should fail
loudly instead of half-working.

Three scopes, where `admin` implies the other two:

| Scope | Grants |
|---|---|
| `read` | every `GET`, plus `POST /v1/postings:filter` |
| `write` | recording postings, applications, notes, contacts, profiles, runs |
| `admin` | everything, plus `/v1/admin/api-keys` |

Batch dedupe is deliberately read-only, so a search agent can do its main job
with a credential that cannot mutate anything.

## Conventions

**Errors** use one envelope, always:

```jsonc
{"error": {
  "code": "invalid_argument",
  "message": "validation failed",
  "details": ["title must not be empty", "url is not absolute"],
  "request_id": "01J9F..."
}}
```

`details` carries every field-level problem at once, so a caller learns all of
them in one round trip rather than one per retry. `request_id` matches the
`X-Request-Id` response header and the server's logs.

| Code | Status | Cause |
|---|---|---|
| `invalid_argument` | 400 | malformed body, failed validation, bad parameter |
| `unauthenticated` | 401 | missing, unknown, revoked, or expired credential |
| `permission_denied` | 403 | valid credential, insufficient scope |
| `not_found` | 404 | no such record, or no route |
| `method_not_allowed` | 405 | the path exists, the verb does not |
| `conflict` | 409 | a uniqueness or state conflict |
| `rate_limited` | 429 | budget exhausted; `Retry-After` says how long |
| `internal` | 500 | logged in full, reported generically |

**Unknown JSON fields are rejected.** A typo in a payload is a 400, not a
silently ignored field.

**Bodies cap at 4 MiB**, and batch endpoints cap at 500 candidates.

**Timestamps are RFC 3339, UTC.** Query parameters that take a time want the
same.

**Pagination is a cursor.** List endpoints return `next_cursor` when a full
page came back; feed it to `?cursor=`. An absent `next_cursor` means the end.

## Health

Unauthenticated, outside instrumentation, so a platform can always reach them.

| Endpoint | Answers |
|---|---|
| `GET /healthz` | the process is up |
| `GET /readyz` | the process is up **and** the database answered a ping |
| `GET /version` | the build version |

On Cloud Run, `/healthz` is intercepted by Google's frontend before it reaches
your container. Use `/readyz` — see [Deployment](DEPLOY.md#health-probes).

## Postings

| Endpoint | Scope | Purpose |
|---|---|---|
| `POST /v1/postings:filter` | `read` | batch dedupe; changes nothing |
| `POST /v1/postings:ingest` | `write` | record what came back new |

```jsonc
POST /v1/postings:filter
{"candidates": [
   {"url": "https://linkedin.com/jobs/view/9988776655",
    "title": "Sr. SWE II", "company": "Acme", "location": "Remote - US"}],
 "search_run_id": "..."}          // optional; accumulates run counters
```

```jsonc
{"results": [{
  "index": 0,
  "verdict": "duplicate",
  "match_tier": "fingerprint",
  "confidence": 0.95,
  "application_status": "applied",
  "days_since": 14,
  "reason": "same normalised role \"senior software engineer\" at \"acme\""
}]}
```

`results[i]` answers `candidates[i]`. There are no correlation IDs to thread
through, and the ordering is guaranteed.

Four verdicts — `new`, `duplicate`, `eligible_again`, `needs_review`. **Never
treat `needs_review` as automatically new**; that is how a duplicate
application gets sent. [Deduplication](DEDUPE.md) covers the tiers, the
thresholds, and the resurfacing policy.

Both endpoints are safe to retry. `filter` mutates nothing, and `ingest`
deduplicates internally, so a retry after a timeout does not create a second
record.

## Applications

| Endpoint | Scope | Purpose |
|---|---|---|
| `GET /v1/applications` | `read` | query the pipeline |
| `GET /v1/applications/followups` | `read` | applications quiet long enough to chase |
| `GET /v1/applications/{id}` | `read` | one application with its event timeline |
| `POST /v1/applications` | `write` | record that you applied; accepts the posting inline |
| `PATCH /v1/applications/{id}` | `write` | move through the pipeline, validated |
| `POST /v1/applications/{id}/notes` | `write` | append a dated note |

`GET /v1/applications` parameters:

| Parameter | Default | Notes |
|---|---|---|
| `status` | — | repeatable: `?status=applied&status=screening` |
| `company` | — | normalized employer name |
| `since` | — | RFC 3339 |
| `limit` | `50` | |
| `cursor` | — | from a previous `next_cursor` |

`GET /v1/applications/{id}` takes `event_limit` (default `100`).
`GET /v1/applications/followups` takes `due_before` (RFC 3339, defaults to now)
and `limit` (default `50`).

A `PATCH` that requests an impossible transition is a 400 whose `details` name
what *is* allowed from the current status, so a model can correct itself rather
than retrying the same call:

```jsonc
{"error": {"code": "invalid_argument",
           "message": "cannot move from \"applied\" to \"accepted\"; allowed: [screening interview offer rejected ghosted withdrawn]",
           "details": ["cannot move from \"applied\" to \"accepted\"; allowed: [...]"]}}
```

The full graph is in [Data model](DATA-MODEL.md#the-status-graph).

## Search profiles

| Endpoint | Scope | Purpose |
|---|---|---|
| `GET /v1/search-profiles` | `read` | list; `?active=true` filters |
| `GET /v1/search-profiles/{name}` | `read` | fetch by name, not by ID |
| `POST /v1/search-profiles` | `write` | create |
| `PATCH /v1/search-profiles/{id}` | `write` | partial update, by ID |

Fetching by name is what makes a profile usable from a prompt: an agent asks
for `backend-remote`, not for a UUID it would have to be told.

`PATCH` is genuinely partial. An omitted field is unchanged; a field set to an
empty array clears it. Every criterion is updatable, including
`resurface_policy`, which maps a status to the number of days before a posting
in it may resurface.

## Search runs

| Endpoint | Scope | Purpose |
|---|---|---|
| `GET /v1/search-runs` | `read` | recent runs; `?since=` and `?limit=` (default 25) |
| `POST /v1/search-runs` | `write` | open a run |
| `PATCH /v1/search-runs/{id}` | `write` | close it, optionally with notes |

A run is a counter, not a container. Pass its ID to `postings:filter` and the
seen, new, duplicate, and needs-review counts accumulate onto it — which is how
you find out that a morning sweep returned ninety candidates and two worth
looking at.

## Contacts and stats

| Endpoint | Scope | Purpose |
|---|---|---|
| `GET /v1/contacts` | `read` | `?company_id=`, `?limit=` (default 50) |
| `POST /v1/contacts` | `write` | upsert a recruiter or hiring manager |
| `GET /v1/stats` | `read` | pipeline counts and funnel rates |

## Admin

`admin` scope only, so a credential handed to an agent cannot mint more
credentials.

| Endpoint | Purpose |
|---|---|
| `GET /v1/admin/api-keys` | list keys and their status |
| `POST /v1/admin/api-keys` | issue one; the secret is in this response and nowhere else |
| `DELETE /v1/admin/api-keys/{id}` | revoke, effective on the next request |

Only a SHA-256 hash of a key is stored. `POST` is the one and only time the
secret exists outside the caller.

Schema migrations, tenant creation, and the very first credential are **not**
here — they live in [the CLI](CLI.md). Bootstrapping has to work before any
credential exists, and a schema change should be a deliberate operator action
rather than something a leaked key can trigger.

## Rate limits

A token bucket per credential: 120 requests per minute sustained, 60 available
at once, both configurable. Exhausting it returns 429 with `Retry-After` in
seconds.

Authentication failures draw on a separate, smaller, service-wide budget.
It is deliberately not per-client-address: `X-Forwarded-For` is caller-supplied
and spoofable, so a per-IP budget would be bypassed by the only attacker it was
meant to stop. A caller holding a valid credential never touches it.

The limiter is in-process, so replicas multiply the effective budget and a cold
start resets it. That is correct for a single-instance deployment, and it is
why `ratelimit.Limiter` is a port.
