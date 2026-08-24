---
title: Data model
---

# Data model

Nine entities, one tenant boundary, and one rule that shapes all of it:

> **A posting is a job as it exists in the world. An application is *your
> relationship* to that job.**

Keeping those separate is what makes deduplication tractable. The same job can
be found four times on four boards and stay one posting; you apply to it once,
and that application accumulates a timeline while the posting stays a fact
about the world.

## The entities

| Entity | Is | Deduplicated by |
|---|---|---|
| **Tenant** | the owner of everything below | slug |
| **Company** | an employer | normalized name, per tenant |
| **Posting** | a job in the world | canonical URL, and separately by fingerprint |
| **Application** | your relationship to a posting | one per posting |
| **ApplicationEvent** | one entry in a timeline | append-only, never deduplicated |
| **Contact** | a recruiter or hiring manager | per company |
| **SearchProfile** | targeting criteria | name, per tenant |
| **SearchRun** | one sweep's counters | not deduplicated |
| **APIKey** | a credential | SHA-256 hash of the secret |

Every table carries `tenant_id`, and every query filters on it. The tests
isolate by tenant rather than by database, which means a query that forgets the
filter fails a test rather than leaking in production.

## Company

The anchor for fuzzy matching. Normalization strips legal suffixes and
punctuation, so `Acme, Inc.` and `acme` resolve to one record.

This is load-bearing: two postings are **only** compared for title similarity
when their companies already agree. Without that gate, "Senior Software
Engineer" at two unrelated employers would score as a near-certain match.

## Posting

The thing being deduplicated. It stores both what the caller sent and what
normalization made of it, because each serves a different tier:

| Field | Why it exists |
|---|---|
| `raw_url` / `canonical_url` | canonical is the raw URL with tracking parameters stripped and components normalized. Unique per tenant, and the first match tier. |
| `source` + `external_id` | the ATS identity — a Greenhouse job ID is the same job however it was surfaced. |
| `title` / `normalized_title` | normalized has abbreviations expanded and level markers stripped. Feeds both the fingerprint and the trigram index. |
| `location_raw` / `location_bucket` | the bucket is `remote`, a normalized city, or a country. |
| `fingerprint` | `sha256(normalized company \| normalized title \| location bucket)`. Unique per tenant. |
| `description_hash` | catches the same job cross-posted under different titles. |
| `first_seen_at` / `last_seen_at` | drive the resurfacing cooldown. |
| `raw` | whatever the caller sent, preserved verbatim. |

That last one is the quiet one. Keeping the original payload means a future
extractor can backfill compensation or seniority from data you already have,
without re-fetching a posting that may no longer exist.

`source` accepts values it does not recognise rather than rejecting them, so a
new job board can be recorded before anyone writes its ID extractor.

**Recognised sources:** `greenhouse`, `lever`, `ashby`, `workday`,
`smartrecruiters`, `workable`, `linkedin`, `indeed`, `company_site`,
`referral`, `other`.

**Enrichment**, all optional: `remote_type` (`remote`, `hybrid`, `onsite`),
`seniority` (`intern` through `executive`), a compensation range in whole
annualized currency units, `visa_sponsorship`, and free-form `tech_tags`.

### Candidate

The unsaved input shape, from a search result. Fields are deliberately loose,
because that is what a search result is. A candidate needs a URL, or else both
a title and a company — enough to identify a job, in whichever form the caller
actually has.

## Application

One per posting, carrying status, dates, the resume variant you used, and the
follow-up SLA.

### The status graph

Transitions are validated. An impossible move is rejected with the list of
moves that *are* possible from where you are.

```
discovered ──┬─▶ screened_out ──┬─▶ queued ──┐
             ├─▶ queued ────────┘            │
             ├─▶ applied ◀───────────────────┘
             └─▶ withdrawn

applied ──┬─▶ screening ──┬─▶ interview ⟲ ──┬─▶ offer ──┬─▶ accepted ■
          ├─▶ interview ──┤                 │           └─▶ rejected ■
          ├─▶ offer ──────┘                 │
          ├─▶ rejected ■                    └─▶ rejected ■
          ├─▶ ghosted ─────▶ back into screening / interview / offer
          └─▶ withdrawn ■
```

Three details are deliberate:

**`interview` self-loops.** Multiple rounds are normal, and each one is worth
its own event. Re-asserting any other status is rejected as a no-op rather than
silently accepted, so a confused agent hears about it.

**`ghosted` is not terminal.** Companies do come back after two months of
silence, and a model should be able to record that without inventing a new
application.

**`accepted`, `rejected`, and `withdrawn` are terminal.** Nothing moves out of
them.

### The event timeline

Append-only. Events are never updated or deleted, because the timeline is the
audit trail that lets an agent reason about how long a stage has been running —
which is the whole basis of follow-ups.

| Type | Recorded when |
|---|---|
| `created` | the application was opened |
| `status_changed` | a validated transition, carrying `from` and `to` |
| `note_added` | a dated note |
| `contact_logged` | a recruiter or hiring manager was associated |
| `follow_up_sent` | you chased it |
| `rediscovered` | the posting resurfaced in a later sweep |

Each event records an `actor`: an API key label, `system`, or a human name. So
"who moved this to `rejected`" has an answer.

## Resurfacing policy

A cooldown in days per status, deciding whether a previously seen posting is
worth showing again. The asymmetry is the point: a role you never judged is
cheap to reconsider, one you deliberately skipped costs more, and one that
rejected you should not come back at all.

| Status | Default cooldown | Reasoning |
|---|---|---|
| `discovered` | 30 days | never judged; cheap to reconsider |
| `screened_out` | 90 days | you looked and passed |
| `ghosted` | 120 days | reposted after silence is a real signal |
| `withdrawn` | 180 days | circumstances change |
| `queued` | never | still on your to-do list |
| `applied`, `screening`, `interview`, `offer` | never | in flight |
| `accepted`, `rejected` | never | settled |

A search profile overrides any subset of these; unmentioned statuses fall back
to the defaults above. A cooldown of `-1` means never.

The reason string is written to be read by a person or a model, not parsed:

```
last seen 47d ago in status "screened_out", within the 90d cooldown
```

## Search profiles and runs

A **profile** is targeting criteria: keywords, titles, locations, seniority
band, remote-only, minimum compensation, company allowlist and blocklist,
disqualifying keywords, and the resurfacing policy. It lives server-side so
criteria can be tuned by editing data rather than by editing an agent's prompt,
and it is fetched **by name** so a prompt can name it.

A **run** is a set of counters — candidates seen, new, duplicate, needs review —
not a container. Open one, pass its ID to `postings:filter`, and the counts
accumulate. That is how you learn that a morning sweep produced ninety
candidates and two worth reading.

## API keys

Only a SHA-256 hash is stored, alongside a short non-secret prefix for telling
keys apart in a log. Comparison is constant time.

Hashing is deliberately **not** memory-hard. These are 40+ character
`crypto/rand` tokens, not human-chosen passwords, so an offline attacker has no
structure to grind against and Argon2 would only add latency to every request.

Revocation and expiry are checked per request, with no cache. There is nothing
to invalidate and no redeploy to wait for.

## Schema and queries

Migrations live in `db/migrations` and are embedded in the CLI binary, so a
fresh clone can migrate with no `goose` installed. Queries live in `db/queries`
and are compiled to typed Go by `sqlc`; nothing hand-writes SQL strings.

Optional query parameters need `sqlc.narg('name')::type` rather than `@name` —
a plain `@name` generates a non-pointer field, which cannot express "leave this
column unchanged".

Every migration writes its `Down` section, and `make migrate-down` is part of
how the schema is verified.
