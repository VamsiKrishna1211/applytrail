# Deployment

ApplyTrail is one container plus one Postgres. It is built to run at zero cost.

## Recommended: Cloud Run + Neon

**Why this pairing.** Cloud Run runs the same Docker image you develop against,
scales to zero between runs, has an always-free request quota that a personal
job search will not approach, and starts a Go binary in a few hundred
milliseconds. Neon's free tier has no expiry and scales its compute to zero
while keeping the project reachable — Supabase pauses free projects after seven
idle days, which would break a weekday-morning routine.

Fly.io is not on this list any more: the free tier is gone, new accounts get a
short trial, and the cheapest always-on app runs a few dollars a month.

Vercel is a poor fit despite the Go runtime existing: it is serverless functions
only, with no long-lived process, which is awkward for connection pooling.

### Steps

1. **Create a Neon project** and copy the pooled connection string.

2. **Migrate**, from your laptop:

   ```bash
   APPLYTRAIL_DATABASE_URL="postgres://...neon.tech/applytrail?sslmode=require" \
     go run ./cmd/applytrail migrate up
   ```

3. **Bootstrap** the tenant and an admin key. The key is printed once:

   ```bash
   APPLYTRAIL_DATABASE_URL="..." go run ./cmd/applytrail bootstrap
   ```

4. **Deploy**:

   ```bash
   gcloud run deploy applytrail \
     --source . \
     --region us-central1 \
     --allow-unauthenticated \
     --set-env-vars APPLYTRAIL_ENV=production \
     --set-secrets APPLYTRAIL_DATABASE_URL=applytrail-db-url:latest \
     --min-instances 0
   ```

   `--allow-unauthenticated` lets requests reach the container; ApplyTrail's own
   API key check is the actual authentication. Put the database URL in Secret
   Manager, not in an env var flag.

   Cloud Run injects `PORT`, which the server reads before falling back to
   `APPLYTRAIL_ADDR`.

5. **Point your agent at it**:

   ```bash
   claude mcp add --transport http applytrail https://<service-url>/mcp \
     --header "Authorization: Bearer at_live_..."
   ```

### Cold starts

Scale-to-zero means the first request after an idle period pays a cold start —
a Go binary plus a Postgres connection, so a second or two. For a scheduled
routine this is irrelevant. If it ever bothers you, `--min-instances 1` removes
it and stops the deployment being free.

## Alternatives

| Platform | Notes |
|----------|-------|
| **Koyeb** | Free web service, scale-to-zero after 1h idle (not disableable), free Postgres. No card required. Tightest limits of the three. |
| **Render** | Free forever, sleeps after 15 min, wakes in seconds. Simplest deploy UX. Render's own free Postgres expires, so pair it with Neon. |
| **Any VPS** | `docker compose -f deploy/docker-compose.yml up` plus the image. Full control, not free. |

The container is a distroless static binary running as nonroot. It needs no
volumes and no sidecars.

## Configuration

Every setting has a default that works locally; see `.env.example`.

| Variable | Default | Notes |
|----------|---------|-------|
| `APPLYTRAIL_DATABASE_URL` | local docker compose | required in production |
| `PORT` / `APPLYTRAIL_ADDR` | `:8080` | `PORT` wins, for Cloud Run |
| `APPLYTRAIL_ENV` | `development` | `production` drops source locations from logs |
| `APPLYTRAIL_LOG_LEVEL` | `info` | JSON to stdout, parsed by Cloud Logging for free |
| `APPLYTRAIL_DB_MAX_CONNS` | `8` | keep low on a serverless Postgres free tier |
| `APPLYTRAIL_FUZZY_MATCH_THRESHOLD` | `0.87` | at or above this, a fuzzy match is a duplicate |
| `APPLYTRAIL_FUZZY_REVIEW_THRESHOLD` | `0.75` | between the two, it is `needs_review` |
| `APPLYTRAIL_RATE_LIMIT_RPM` | `120` | per credential |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | **unset means no telemetry is exported at all** |

Invalid configuration fails at boot with every problem listed, rather than on
the first request that touches it.

## Health probes

- `GET /healthz` — liveness. Deliberately checks nothing: a process that can
  serve this is alive, and failing it would make the platform restart a server
  that is merely waiting on its database.
- `GET /readyz` — readiness. Pings Postgres with a 3s timeout.

Neither needs a credential.

## Operations

```bash
applytrail migrate status              # what has been applied
applytrail apikey list                 # who can call this deployment
applytrail apikey revoke <id>          # takes effect immediately
```

Revocation is checked on every request, so it needs no restart and no cache
invalidation.
