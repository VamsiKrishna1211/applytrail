# ApplyTrail

**A job application registry with a real dedupe engine, built for agent-driven job searches.**

📖 **[Documentation](https://vamsikrishna1211.github.io/applytrail/)** ·
[Getting started](https://vamsikrishna1211.github.io/applytrail/GETTING-STARTED.html) ·
[REST API](https://vamsikrishna1211.github.io/applytrail/API.html) ·
[MCP tools](https://vamsikrishna1211.github.io/applytrail/MCP.html) ·
[Web UI](https://vamsikrishna1211.github.io/applytrail/WEB-UI.html)

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
make up          # postgres 17
make migrate
make bootstrap   # prints an admin key, once
make run
```

No configuration needed: every command defaults to the database `make up`
starts. Set `APPLYTRAIL_DATABASE_URL` to point elsewhere.

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

## Browser UI

Optional, and off by default. A single-page client for the pipeline — the
funnel, the follow-up queue, a board whose moves are the ones the API accepts,
and an application's event timeline. A search in the top bar finds an
application in any status; both pages scroll their panels rather than the
window; light and dark follow the system until you say otherwise.

```bash
make web-install     # once
make run-all         # builds the client into the binary, serves it on :8080
```

It calls the same `/v1` endpoints an agent calls and holds no rules of its own.
Sign in with a key scoped `read,write` — never `admin`:

```bash
go run ./cmd/applytrail apikey create --label browser-ui --scopes read,write
```

`make run` still builds and serves the API alone.

Deployments get it without asking: the production image builds the client in a
stage of its own and compiles it in, so a Cloud Run service answers the UI and
the API on one origin. See
[Web UI](https://vamsikrishna1211.github.io/applytrail/WEB-UI.html) and
[Deployment](https://vamsikrishna1211.github.io/applytrail/DEPLOY.html).

## Connect an agent

```bash
claude mcp add --transport http applytrail https://<your-host>/mcp \
  --header "Authorization: Bearer at_live_..."
```

The key may also travel in an `X-Api-Key` header, carrying the bare token with
no scheme. That exists for clients which reserve `Authorization` for their own
OAuth token and will not let you set it — claude.ai's custom connector is one.
See [MCP tools](docs/MCP.md).

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

See [Cloud routines](docs/ROUTINE.md) for a complete routine prompt.

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

### 📖 [vamsikrishna1211.github.io/applytrail](https://vamsikrishna1211.github.io/applytrail/)

The full site, with navigation and cross-links. Every page below is also plain
Markdown in [`docs/`](docs/), so it reads on GitHub too — there is exactly one
copy of each.

| | |
|---|---|
| [Getting started](https://vamsikrishna1211.github.io/applytrail/GETTING-STARTED.html) | from a fresh clone to a deduped posting in five minutes |
| [Configuration](https://vamsikrishna1211.github.io/applytrail/CONFIGURATION.html) | every environment variable, its default, and its bounds |
| [Deduplication](https://vamsikrishna1211.github.io/applytrail/DEDUPE.html) | how matching works, and why each rule is the way it is |
| [Data model](https://vamsikrishna1211.github.io/applytrail/DATA-MODEL.html) | entities, the status graph, and the event timeline |
| [Architecture](https://vamsikrishna1211.github.io/applytrail/ARCHITECTURE.html) | layers, ports, and the decisions behind them |
| [REST API](https://vamsikrishna1211.github.io/applytrail/API.html) | every endpoint, its scope, and the error envelope |
| [MCP tools](https://vamsikrishna1211.github.io/applytrail/MCP.html) | the tool surface, and connecting an agent to it |
| [CLI](https://vamsikrishna1211.github.io/applytrail/CLI.html) | migrations, tenants, credentials, and profiles |
| [Web UI](https://vamsikrishna1211.github.io/applytrail/WEB-UI.html) | the browser client, how it authenticates, and how it is embedded |
| [Deployment](https://vamsikrishna1211.github.io/applytrail/DEPLOY.html) | running it for $0, and hardening it |
| [Cloud routines](https://vamsikrishna1211.github.io/applytrail/ROUTINE.html) | driving it from a scheduled Claude Code session |
| [Security](https://vamsikrishna1211.github.io/applytrail/SECURITY.html) | the trust model, and reporting a vulnerability |
| [Contributing](https://vamsikrishna1211.github.io/applytrail/CONTRIBUTING.html) | adding a job board, changing the schema, running the tests |
| [`api/openapi.yaml`](api/openapi.yaml) | the machine-readable REST contract |

## Deployment

One container, one Postgres, designed to cost nothing: Cloud Run scales to zero
and Neon's free tier has no expiry. See [Deployment](docs/DEPLOY.md).

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

See [CONTRIBUTING](docs/CONTRIBUTING.md).

## Security

Report vulnerabilities privately — see [SECURITY](docs/SECURITY.md), which also
describes the trust model and the two deployment settings most worth getting
right.

## License

[Apache-2.0](LICENSE)
