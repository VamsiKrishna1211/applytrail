---
title: MCP tools
---

# The MCP surface

ApplyTrail speaks the Model Context Protocol over streamable HTTP at `/mcp`. It
is not a second implementation: the MCP tools and the REST endpoints are both
thin adapters over `internal/service`, so they cannot drift.

## Connecting

The endpoint authenticates and rate-limits exactly like the REST API. The
transport differs; the trust boundary does not.

### Claude Code, locally

```bash
claude mcp add --transport http applytrail https://<your-host>/mcp \
  --header "Authorization: Bearer at_live_..."
```

### Signing in with Clerk (OAuth)

With [Clerk configured](CONFIGURATION.md#clerk), an MCP client can sign in
instead of carrying a key. Add the server with no header:

```bash
claude mcp add --transport http applytrail https://<your-host>/mcp
```

then run `/mcp` in Claude Code and choose to authenticate. A browser opens on
Clerk's sign-in page; once you are back, the connection carries a Clerk access
token. In claude.ai, add the custom connector with only its URL and leave
**Authentication** on what the form detects.

How a client finds its way, which is what Clerk's `configure_mcp_server`
expects:

1. An unauthenticated call to `/mcp` gets `401` with
   `WWW-Authenticate: Bearer resource_metadata="https://<your-host>/.well-known/oauth-protected-resource/mcp"`.
2. That document names the resource, `https://<your-host>/mcp`, and Clerk as
   its only authorization server.
3. The client registers with Clerk through a client ID metadata document and
   signs the user in with PKCE.
4. Every token Clerk issues names `https://<your-host>/mcp` as its audience,
   and the server refuses a token that does not, so a token minted for another
   server on the same Clerk instance does not work here.

The MCP server URL must be registered on the Clerk instance first, and
`APPLYTRAIL_PUBLIC_URL` must produce exactly that URL.

### The claude.ai connector UI

Use this when a **cloud** routine needs the tools. A cloud session runs in
isolation and never sees your local config, so it reads the connector list on
your account instead.

At [claude.ai/customize/connectors](https://claude.ai/customize/connectors),
add a custom connector:

| Field | Value |
|-------|-------|
| Name | `ApplyTrail` |
| Remote MCP server URL | `https://<your-host>/mcp` |
| Authentication | **None** |
| Header name | `x-api-key` |
| Header value | `at_live_...` — the bare token, no `Bearer` prefix |

Two things trip people up here.

**Authentication must be set to `None`** when you connect with a key. The
form pre-selects "Always required — Detected", because ApplyTrail answers an
unauthenticated request with a `WWW-Authenticate` challenge and the client
reads that as an OAuth server. Without Clerk configured there is no OAuth
server behind it; with Clerk configured there is, and signing in (above) is
the better choice.

**`Authorization` is not selectable** in the header list — the client reserves
it for its own OAuth token. That is why the server accepts `X-Api-Key` as well.

### Which header to use

Both are accepted, and both carry the same key:

- `Authorization: Bearer at_live_...` — the primary channel, tried first.
- `X-Api-Key: at_live_...` — a bare token with no scheme, for clients that
  cannot set `Authorization`.

`Authorization` wins when both are present, so a stale fallback value cannot
shadow an explicit one. A `Bearer `-prefixed value in `X-Api-Key` is rejected
rather than unwrapped: a misconfiguration should fail loudly instead of
half-working.

## The tools

Nine tools, shaped around the steps of a search run rather than mirroring the
REST endpoints. An agent should be able to work through them in order without
reading the API reference.

| Tool | Scope | Purpose |
|------|-------|---------|
| `get_search_profile` | `read` | fetch targeting criteria; optionally open a run to log against |
| `filter_new_postings` | `read` | batch dedupe. Read-only, safe to retry |
| `record_discoveries` | `write` | remember what came back new |
| `record_application` | `write` | record that you applied; accepts the posting inline |
| `update_application_status` | `write` | move through the pipeline, validated |
| `list_applications` | `read` | query the pipeline |
| `get_followups_due` | `read` | applications gone quiet long enough to chase |
| `add_note` | `write` | append a dated note |
| `upsert_contact` | `write` | recruiters and hiring managers |

A `read` credential is enough to check for duplicates, which is the main job. A
search agent needs `write` only because it also records what it finds.

## A search run

```
get_search_profile(name, start_run: true)
  → criteria + search_run_id

  ... the agent searches the web itself ...

filter_new_postings(candidates, search_run_id)
  → one verdict per candidate, correlated by index

record_discoveries(everything judged new or eligible_again)
get_followups_due()
```

### Verdicts

`filter_new_postings` returns one of four verdicts per candidate:

| Verdict | Meaning |
|---------|---------|
| `new` | nothing on record; pursue it |
| `duplicate` | already seen, and policy says do not resurface; skip it |
| `eligible_again` | seen before, but enough time has passed to reconsider |
| `needs_review` | a close but inconclusive match |

**Never treat `needs_review` as automatically new.** That is how a duplicate
application gets sent. The reason field names the words that differed, so the
agent can decide and say which way it went.

Each verdict also reports `match_tier` — how it matched — because an ATS job ID
match is a fact and a title similarity is an inference. See
[DEDUPE.md](DEDUPE.md).

## Behaviour worth relying on

**Everything is safe to retry.** `filter_new_postings` changes nothing.
`record_discoveries` and `record_application` deduplicate internally, so a
retried call after a timeout does not create a second record.

**Results correlate by index.** `results[i]` answers `candidates[i]`. There are
no correlation IDs to thread through.

**Errors are actionable.** A rejected status transition names what is allowed
from the current status, so a model can correct itself rather than retrying the
same call.

**Batches cap at 500 candidates.** Split larger sweeps; the counts accumulate
onto the same `search_run_id`.

## Verifying a deployment by hand

```bash
curl -s https://<your-host>/mcp \
  -H "X-Api-Key: at_live_..." \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",
       "params":{"protocolVersion":"2024-11-05","capabilities":{},
                 "clientInfo":{"name":"probe","version":"1"}}}'
```

A working server returns its capabilities, instructions, and a `serverInfo`
carrying the build version — which is also the quickest way to confirm which
revision is actually serving traffic.
