---
title: Contributing
---

# Contributing

## Setup

```bash
make up        # postgres 17 in docker
make migrate
make test
```

`make tools` installs sqlc, goose, and golangci-lint. You only need them to
regenerate queries, author a new migration, or lint — applying migrations goes
through the CLI, which embeds them.

`make check` runs what CI runs.

## The one rule

**Business logic lives in `internal/service`, and nowhere else.**

REST handlers and MCP tools decode a request, call one service method, and
encode the result. If you find yourself writing a conditional in a handler, it
belongs a layer down. This is what keeps the two transports from drifting apart,
and it is the thing to protect in review.

## Adding support for a job board

One entry in the table in `internal/dedupe/ats.go`:

```go
{
    source: domain.SourceExample,
    hosts:  []string{"jobs.example.com"},
    path:   regexp.MustCompile(`^/(?P<company>[^/]+)/job/(?P<id>\d+)`),
},
```

Plus one case in `TestExtractIdentity`. That is the whole change.

Two things to check:

- **Is the job ID globally unique, or per employer?** If per employer, the
  extractor must capture a `company` group (or set `companyFromSubdomain`), or
  unrelated jobs at different companies will be reported as duplicates.
  `TestIdentityKey_QualifiesByEmployer` covers exactly this for Workday.
- **Does the board carry the job in a query parameter?** If so, add it to
  `query` and make sure it is *not* in `trackingParams` — stripping an identity
  parameter collapses unrelated jobs onto one canonical URL, which silently
  hides real openings.

## Changing the schema

```bash
make migrate-new NAME=add_something
# edit db/migrations/*.sql — write the Down section too, it is tested
make migrate
make sqlc                # regenerate typed queries
```

Write `Down` properly. `make migrate-down` is part of how the schema is
verified.

Optional query parameters need `sqlc.narg('name')::type`, not `@name`. Plain
`@name` generates a non-pointer field, which cannot express "leave unchanged".

## The documentation

The pages in `docs/` are the GitHub Pages site as well as the Markdown GitHub
renders, so there is exactly one copy of everything. Preview it before pushing:

```bash
make docs        # http://localhost:4000, with live reload
```

It runs in a container against the pinned `github-pages` gem, which is the same
Jekyll and the same plugin set the Actions build uses — so what you see locally
is what deploys. The first run installs gems into a named volume; later runs
start immediately.

Adding a page means adding it to `docs/_data/nav.yml`. That file drives the
sidebar, the ordering, and the previous/next links, so a page missing from it is
a page nobody can reach. Front matter carries only `title`, because everything
else the layout needs comes from the nav entry.

Links between pages are written as `.md` and rewritten to `.html` at build time
by `jekyll-relative-links`, which is what lets the same link work on GitHub and
on the site. Write `[Deployment](DEPLOY.md)`, never `DEPLOY.html`.

## Tests

Write the test that would have caught the bug, and say in the assertion message
*why* the behaviour matters — not what the code does.

```go
assert.False(t, eligible,
    "re-applying to a job you already applied to is the failure this service exists to prevent")
```

Unit tests for `domain`, `dedupe`, `ratelimit`, and `auth` must not touch I/O.
Everything above uses `testsupport.Store(t)`, which shares one container across
the run and isolates by tenant. Use `-short` to skip anything needing Docker.

## Style

- Comments explain *why*, not *what*. A comment restating the code is worse than
  none, because it drifts.
- Errors wrap a `domain` sentinel so transports map statuses without knowing
  about SQL.
- Validation collects every problem before returning, so a caller learns all of
  them in one round trip.
- No new dependency without a reason in the PR description. The dependency list
  is short deliberately.

## Commits

Explain the decision, not the diff. A reader in a year needs to know why the
code is shaped this way; `git diff` already tells them what changed.
