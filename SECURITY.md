# Security

## Reporting a vulnerability

Report privately through [GitHub's private vulnerability
reporting](https://github.com/VamsiKrishna1211/applytrail/security/advisories/new).
Please do not open a public issue for anything exploitable.

Include what you did, what happened, and what you expected. A proof of concept
helps but is not required. Expect an acknowledgement within a week.

This is a personal project with no paid maintainer and no bug bounty. What it
can promise is a straight answer and a fix or a documented "won't fix", rather
than silence.

## The trust model

ApplyTrail is a single-tenant service that one person's agents call. It is not
designed to be a multi-tenant SaaS, and a few decisions only make sense with
that in mind.

**API keys are the whole authentication story.** There is no browser login, no
OAuth server, no session. A key is 40+ characters from `crypto/rand`, stored as
a SHA-256 hash, compared in constant time. Hashing is deliberately not
memory-hard: these are high-entropy random tokens, not human-chosen passwords,
so an offline attacker has nothing to grind against and Argon2 would only add
latency to every request.

**Scopes are coarse on purpose.** `read`, `write`, `admin`, where `admin`
implies the others. Only `admin` reaches `/v1/admin/api-keys`, so a credential
handed to an agent cannot mint more credentials.

**Revocation and expiry are checked on every request.** There is no cache to
invalidate, so a revoked key stops working immediately with no redeploy.

**Rate limiting is per credential and in-process.** Several replicas multiply
the effective budget and a cold start resets it, which is correct for the
single-instance deployment this is built for and is why `ratelimit.Limiter` is
a port. Authentication *failures* are governed separately, by a smaller
service-wide budget, because the per-credential limiter cannot see a request
that has no valid credential.

## Deploying it safely

The two things most likely to bite, both covered in
[docs/DEPLOY.md](docs/DEPLOY.md#hardening):

**Do not point `APPLYTRAIL_DATABASE_URL` at the database owner role.** Managed
Postgres hands you an owner credential by default. ApplyTrail needs only DML at
runtime; migrations are the one thing that needs more, and they do not run from
the serving container.

**Set an expiry on every key.** `apikey create --expires` turns a leaked
credential into a problem with an end date.

If you expose the service publicly — which you must, for a cloud agent to reach
it — the API key is the only thing between the internet and your data. Bound
the blast radius with `--max-instances`, and give agents `read` where `read` is
enough.

## What is out of scope

- Denial of service against a deliberately public endpoint. Bound it at the
  platform layer.
- Anything requiring an already-valid `admin` credential.
- The example `docker-compose.yml` credentials. They are local throwaways and
  are documented as such.
