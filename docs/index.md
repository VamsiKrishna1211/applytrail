# ApplyTrail

**A job application registry with a real dedupe engine, built for agent-driven
job searches.**

If you have an agent searching job boards on a schedule, it has no memory
between runs. It re-surfaces the same postings every morning, and eventually
re-applies to a job you already applied to.

ApplyTrail is the memory. It answers one question well — *have I seen this job
before?* — with a confidence-scored verdict, then records what happened next and
hands back the follow-ups you owe.

ApplyTrail does **not** scrape job boards. Your agent searches; ApplyTrail
remembers.

## Documentation

| | |
|---|---|
| [DEDUPE](DEDUPE.md) | how matching works, and why each rule is the way it is |
| [ARCHITECTURE](ARCHITECTURE.md) | layers, ports, and the decisions behind them |
| [MCP](MCP.md) | the tool surface, and connecting an agent to it |
| [ROUTINE](ROUTINE.md) | driving it from a Claude Code cloud routine |
| [DEPLOY](DEPLOY.md) | running it for $0, and hardening it |

The REST surface is specified in
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
starts.

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

## Source

[github.com/VamsiKrishna1211/applytrail](https://github.com/VamsiKrishna1211/applytrail)
— Apache-2.0.
