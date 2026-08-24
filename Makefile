# ApplyTrail — developer tasks
# Requires: go 1.25+, docker, sqlc, goose, golangci-lint (see `make tools`)

MODULE      := github.com/VamsiKrishna1211/applytrail
SERVER_BIN  := bin/applytrail-server
CLI_BIN     := bin/applytrail
COMPOSE     := docker compose -f deploy/docker-compose.yml
MIGRATIONS  := db/migrations
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

# load .env if present so DATABASE_URL etc. are available to recipes
ifneq (,$(wildcard .env))
include .env
export
endif

# Migrations run through the CLI rather than the goose binary: the CLI embeds
# the migration files and already knows the local docker-compose default, so
# `make migrate` works on a fresh clone with no .env and no goose installed.
# The goose binary is still needed to author a new migration file.
APPLYTRAIL := go run ./cmd/applytrail

.DEFAULT_GOAL := help

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F: '{printf "  \033[36m%-16s\033[0m%s\n", $$1, $$2}'

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

## migrate-new: create a migration (usage: make migrate-new NAME=add_foo)
migrate-new:
	@test -n "$(NAME)" || (echo "usage: make migrate-new NAME=add_foo" && exit 1)
	goose -dir $(MIGRATIONS) create $(NAME) sql

## sqlc: regenerate typed query code from db/queries
sqlc:
	sqlc generate

## build: compile server and CLI into ./bin
build:
	go build -ldflags '$(LDFLAGS)' -o $(SERVER_BIN) ./cmd/applytrail-server
	go build -ldflags '$(LDFLAGS)' -o $(CLI_BIN) ./cmd/applytrail

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

## check: fmt + lint + short tests, what CI runs
check: fmt lint test-short

## docker-build: build the production container image
docker-build:
	docker build -f deploy/Dockerfile -t applytrail:$(VERSION) .

.PHONY: help tools up down nuke psql migrate migrate-down migrate-status migrate-new bootstrap \
        sqlc build run test test-short cover lint fmt check docker-build
