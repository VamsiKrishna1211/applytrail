---
title: Getting started
---

# Getting started

Five minutes from a fresh clone to a posting that ApplyTrail recognises as a
duplicate of one it has already seen.

You need **Go 1.25+** and **Docker**. Nothing else — the CLI embeds the
migrations, so there is no `goose` or `sqlc` to install unless you intend to
change the schema.

## 1. Bring up the stack

```bash
git clone https://github.com/VamsiKrishna1211/applytrail
cd applytrail

make up          # postgres 17, waits until healthy
make migrate     # apply every migration
make bootstrap   # create the first tenant and print an admin key
```

`make bootstrap` prints a key **once** and stores only its SHA-256 hash. There
is no way to recover it later; issue a new one instead.

```
tenant "default" created (6f9c1e7a-1f4b-4a2e-9f0a-2c8f5b1d3a44)

API key created: bootstrap

  at_live_7yDYxSEV3q...

Store it now. Only a hash is kept, so it cannot be shown again.
```

No configuration is needed for any of this. Every command defaults to the
database `make up` starts. Point somewhere else by setting
`APPLYTRAIL_DATABASE_URL` — see [Configuration](CONFIGURATION.md).

## 2. Issue a key for your agent

The admin key can mint credentials; it should not be the credential an agent
carries. Give the agent the narrowest scope that does its job:

```bash
go run ./cmd/applytrail apikey create \
  --label search-agent --scopes read,write --expires 2027-01-01
```

A search agent needs `read` to check for duplicates and `write` only because it
also records what it finds. Neither reaches the admin endpoints. See
[the CLI reference](CLI.md#apikey) for every flag.

## 3. Create a search profile

Targeting criteria live server-side, so they can be tuned by editing data
rather than by editing an agent's prompt:

```bash
go run ./cmd/applytrail profile create --name backend-remote \
  --keywords "golang,postgres,distributed systems" \
  --titles "senior software engineer,staff engineer" \
  --locations Remote --remote-only \
  --min-comp 180000 --currency USD \
  --exclude-keywords "clearance required,on-site"
```

An agent fetches this with `get_search_profile`, or `GET
/v1/search-profiles/backend-remote`. Without a profile, that first step of a
routine has nothing to return and the run fails at step one.

## 4. Run the server

```bash
make run     # or: go run ./cmd/applytrail-server
```

It listens on `:8080` and logs structured JSON. `GET /healthz` answers
immediately; `GET /readyz` also pings the database.

## 5. Watch the dedupe engine work

Record a posting found on Greenhouse:

```bash
KEY=at_live_...        # the search-agent key from step 2

curl -s localhost:8080/v1/postings:ingest \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"candidates":[{
        "url":"https://boards.greenhouse.io/acme/jobs/4012345",
        "title":"Senior Software Engineer II",
        "company":"Acme, Inc.",
        "location":"Remote"}]}'
```

Now hand it the same job as it appears on LinkedIn the next morning — different
host, different job ID, abbreviated title, differently spelled employer,
differently worded location:

```bash
curl -s localhost:8080/v1/postings:filter \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"candidates":[{
        "url":"https://www.linkedin.com/jobs/view/9988776655?trk=public_jobs",
        "title":"Sr. SWE II",
        "company":"Acme",
        "location":"Remote - US"}]}'
```

```jsonc
{"results":[{
  "index": 0,
  "verdict": "duplicate",
  "match_tier": "fingerprint",
  "confidence": 0.95,
  "reason": "same normalised role \"senior software engineer\" at \"acme\" in \"remote\""
}]}
```

`match_tier` says *how* it matched, because an ATS job ID match is a fact and a
title similarity is an inference. [Deduplication](DEDUPE.md) explains all five
tiers.

## 6. Connect an agent

```bash
claude mcp add --transport http applytrail http://localhost:8080/mcp \
  --header "Authorization: Bearer $KEY"
```

That is the whole integration. [MCP tools](MCP.md) lists the nine tools and
covers connecting a hosted deployment to the claude.ai connector UI, which has
two non-obvious settings.

## Where to go next

- [Deduplication](DEDUPE.md) — what the verdicts mean and when to trust them.
- [REST API](API.md) — every endpoint and its scope.
- [Deployment](DEPLOY.md) — running it on Cloud Run and Neon for nothing.
- [Cloud routines](ROUTINE.md) — a scheduled agent that uses all of it.

## Common problems

**`make migrate` cannot reach the database.** `make up` waits for the container
to report healthy, but a stale container from an earlier run can hold port
5432. `make down` then `make up`, or `make nuke` to also drop the volume.

**`bootstrap` says the deployment is already bootstrapped.** It refuses to run
twice on purpose. Use `apikey create` for further credentials, or `make nuke`
to start over locally.

**A key stopped working.** Revocation and expiry are checked on every request
with no cache, so a revoked key fails immediately. `apikey list` shows the
status of each.

**Tests want Docker.** `make test-short` runs the domain, dedupe, ratelimit and
auth suites with no I/O at all.
