---
title: CLI
---

# The `applytrail` CLI

Schema migrations, tenants, credentials, and search profiles. These operations
are deliberately **not** exposed over the API: bootstrapping a deployment has to
work before any credential exists, and a schema change should be a deliberate
operator action rather than something a leaked key can trigger.

```bash
go run ./cmd/applytrail <command> [flags]
# or, after `make build`
./bin/applytrail <command> [flags]
```

Every command reads the database from `APPLYTRAIL_DATABASE_URL`, or from
`--database-url`, and falls back to the local compose default. The migrations
are embedded in the binary, so a fresh clone needs no `goose` installed and no
`.env` present.

```
commands:
  migrate     apply, roll back, or inspect schema migrations
  bootstrap   create the first tenant and an admin key
  apikey      create, list, and revoke credentials
  profile     manage search profiles
  version     print the build version
```

Interrupting is safe. Signals cancel the operation in flight, so a migration
against a slow database can be stopped without leaving the shell hanging.

## `migrate`

```bash
applytrail migrate [up|down|status|version] [--database-url ...]
```

| Sub-command | Effect |
|---|---|
| `up` | apply every pending migration (the default) |
| `down` | roll back the most recent migration |
| `status` | show which migrations have been applied |
| `version` | print the current schema version |

`down` is real, tested, and expected to work: `make migrate-down` is part of how
the schema is verified. Every migration must therefore write its `Down` section
properly.

Migrations do not run from the serving container. Run them from your machine or
a one-off job, which is what lets the runtime connect as a role with no DDL
rights at all — see [Hardening](DEPLOY.md#hardening).

## `bootstrap`

```bash
applytrail bootstrap [--name default] [--slug default] [--label bootstrap]
```

Creates the first tenant and an admin key. Run it once, after `migrate up`.

It refuses to run a second time — if a tenant already exists it says so and
points at `apikey create` — so it cannot quietly mint a second admin credential
on a deployment that already has one.

The secret is printed once. Only a SHA-256 hash is stored, so it cannot be
recovered; issue a new key instead.

## `apikey`

```bash
applytrail apikey create [flags]
applytrail apikey list   [--tenant <slug>]
applytrail apikey revoke <key-id> [--tenant <slug>]
```

### `apikey create`

| Flag | Default | Meaning |
|---|---|---|
| `--label` | *(empty)* | what this key is for, e.g. `search-agent` |
| `--scopes` | `read,write` | comma-separated: `read`, `write`, `admin` |
| `--expires` | *(never)* | `YYYY-MM-DD` |
| `--tenant` | *(the only one)* | tenant slug; optional while one tenant exists |

```bash
applytrail apikey create --label search-agent --scopes read,write --expires 2027-01-01
```

Set an expiry on every key. It turns a leaked credential into a problem with an
end date rather than an open one. A search agent needs `read` and `write`;
nothing an agent does needs `admin`.

### `apikey list`

Shows each key's ID, prefix, label, scopes, status — `active`, `expired`, or
`revoked` — and when it was last used. The prefix is the leading fragment of the
secret, enough to tell two keys apart in a log without being enough to use one.
The last-used column is how you find the credential nobody needs any more.

### `apikey revoke`

```bash
applytrail apikey revoke 593886fd-2668-4026-83b4-c0d894e4703c
```

Effective on the next request. Revocation and expiry are checked per request
with no cache, so there is nothing to invalidate and no redeploy to wait for.

## `profile`

```bash
applytrail profile create --name <name> [flags]
applytrail profile list [--tenant <slug>]
applytrail profile show <name> [--tenant <slug>]
```

Search criteria live server-side so they can be tuned by editing data rather
than by editing an agent's prompt. That distinction matters more than it looks:
a criteria change becomes a database write instead of a prompt edit, a redeploy,
and a hope that the model reads the new wording the same way.

| Flag | Meaning |
|---|---|
| `--name` | **required.** Unique, and how an agent fetches it |
| `--description` | what this profile targets |
| `--keywords` | comma-separated search keywords |
| `--titles` | comma-separated target job titles |
| `--locations` | comma-separated target locations |
| `--seniority` | `junior`, `mid`, `senior`, `staff`, `principal`, `lead`, `manager` |
| `--remote-only` | only consider fully remote roles |
| `--min-comp` | minimum acceptable annual compensation |
| `--currency` | ISO currency code for `--min-comp` |
| `--allow-companies` | employers to consider exclusively |
| `--block-companies` | employers to skip |
| `--exclude-keywords` | keywords that disqualify a posting |

```bash
applytrail profile create --name backend-remote \
  --keywords "golang,postgres,distributed systems" \
  --titles "senior software engineer,staff engineer" \
  --locations Remote --remote-only \
  --min-comp 180000 --currency USD \
  --block-companies "Initech" \
  --exclude-keywords "clearance required,on-site"
```

An agent then fetches it with the `get_search_profile` tool, or
`GET /v1/search-profiles/backend-remote`.

Resurfacing policy — how long a posting in a given status waits before it may
come back — is set through `PATCH /v1/search-profiles/{id}`, not through a flag.
See [Deduplication](DEDUPE.md#resurfacing).

## Make targets

`make help` lists them all. The ones worth knowing:

| Target | Does |
|---|---|
| `make up` / `make down` / `make nuke` | local Postgres; `nuke` also deletes the volume |
| `make migrate` / `make migrate-status` | schema |
| `make bootstrap` | first tenant and admin key |
| `make run` | run the server from source |
| `make test` | everything, needs Docker |
| `make test-short` | unit tests only, no Docker |
| `make check` | fmt, lint, and short tests — what CI runs |
| `make sqlc` | regenerate typed queries from `db/queries` |
| `make migrate-new NAME=...` | author a migration |
| `make docker-build` | build the production image |

`make tools` installs `sqlc`, `goose`, and `golangci-lint`. You need them only
to regenerate queries, author a migration, or lint.
