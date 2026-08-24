# Architecture

## The one rule

`internal/service` is the only place business logic lives. The REST handlers
and the MCP tools decode a request, call one service method, and encode the
result. Nothing more.

That is what guarantees an agent and a `curl` command get identical behaviour,
rather than two implementations that drift apart over a year of small fixes.

```
agent  ──MCP───▶  transport/mcpserver  ─┐
                                         ├─▶  service  ──▶  dedupe   (Matcher port)
client ──REST──▶  transport/httpapi   ──┘        │           auth     (Authenticator port)
                                                 ├──▶       store    (Store port ─▶ postgres)
                                                 └──▶       ratelimit (Limiter port)
```

Both transports mount on the same chi router and sit behind the same
authentication, scope, and rate-limit middleware. The MCP endpoint is not a
side door.

## Layers

| Package | Responsibility | Depends on |
|---------|---------------|------------|
| `internal/domain` | entities, the status machine, resurfacing policy, errors | stdlib + `uuid` |
| `internal/dedupe` | normalizers, ATS extractors, the five match tiers | `domain` |
| `internal/store` | persistence ports | `domain` |
| `internal/store/postgres` | sqlc queries, pgx pool, error translation | `store`, `domain` |
| `internal/service` | use cases, transactions, authorisation | everything below |
| `internal/transport/*` | encode and decode | `service`, `domain` |
| `internal/auth` | `Authenticator` port + API key implementation | `store`, `domain` |

`domain` depends on nothing, so every layer can import it freely. Dependencies
point inward; nothing below `service` knows a transport exists.

## Ports

Four interfaces are the extension points:

- **`store.Store`** — swap Postgres for anything. Sub-stores are returned by
  accessor so a test fake implements only what it needs.
- **`auth.Authenticator`** — API keys today. `internal/auth/oidc` documents
  exactly what a browser-login implementation has to satisfy; no handler
  changes.
- **`dedupe.Matcher`** — replace the tiered engine with, say, an embedding
  model.
- **`ratelimit.Limiter`** — the in-memory token bucket is correct for one
  instance. Multiple replicas need a shared implementation.

## Decisions worth knowing

**Ingest runs in one transaction, and the matcher reads through it.** Two
candidates in one batch that are the same job would otherwise both look new, and
the second insert would collide on the fingerprint constraint.

**`needs_review` writes nothing.** The engine could not tell whether the
candidate duplicates something. Recording either answer is worse than handing
the decision back.

**Status transitions are compare-and-swap.** The update only applies while the
row still reads the expected status, so a concurrent change loses loudly with a
409 instead of overwriting silently.

**API keys are hashed with SHA-256, not argon2.** A key is 256 bits of CSPRNG
output, so there is nothing to guess and a slow KDF buys no security — while
costing tens of milliseconds and tens of megabytes on every authenticated
request. Password KDFs exist for low-entropy human-chosen secrets. Every
rejection path returns one identical message, so the error cannot be used to
probe which keys exist.

**Rate limits key on the credential, not the client IP.** An agent running in a
cloud sandbox shares its address with everyone else there.

**MCP is stateless with JSON responses, not SSE.** On a scale-to-zero platform
consecutive requests can land on different container instances, and a session
map lives in one process only. Stateless costs server-initiated messages, which
none of the nine tools need.

**Telemetry exports nothing by default.** OpenTelemetry is wired throughout, but
no exporter is installed unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. A
free-tier deployment stays free; a production one is one env var away.

**Every table carries `tenant_id` and every query filters on it,** even though
one deployment holds one tenant. Multi-user is then a deployment change rather
than a schema migration.

**Statuses are `text` with `CHECK` constraints, not Postgres enums.** Adding a
value to a native enum is awkward to reverse, and this schema is expected to
grow statuses as people adopt it.

**Some invariants live in the schema as well as the service.** An `applied`
application must carry an `applied_at`. Enforcing that in Postgres too means a
manual SQL fix cannot silently corrupt the funnel report.

## Testing

Unit tests cover `domain`, `dedupe`, `ratelimit`, and `auth` with no I/O. Every
layer above runs against a real Postgres via testcontainers, isolating by tenant
rather than by resetting the schema — faster, and a genuine exercise of the
tenant scoping every query relies on.

The MCP tests drive a real MCP client through the real router. The REST tests
drive a real HTTP client through it. Both have already caught bugs that unit
tests would not have: a nil `text[]` reaching a `NOT NULL` column as SQL NULL,
and nil Go maps marshalling to JSON `null` and failing the tools' own inferred
output schemas.

```bash
make test         # everything, needs Docker
make test-short   # unit tests only, no Docker
```
