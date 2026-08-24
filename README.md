# ApplyTrail

**A job application registry with a real dedupe engine, built for agent-driven job searches.**

If you have an agent — a Claude Code cloud routine, a cron script, anything —
searching job boards on a schedule, it has no memory between runs. It
re-surfaces the same postings every morning, and eventually re-applies to a job
you already applied to.

ApplyTrail is the memory. It answers one question well:

> **Have I seen this job before?**

with a confidence-scored verdict, then records what happened next and hands back
the follow-ups you owe.

ApplyTrail does **not** scrape job boards. Your agent searches; ApplyTrail
remembers.

---

## Why it is not trivial

The same job looks different depending on where you find it:

```
https://boards.greenhouse.io/acme/jobs/4012345
  "Senior Software Engineer II" · Acme, Inc. · Remote

https://linkedin.com/jobs/view/9988776655?trk=public_jobs
  "Sr. SWE II" · Acme · Remote - US
```

Different URL, different job ID, different title, different company spelling,
different location string. Same job.

ApplyTrail catches it:

```jsonc
// POST /v1/postings:filter
{"candidates":[{"url":"https://linkedin.com/jobs/view/9988776655",
                "title":"Sr. SWE II","company":"Acme","location":"Remote - US"}]}

// →
{"results":[{
  "index": 0,
  "verdict": "duplicate",
  "match_tier": "fingerprint",
  "confidence": 0.95,
  "application_status": "applied",
  "days_since": 14,
  "reason": "same normalised role \"senior software engineer\" at \"acme\" in \"remote\"; status \"applied\" never resurfaces"
}]}
```

## What it does

- **Five-tier matching.** Canonical URL → ATS job ID → normalized fingerprint →
  content hash → trigram-shortlisted title similarity. Each tier reports how it
  matched, because an ATS ID match is a fact and a title similarity is an
  inference.
- **Honest uncertainty.** A borderline fuzzy match returns `needs_review` and
  names the words that differed, instead of guessing. A wrong "duplicate"
  silently costs an opportunity; a wrong "new" costs a duplicate application.
- **Policy-driven resurfacing.** A role you skipped 90 days ago can come back. A
  role that rejected you does not.
- **Full pipeline state.** Postings, applications, an append-only event
  timeline, contacts, resume variants, compensation metadata, follow-up SLAs.
- **Two transports, one brain.** REST and MCP are thin adapters over a single
  service layer, so they cannot drift.

## Quick start

Requires Go 1.25+ and Docker.

```bash
make tools                  # sqlc, goose, golangci-lint
make up                     # postgres 17
export APPLYTRAIL_DATABASE_URL='postgres://applytrail:applytrail@localhost:5432/applytrail?sslmode=disable'

go run ./cmd/applytrail migrate up
go run ./cmd/applytrail bootstrap        # prints an admin key, once
make run
```

Create a targeting profile and a key for your agent:

```bash
go run ./cmd/applytrail profile create --name backend-remote \
  --keywords "golang,postgres,distributed systems" \
  --locations Remote --remote-only --min-comp 180000 --currency USD

go run ./cmd/applytrail apikey create --label search-agent --scopes read,write
```

Try it:

```bash
KEY=at_live_...

curl -s localhost:8080/v1/postings:ingest -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' -d '{"candidates":[
    {"url":"https://boards.greenhouse.io/acme/jobs/4012345",
     "title":"Senior Software Engineer II","company":"Acme, Inc.","location":"Remote"}]}'

# the same job, found on LinkedIn tomorrow
curl -s localhost:8080/v1/postings:filter -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' -d '{"candidates":[
    {"url":"https://www.linkedin.com/jobs/view/9988776655",
     "title":"Sr. SWE II","company":"Acme","location":"Remote - US"}]}'
```

`make help` lists every task.

## Connect an agent

```bash
claude mcp add --transport http applytrail https://<your-host>/mcp \
  --header "Authorization: Bearer at_live_..."
```

Nine tools, shaped around the steps of a search run rather than mirroring the
REST endpoints:

| Tool | Purpose |
|------|---------|
| `get_search_profile` | fetch targeting criteria; optionally open a run to log against |
| `filter_new_postings` | **the workhorse** — batch dedupe, read-only, safe to retry |
| `record_discoveries` | remember what came back new |
| `record_application` | record that you applied; accepts the job inline |
| `update_application_status` | move through the pipeline, validated |
| `list_applications` | query the pipeline |
| `get_followups_due` | applications gone quiet long enough to chase |
| `add_note` | append a dated note |
| `upsert_contact` | recruiters and hiring managers |

A read-only credential is enough to check for duplicates, so a search agent
needs no write access to do its main job.

See [`docs/ROUTINE.md`](docs/ROUTINE.md) for a complete routine prompt.

## Architecture

`internal/service` is the only place business logic lives. Everything else
translates for it.

```
agent  ──MCP───▶  transport/mcpserver  ─┐
                                         ├─▶  service  ──▶  dedupe    (Matcher port)
client ──REST──▶  transport/httpapi   ──┘        │           auth      (Authenticator port)
                                                 ├──▶       store     (Store port ─▶ postgres)
                                                 └──▶       ratelimit (Limiter port)
```

Storage, authentication, rate limiting, and match strategy are all ports.
Swapping in a SQLite store, OIDC browser login, a Redis limiter, or an
embedding-based matcher means writing one implementation — not touching the
service layer.

Go 1.25, chi, pgx, sqlc, goose, and the official MCP Go SDK. No web framework
magic, no ORM.

## Documentation

| | |
|---|---|
| [`docs/DEDUPE.md`](docs/DEDUPE.md) | how matching works, and why each rule is the way it is |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | layers, ports, and the decisions behind them |
| [`docs/ROUTINE.md`](docs/ROUTINE.md) | driving it from a Claude Code cloud routine |
| [`docs/DEPLOY.md`](docs/DEPLOY.md) | running it for $0 |
| [`api/openapi.yaml`](api/openapi.yaml) | the REST surface |

## Deployment

One container, one Postgres, designed to cost nothing: Cloud Run scales to zero
and Neon's free tier has no expiry. See [`docs/DEPLOY.md`](docs/DEPLOY.md).

## Testing

```bash
make test         # everything, needs Docker
make test-short   # unit tests only, no Docker
```

Unit tests cover the domain, the dedupe engine, rate limiting, and auth with no
I/O. Everything above runs against a real Postgres via testcontainers, a real
HTTP client, and a real MCP client.

## Contributing

Adding support for a job board is one entry in the extractor table in
[`internal/dedupe/ats.go`](internal/dedupe/ats.go) plus one test case. That is
the design working as intended.

See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

[Apache-2.0](LICENSE)
