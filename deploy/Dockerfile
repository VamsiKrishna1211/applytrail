# syntax=docker/dockerfile:1

# ---- browser client ----
# Built first and separately: the bundle only changes when web/ changes, so a
# Go-only edit reuses this layer entirely.
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
# schema.d.ts is generated from api/openapi.yaml and committed; CI fails if the
# two have drifted, so the image build does not need the document itself.
RUN npm run build

# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# cache module downloads separately from source changes
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# The bundle lands where the //go:embed directive expects it. The `webui` tag
# is what turns that directive on; the CLI never needs it.
COPY --from=web /internal/transport/webui/dist ./internal/transport/webui/dist

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -tags webui -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/applytrail-server ./cmd/applytrail-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/applytrail ./cmd/applytrail

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/applytrail-server /applytrail-server
COPY --from=build /out/applytrail /applytrail
USER nonroot:nonroot

# Cloud Run injects PORT; the server reads it and falls back to APPLYTRAIL_ADDR.
ENV APPLYTRAIL_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/applytrail-server"]
