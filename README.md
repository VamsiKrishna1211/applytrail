# ApplyTrail

**A job application registry with a real dedupe engine, built for agent-driven job searches.**

If you have an agent (a Claude Code cloud routine, a cron script, anything) that searches job boards on a schedule, it has no memory between runs. It re-surfaces the same postings, and eventually re-applies to a job you already applied to.

ApplyTrail is the memory. It answers one question well —

> **Have I seen this job before?**

— with a confidence-scored verdict, then records what happened next and hands back the follow-ups you owe.

ApplyTrail does **not** scrape job boards. Your agent searches; ApplyTrail remembers.

---

## What it does

- **Layered dedupe.** Canonical URL → ATS job ID → normalized fingerprint → content hash → trigram fuzzy match. A "Sr. SWE II" on LinkedIn and a "Senior Software Engineer II" on the company's Greenhouse board are recognised as the same job.
- **Honest uncertainty.** Borderline fuzzy matches return `needs_review` instead of guessing, so the caller adjudicates with the facts in hand.
- **Policy-driven resurfacing.** A role you skipped 6 months ago can become eligible again; one that rejected you does not.
- **Full pipeline state.** Postings, applications, an append-only event log, contacts, resume variants, comp metadata, and follow-up SLAs.
- **Two transports, one brain.** A REST API and a Model Context Protocol server, both thin adapters over a single service layer.

## Interfaces

**MCP** (for agents) — nine workflow-shaped tools at `POST /mcp`:

```bash
claude mcp add --transport http applytrail https://your-host/mcp \
  --header "Authorization: Bearer at_live_..."
```

**REST** (for everything else) — `/v1`, bearer auth, cursor pagination. See [`api/openapi.yaml`](api/openapi.yaml).

The hot path is batch dedupe:

```jsonc
// POST /v1/postings:filter
{"candidates":[{"url":"...","title":"Sr. SWE II","company":"Acme, Inc.","location":"Remote"}]}

// →
{"results":[{"index":0,"verdict":"duplicate","match_tier":"ats_id","confidence":1.0,
  "application_status":"applied","days_since":14,
  "reason":"same Greenhouse job id; applied 14 days ago"}]}
```

## Quick start

Requires Go 1.25+ and Docker.

```bash
make tools          # sqlc, goose, golangci-lint
make up             # postgres 17 in docker
make migrate
make run
```

Then mint a key and try it:

```bash
go run ./cmd/applytrail apikey create --scopes read,write --label dev
curl -s localhost:8080/healthz
```

Run `make help` for the full task list.

## Architecture

`internal/service` is the only place business logic lives. REST handlers and MCP tools decode, call it, and encode — nothing more.

```
agent  ──MCP──▶  transport/mcpserver  ─┐
                                        ├─▶  service  ──▶  dedupe (Matcher port)
client ──REST─▶  transport/httpapi   ──┘         │         auth   (Authenticator port)
                                                 └──▶      store  (Store port ─▶ postgres)
```

Storage, authentication, rate limiting, and match strategy are all ports. Swapping in a SQLite store, OIDC login, a Redis limiter, or an embedding-based matcher means writing one implementation, not touching the service layer.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) and [`docs/DEDUPE.md`](docs/DEDUPE.md).

## Deployment

Designed to run at zero cost: a single scale-to-zero container plus a free-tier Postgres. See [`docs/DEPLOY.md`](docs/DEPLOY.md).

## Status

Under active development. See the milestone checklist in [`docs/ROADMAP.md`](docs/ROADMAP.md).

## License

[Apache-2.0](LICENSE)
