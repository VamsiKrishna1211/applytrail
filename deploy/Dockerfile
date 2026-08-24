# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# cache module downloads separately from source changes
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
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
