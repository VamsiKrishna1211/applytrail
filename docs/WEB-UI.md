---
title: Web UI
---

# Web UI

A browser client for the pipeline: the funnel and the follow-up queue on one
page, a board on another, and one drawer that opens an application's timeline
from either.

It is a thin client in the strict sense. It calls the same `/v1` endpoints an
agent calls, holds no rules of its own, and has no privileges an API key does
not already grant. Nothing in `internal/service` knows it exists.

## Running it

```bash
make web-install     # once
make run-all         # builds the client, then serves it at http://localhost:8080
```

`make run-all` compiles the bundle into the server binary, so the UI and the
API answer on the same origin. There is no second process, no static host, and
no CORS configuration anywhere.

For a hot-reloading loop, run the two apart:

```bash
make run       # the API on :8080
make web-dev   # the client on :5173, proxying /v1 to :8080
```

Vite proxies the API in development, so the browser still sees one origin and
the server still needs no CORS.

## Signing in

There is no browser login. The API authenticates with API keys, so the UI asks
for one and sends it as `Authorization: Bearer …` exactly as any other client
would.

**Issue it `read` and `write`, never `admin`:**

```bash
applytrail apikey create -label browser-ui -scopes read,write
```

The key is held in `sessionStorage` and is gone when the tab closes. That is
better than `localStorage`, which would survive a shared machine, but it is not
a security boundary: anything that can execute script on the page can read the
key it is holding. An admin key could mint more credentials from there; a
read+write key cannot.

The document is served with a content security policy that permits no inline
script and no third-party origin, which is what keeps that budget at zero. See
[Security](SECURITY.md) for the wider trust model.

If the server rejects a key, the client drops it and returns to the unlock
screen rather than leaving a signed-in UI that fails every request — and says
why, quoting the error code, the message and the request id. Dropping the key
silently was worse than not dropping it: the unlock appeared to work and then
bounced, with nothing to read.

The commonest cause of a refusal is not a bad key but the wrong database. A key
exists only in the database it was created in, so one minted against local
Postgres is refused by a deployment reading Neon, and the reverse. The unlock
screen names this, because the key itself looks perfectly valid.

## What it shows

| Page | Reads | Writes |
|---|---|---|
| Top-bar search | `GET /v1/applications` (every status) | — |
| Dashboard | `GET /v1/stats`, `GET /v1/applications/followups` | — |
| Pipeline | `GET /v1/applications?status=…` | `PATCH /v1/applications/{id}` |
| Detail drawer | `GET /v1/applications/{id}` | `PATCH /v1/applications/{id}`, `POST /v1/applications/{id}/notes` |

The funnel reports every status, empty ones included, because that is what the
endpoint returns and a chart that changes shape as data arrives is harder to
read than one that does not.

The board splits into **Active** (`queued` through `offer`) and **Closed**
(`accepted`, `rejected`, `ghosted`, `withdrawn`). `discovered` and
`screened_out` appear in neither: those are triage states, and a posting in
them has no application to move.

## Search

The box in the top bar searches every application, on any page, across
company, role, location, resume and cover-letter variant, notes and status.
Terms match independently and as substrings, so `goog staff` finds a staff role
at Google in either order. `/` focuses it from anywhere, arrow keys walk the
results, Enter opens the highlighted one and Escape clears it.

Choosing a result opens the detail drawer, which is where a status is changed
by hand — so a company can be found and moved without knowing which board it
was sitting on, or whether it was on one at all.

The Pipeline's own box, beside **Active/Closed**, is a different gesture on a
smaller subject: it narrows the board in front of you rather than searching the
pipeline, matching with the same rules over the cards already loaded. It says
`12 of 40` while it is filtering, because a board of empty columns otherwise
reads as a board with nothing on it, and Escape clears it. A card the filter
hides takes the move bar with it and gets it back when the filter is cleared.

It searches every status, not the current view, and it fetches nothing until
two characters are typed. That one request covers the most recent 200
applications and every keystroke after it is local: the list endpoint's only
text parameter is an exact company match, so substring search has to happen in
the browser. The footer says how many were searched and offers **search older**
while pages remain, rather than implying a whole pipeline was covered.

## Both pages scroll in place

Neither page is a document. The window does not scroll; the parts do.

On **Pipeline** the top bar, the page head and the move bar are fixed rows, and
each status column scrolls its own cards with its heading pinned. A long
`applied` column no longer drags every other column's heading off the screen,
and the move bar is where it was when you selected the card. Columns hold a
minimum width rather than dividing whatever is there, so five columns on a
narrow window scroll sideways instead of squeezing to widths nothing can be
read at.

On the **Dashboard** the head and the four tiles are fixed, and the three
panels below them each scroll inside their own bounds. That matters most for
the funnel and the queue: the funnel is the control that filters the queue
beside it, and reading to the bottom of a fifty-row queue used to take the
control with it.

The variant table takes only the height its rows need rather than a fixed
share, so a handful of variants leaves the space to the lists above — but it is
capped and scrolls beyond that, because `/v1/stats` returns one row per resume
variant ever used and puts no limit on how many that is. Its column headings
pin themselves while its rows move.

Below 64rem both dashboard columns stack, and a frame stops helping: four
scroll boxes sharing a phone's height is worse than a page that simply scrolls.
At that width the Dashboard hands the scroll back to the page.

## Light and dark

The tokens follow `prefers-color-scheme`, and the control beside **sign out**
overrides it: system, light, dark, then back to system. "Follow the system" is
the default and needs no stored value, so nothing is written until a side is
picked.

The choice is held in `localStorage` — unlike the credential, which is
deliberately in `sessionStorage` — because a theme is a preference rather than
a secret, and one that died with the tab would be worth less than none. It is
applied by `web/public/theme.js` in `<head>`, ahead of the first paint and long
ahead of React, so a chosen theme does not flash the other one on every load.
That is a file rather than an inline script because the CSP permits no inline
script; being same-origin, it needs no exception.

## Transitions

Selecting a card offers exactly the moves
[the status graph](DATA-MODEL.md#the-status-graph) permits from that status.
Illegal targets are absent, not disabled — a greyed-out control invites a click
and then explains why it was refused, while an absent one never made the
promise.

This is a convenience, not an authority. The service still validates every
transition, and a card that moved in another tab produces a `409` that the UI
reports rather than swallows.

The bar also carries the posting's link, beside the company and the current
status, so which listing a card stands for is visible at the moment you decide
where it goes. The URL is free text whoever recorded the application supplied,
so `postingLink` in `web/src/ui/format.ts` parses it and renders a link only
for `http` and `https` — a `javascript:` URL becomes no link at all. The label
drops the scheme and any leading `www.` and truncates; the full URL is the
`title`. A posting with no usable link says so rather than showing nothing.

The client's copy of the graph lives in `web/src/core/domain/status.ts` and is
compared against `internal/domain/application.go` by a Go test. Change the Go
file first; the test names the mismatch if you forget the other.

## How it is built

```
web/src/
  api/         types generated from api/openapi.yaml — never edited by hand
  core/        domain types, ports, and pure use cases; no React, no fetch
  infra/       the adapters: fetch transport, repositories, credential storage
  app/         the composition root and the React context that carries it
  ui/          components, hooks, pages
```

The dependency arrow points inwards. `core/` names no concrete class, `ui/`
resolves interfaces from context and constructs nothing, and the only function
that says `new HttpApplicationsRepository(...)` is `createServices` in
`app/services.ts`. Swapping the transport, or handing a test a stub pipeline,
is a change to that one function.

`src/api/schema.d.ts` is generated by `make web-api` and committed, so the
container image needs no generator. CI regenerates it and fails on a diff, so
it cannot drift from `api/openapi.yaml`.

## Building it into the binary

The bundle is embedded behind the `webui` build tag:

```bash
make web                                   # writes internal/transport/webui/dist
go build -tags webui ./cmd/applytrail-server
```

Without the tag nothing is embedded, `webui.Handler()` returns nil, and the
router treats it the way it treats any absent dependency: every unmatched path
stays a JSON 404. That is what lets `go build ./...` and `go test ./...` work
on a clone that has never run npm.

`make build-all` and the production `Dockerfile` do both halves. API misses
under `/v1`, `/mcp` and the health probes always answer in the JSON envelope,
UI or not — a routing mistake should never come back as an HTML document.

## In a deployment

Nothing about the UI is deployment-specific. The production image builds the
client in a `node` stage, copies the bundle to where the embed directive
expects it, and compiles the server with `-tags webui`; the result is a single
distroless binary that serves both surfaces on one origin. Opening the service
URL is all there is to it — see [Deployment](DEPLOY.md).

Two consequences worth stating:

- **The UI is served unauthenticated, and that is correct.** It is a static
  document with no data in it. The credential is supplied by whoever opens it
  and spent on `/v1`, which authenticates as it always has. Cloud Run's
  `--allow-unauthenticated` governs whether requests reach the container, not
  whether they are trusted.
- **The bundle's URL is the deployment's URL,** so the client always talks to
  the server that served it. There is no API base to configure, no environment
  substitution at boot, and no way for a stale build to point at a different
  backend.
