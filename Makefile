# ApplyTrail — developer tasks
# Requires: go 1.25+, docker, sqlc, goose, golangci-lint (see `make tools`)

MODULE      := github.com/VamsiKrishna1211/applytrail
SERVER_BIN  := bin/applytrail-server
CLI_BIN     := bin/applytrail
COMPOSE     := docker compose -f deploy/docker-compose.yml
MIGRATIONS  := db/migrations
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

# Load .env, then .env.local, so recipes see the same configuration the server
# reads. .env.local is included last and therefore wins: shared defaults belong
# in .env, machine-specific choices in .env.local.
#
# Which database is in scope is the setting that actually bites here. An API key
# exists only in the database it was created in, so `apikey create` and `run`
# must agree about which one that is — and they do, because both read this.
ifneq (,$(wildcard .env))
include .env
endif
ifneq (,$(wildcard .env.local))
include .env.local
endif
export

# Migrations run through the CLI rather than the goose binary: the CLI embeds
# the migration files and already knows the local docker-compose default, so
# `make migrate` works on a fresh clone with no .env and no goose installed.
# The goose binary is still needed to author a new migration file.
APPLYTRAIL := go run ./cmd/applytrail

.DEFAULT_GOAL := help

## help: list available targets
# Only this file: `include` appends to MAKEFILE_LIST, so grepping all of it
# would scan .env too and prefix every line with a filename, which the awk
# split then reads as the target name.
help:
	@grep -E '^## ' $(firstword $(MAKEFILE_LIST)) | sed 's/## //' | awk -F: '{printf "  \033[36m%-16s\033[0m%s\n", $$1, $$2}'

## tools: install pinned dev tooling into GOBIN
tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

## up: start local postgres and wait for health
up:
	$(COMPOSE) up -d --wait

## down: stop local postgres (keeps volume)
down:
	$(COMPOSE) down

## nuke: stop local postgres and DELETE its data volume
nuke:
	$(COMPOSE) down -v

## psql: open a psql shell against local postgres
psql:
	$(COMPOSE) exec postgres psql -U applytrail -d applytrail

## migrate: apply all pending migrations
migrate:
	$(APPLYTRAIL) migrate up

## migrate-down: roll back the most recent migration
migrate-down:
	$(APPLYTRAIL) migrate down

## migrate-status: show migration state
migrate-status:
	$(APPLYTRAIL) migrate status

## bootstrap: create the first tenant and an admin key
bootstrap:
	$(APPLYTRAIL) bootstrap

## apikey: issue a credential (usage: make apikey LABEL=browser-ui SCOPES=read,write)
# Goes through the same .env / .env.local layering as `run`, so the key lands in
# the database the server will actually read. A key created against one database
# authenticates against no other.
SCOPES ?= read,write
apikey:
	@test -n "$(LABEL)" || (echo 'usage: make apikey LABEL=browser-ui [SCOPES=read,write]' && exit 1)
	$(APPLYTRAIL) apikey create -label '$(LABEL)' -scopes '$(SCOPES)'

## migrate-new: create a migration (usage: make migrate-new NAME=add_foo)
migrate-new:
	@test -n "$(NAME)" || (echo "usage: make migrate-new NAME=add_foo" && exit 1)
	goose -dir $(MIGRATIONS) create $(NAME) sql

## sqlc: regenerate typed query code from db/queries
sqlc:
	sqlc generate

## web-install: install the browser client's dependencies
web-install:
	cd web && npm ci

## web: build the browser client into the package that embeds it
web:
	cd web && npm run build

## web-dev: run the client with hot reload, proxying the API to localhost:8080
web-dev:
	cd web && npm run dev

## web-api: regenerate the client's types from api/openapi.yaml
web-api:
	cd web && npm run api

## web-check: typecheck the browser client
web-check:
	cd web && npm run typecheck

## build: compile server and CLI into ./bin (API only, no browser client)
build:
	go build -ldflags '$(LDFLAGS)' -o $(SERVER_BIN) ./cmd/applytrail-server
	go build -ldflags '$(LDFLAGS)' -o $(CLI_BIN) ./cmd/applytrail

## build-all: build the client and compile it into the server binary
# The tag is what makes the embed directive real. Without it the dist directory
# need not exist, which is why a clone with no npm still builds and tests.
build-all: web
	go build -tags webui -ldflags '$(LDFLAGS)' -o $(SERVER_BIN) ./cmd/applytrail-server
	go build -ldflags '$(LDFLAGS)' -o $(CLI_BIN) ./cmd/applytrail

## run-all: build the client, then run the server serving it at http://localhost:8080
run-all: web
	go run -tags webui ./cmd/applytrail-server

## run: run the server from source
run:
	go run ./cmd/applytrail-server

## test: run the full test suite (needs docker for store tests)
test:
	go test ./... -race -count=1

## test-short: run unit tests only, skipping anything needing docker
test-short:
	go test ./... -short -count=1

## cover: run tests and open an HTML coverage report
cover:
	go test ./... -short -coverprofile=coverage.out
	go tool cover -html=coverage.out

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: format and tidy
fmt:
	go fmt ./...
	go mod tidy

## check: fmt + lint + short tests + client typecheck, what CI runs
check: fmt lint test-short web-check

## docs: preview the documentation site locally on port 4000
# Runs in a container because the site is built by GitHub's Ruby, not the one
# on this machine, and because macOS ships a Ruby too old for the gem set.
# The named volume caches gems so only the first run pays for bundle install.
docs:
	docker run --rm -it \
	  -v "$(CURDIR)/docs":/site -w /site \
	  -v applytrail-docs-gems:/usr/local/bundle \
	  -p 4000:4000 -p 35729:35729 \
	  ruby:3.3 sh -c "bundle install && bundle exec jekyll serve \
	    --host 0.0.0.0 --livereload --force_polling"

## docker-build: build the production container image
docker-build:
	docker build -f deploy/Dockerfile -t applytrail:$(VERSION) .

.PHONY: help docs tools up down nuke psql migrate migrate-down migrate-status migrate-new bootstrap \
        sqlc apikey build build-all run run-all test test-short cover lint fmt check docker-build \
        web web-install web-dev web-api web-check
