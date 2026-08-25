---
title: Deployment
---

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

Everything below is free-tier. Substitute your own project id and region; the
examples use `us-east5`, which is the closest Cloud Run region to Neon's
`aws-us-east-2`.

1. **Create a Neon project.** Copy both connection strings: the pooled one for
   the service, the direct one for migrations.

2. **Migrate**, from your laptop:

   ```bash
   APPLYTRAIL_DATABASE_URL="postgres://...neon.tech/neondb?sslmode=verify-full" \
     go run ./cmd/applytrail migrate up
   ```

   Prefer `sslmode=verify-full` over `require`. `require` encrypts but does not
   check the server certificate, which leaves the connection open to an active
   attacker; `verify-full` validates the chain and hostname against the system
   roots, and Neon presents a publicly trusted certificate.

3. **Bootstrap** the tenant and an admin key. The key is printed once:

   ```bash
   APPLYTRAIL_DATABASE_URL="..." go run ./cmd/applytrail bootstrap
   ```

4. **Put the connection string in Secret Manager**, never in an env var flag:

   ```bash
   PROJECT=your-project
   printf '%s' "$POOLED_URL" | gcloud secrets create applytrail-db-url \
     --data-file=- --project $PROJECT
   ```

   Piping from stdin keeps the URL out of your shell history and out of the
   process table.

5. **Give the service its own identity**, holding one binding and nothing else:

   ```bash
   gcloud iam service-accounts create applytrail-run \
     --display-name="ApplyTrail Cloud Run runtime" --project $PROJECT

   gcloud secrets add-iam-policy-binding applytrail-db-url \
     --member="serviceAccount:applytrail-run@$PROJECT.iam.gserviceaccount.com" \
     --role=roles/secretmanager.secretAccessor --project $PROJECT
   ```

   Scope the binding to the secret, not to the project. Running as the default
   compute service account would hand the container every permission that
   account holds.

6. **Build and push.** Do not use `gcloud run deploy --source .`: that looks for
   a `Dockerfile` at the repository root, and ApplyTrail's lives at
   `deploy/Dockerfile`. It would silently fall through to buildpacks and then
   fail on two `main` packages.

   ```bash
   REGION=us-east5
   gcloud artifacts repositories create applytrail \
     --repository-format=docker --location=$REGION --project $PROJECT
   gcloud auth configure-docker $REGION-docker.pkg.dev

   VERSION=$(git describe --tags --always --dirty)
   IMAGE=$REGION-docker.pkg.dev/$PROJECT/applytrail/applytrail:$VERSION

   docker build --platform linux/amd64 -f deploy/Dockerfile \
     --build-arg VERSION=$VERSION -t $IMAGE .
   docker push $IMAGE
   ```

   `--platform linux/amd64` is required when building on Apple Silicon: Cloud
   Run does not run arm64 images, and the failure surfaces only at deploy time.

   The image builds the [browser client](WEB-UI.md) in a stage of its own and
   compiles it into the server binary with `-tags webui`, so the deployment
   serves the UI and the API from one origin with no static host and no CORS.
   Nothing needs to be enabled for this; there is no build argument to set.

7. **Deploy**:

   ```bash
   gcloud run deploy applytrail \
     --image $IMAGE \
     --region $REGION \
     --service-account applytrail-run@$PROJECT.iam.gserviceaccount.com \
     --allow-unauthenticated \
     --set-env-vars APPLYTRAIL_ENV=production,APPLYTRAIL_DB_MAX_CONNS=4 \
     --set-secrets APPLYTRAIL_DATABASE_URL=applytrail-db-url:latest \
     --min-instances 0 --max-instances 2 \
     --cpu 1 --memory 512Mi --concurrency 40 --timeout 60s
   ```

   `--allow-unauthenticated` lets requests reach the container; ApplyTrail's own
   API key check is the actual authentication.

   `--max-instances` is a database setting as much as a cost setting: instances
   multiplied by `APPLYTRAIL_DB_MAX_CONNS` is your connection ceiling on a free
   Postgres tier, and it is also what bounds the bill if the public endpoint
   ever takes sustained traffic.

   Cloud Run injects `PORT`, which the server reads before falling back to
   `APPLYTRAIL_ADDR`.

8. **Open the UI** at the service URL the deploy prints. It asks for a key,
   which should be scoped `read,write` and never `admin` — the browser holds
   it, and an admin key held in a browser can mint more:

   ```bash
   APPLYTRAIL_DATABASE_URL="..." \
     go run ./cmd/applytrail apikey create -label browser-ui -scopes read,write
   ```

   Point it at the **same** database the service reads. A key exists only in
   the database it was created in, so one minted against a local Postgres
   authenticates against nothing on Cloud Run, and the UI will say so.

9. **Point your agent at it**:

   ```bash
   claude mcp add --transport http applytrail https://<service-url>/mcp \
     --header "Authorization: Bearer at_live_..."
   ```

   For the claude.ai connector UI, which will not let you set `Authorization`,
   see [MCP.md](MCP.md).

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
| `APPLYTRAIL_RATE_LIMIT_RPM` | `120` | per credential, per instance |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | **unset means no telemetry is exported at all** |

Invalid configuration fails at boot with every problem listed, rather than on
the first request that touches it.

## Health probes

- `GET /healthz` — liveness. Deliberately checks nothing: a process that can
  serve this is alive, and failing it would make the platform restart a server
  that is merely waiting on its database.
- `GET /readyz` — readiness. Pings Postgres with a 3s timeout.

Neither needs a credential.

**On Cloud Run, configure probes against `/readyz`, not `/healthz`.** Google's
frontend intercepts the exact path `/healthz` and answers it itself, so the
request never reaches the container: `/healthz` returns Google's HTML 404 while
`/healthz/` and every other route pass through normally. `/readyz` is the more
useful probe anyway, since it is the one that proves the database is reachable.

## Issuing keys against a deployed instance

The CLI talks to the **database**, not to the service, so where the service runs
does not matter. There is also no shell in the container: the image is
distroless, which is deliberate.

### From your machine (simplest)

Point the CLI at the same database the deployment uses:

```bash
export APPLYTRAIL_DATABASE_URL="postgres://...neon.tech/neondb?sslmode=verify-full"

applytrail apikey create --label search-agent --scopes read,write
applytrail apikey list
applytrail apikey revoke <id>
```

Nothing needs to be redeployed. Authentication reads the key table on every
request, so a new key works immediately and a revoked one stops immediately.

Keep the connection string in your password manager, not your shell history:

```bash
APPLYTRAIL_DATABASE_URL="$(op read op://private/applytrail/neon-url)" \
  applytrail apikey create --label search-agent --scopes read,write
```

### Over the API (no database access needed)

Once bootstrapped, an admin key can mint further keys, which is the right path
for rotation or anything automated:

```bash
curl -s https://<service-url>/v1/admin/api-keys \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"label":"search-agent","scopes":["read","write"]}'
```

The response carries `secret` once. `GET` the same path to list keys — the
listing never returns anything usable — and `DELETE /v1/admin/api-keys/{id}` to
revoke.

This is the only path that does not require the database URL on the machine
issuing the key, so it is the one to give a CI job.

### As a Cloud Run Job

If you would rather the database credentials never sit on a laptop, the image
already contains the CLI at `/applytrail`; only the entrypoint differs.

```bash
gcloud run jobs deploy applytrail-admin \
  --source . \
  --region us-central1 \
  --set-secrets APPLYTRAIL_DATABASE_URL=applytrail-db-url:latest \
  --command /applytrail \
  --args migrate,up

gcloud run jobs execute applytrail-admin --region us-central1 --wait
```

This is a good way to run **migrations** on deploy.

It is a bad way to create a **key**: the CLI prints the secret to stdout, and a
job's stdout goes to Cloud Logging, where it is retained and readable by anyone
with log access. Use one of the two methods above for keys.

## Hardening

The defaults above are the free-tier happy path. Four things are worth doing
before this holds anything you would mind losing.

**Do not run as the database owner.** The connection string a managed Postgres
hands you is usually the owner role, which can `DROP` every table. ApplyTrail
only ever needs DML at runtime; migrations are the one thing that needs more,
and they run from your laptop or a job, not from the serving container.

```sql
CREATE ROLE applytrail_app LOGIN PASSWORD '...';
GRANT USAGE ON SCHEMA public TO applytrail_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO applytrail_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO applytrail_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO applytrail_app;
```

Point `APPLYTRAIL_DATABASE_URL` at `applytrail_app` and keep the owner
credential for `migrate up`.

**Give every credential an expiry.** `apikey create --expires` costs nothing and
turns a leaked key into a problem with an end date. Revocation is checked on
every request, so rotation needs no redeploy.

**Restrict who can reach the database.** Managed Postgres is usually open to the
internet with only the password in the way. If your provider offers an IP
allowlist and your platform offers static egress, use both. On Cloud Run static
egress costs money, which is the honest reason this is a recommendation rather
than a step above.

**Know what the rate limits do and do not cover.** `APPLYTRAIL_RATE_LIMIT_RPM`
is per credential and per instance: several replicas multiply the effective
budget, and a cold start resets it. Authentication failures are governed by a
separate, smaller, service-wide budget so credential guessing is not free — a
caller holding a valid key never touches it.

## Operations

```bash
applytrail migrate status              # what has been applied
applytrail apikey list                 # who can call this deployment
applytrail apikey revoke <id>          # takes effect immediately
```

Revocation is checked on every request, so it needs no restart and no cache
invalidation.

Keys can also carry an expiry, which is worth doing for anything automated:

```bash
applytrail apikey create --label ci --scopes read --expires 2027-01-01
```
