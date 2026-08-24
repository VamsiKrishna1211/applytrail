---
title: Overview
---

# ApplyTrail

A job application registry with a real dedupe engine, built for agent-driven
job searches.
{: .lede }

If you have an agent searching job boards on a schedule, it has no memory
between runs. It re-surfaces the same postings every morning, and eventually
re-applies to a job you already applied to.

ApplyTrail is the memory. It answers one question well — *have I seen this job
before?* — with a confidence-scored verdict, then records what happened next and
hands back the follow-ups you owe.

ApplyTrail does **not** scrape job boards. Your agent searches; ApplyTrail
remembers.

[Get started](GETTING-STARTED.md){: .primary}
[Connect an agent](MCP.md)
[Source on GitHub](https://github.com/VamsiKrishna1211/applytrail)
{: .cta }

## Why it is not trivial

The same job looks different depending on where you find it:

```
https://boards.greenhouse.io/acme/jobs/4012345
  "Senior Software Engineer II" · Acme, Inc. · Remote

https://linkedin.com/jobs/view/9988776655?trk=public_jobs
  "Sr. SWE II" · Acme · Remote - US
```

Different URL, different job ID, different title, different company spelling,
different location string. Same job. ApplyTrail catches it, and tells you which
of five tiers it matched on — because an ATS job ID match is a fact and a title
similarity is an inference.

A borderline match returns `needs_review` and names the words that differed,
instead of guessing. A wrong "duplicate" silently costs an opportunity; a wrong
"new" costs a duplicate application.

## The documentation

Every page is reachable from the sidebar on the left, and the pages are ordered
so that following "next" from here walks the whole set once.

### Start here

- [**Getting started**<br><span>From a fresh clone to a deduped posting in five minutes.</span>](GETTING-STARTED.md)
- [**Configuration**<br><span>Every environment variable, its default, and its bounds.</span>](CONFIGURATION.md)
{: .cards }

### Concepts

- [**Deduplication**<br><span>The five tiers, normalization, and the resurfacing policy.</span>](DEDUPE.md)
- [**Data model**<br><span>Entities, the status graph, and the event timeline.</span>](DATA-MODEL.md)
- [**Architecture**<br><span>Layers, ports, and the decisions behind them.</span>](ARCHITECTURE.md)
{: .cards }

### Reference

- [**REST API**<br><span>Every endpoint, its scope, and the error envelope.</span>](API.md)
- [**MCP tools**<br><span>The nine tools, and connecting an agent to them.</span>](MCP.md)
- [**CLI**<br><span>Migrations, tenants, credentials, and search profiles.</span>](CLI.md)
{: .cards }

### Operate

- [**Deployment**<br><span>Cloud Run and Neon, and how to harden it.</span>](DEPLOY.md)
- [**Cloud routines**<br><span>Driving a search from a scheduled Claude Code session.</span>](ROUTINE.md)
- [**Security**<br><span>The trust model, and reporting a vulnerability.</span>](SECURITY.md)
- [**Contributing**<br><span>Adding a job board, changing the schema, running tests.</span>](CONTRIBUTING.md)
{: .cards }

The REST surface is also specified machine-readably in
[`api/openapi.yaml`](https://github.com/VamsiKrishna1211/applytrail/blob/main/api/openapi.yaml).

## Quick start

Requires Go 1.25+ and Docker.

```bash
make up          # postgres 17
make migrate
make bootstrap   # prints an admin key, once
make run
```

No configuration needed: every command defaults to the database `make up`
starts. Full walkthrough in [Getting started](GETTING-STARTED.md).

## How it fits together

```
agent  ──MCP───▶  transport/mcpserver  ─┐
                                        ├─▶  service  ──▶  dedupe    (Matcher port)
client ──REST──▶  transport/httpapi   ──┘        │           auth      (Authenticator port)
                                                 ├──▶       store     (Store port ─▶ postgres)
                                                 └──▶       ratelimit (Limiter port)
```

Business logic lives in `internal/service` and nowhere else, so the REST and MCP
surfaces cannot drift apart — neither of them implements a rule. Storage,
authentication, rate limiting, and match strategy are all ports.

Go 1.25, chi, pgx, sqlc, goose, and the official MCP Go SDK. No web framework
magic, no ORM.

## Source

[github.com/VamsiKrishna1211/applytrail](https://github.com/VamsiKrishna1211/applytrail)
— Apache-2.0.
