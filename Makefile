SHELL := /bin/bash
.DEFAULT_GOAL := help

GO ?= go
COMPOSE ?= docker compose
MIGRATION_DB_URL ?= postgres://nodus:nodus@localhost:5433/nodus_health?sslmode=disable
export MIGRATION_DB_URL

# Schema owner, used for migrations and by db-role. The API itself connects as
# the least-privileged role in deploy/app_role.sql — see DB_URL in .env.
DB_OWNER ?= nodus
DB_NAME ?= nodus_health

GO_FILES := $(shell find . -type f -name '*.go' -not -path './vendor/*' -not -path './tmp/*')
BUILD_DIR := tmp/bin
COVERAGE_PROFILE := coverage.out
COVERAGE_HTML := coverage.html

.PHONY: help setup deps tidy dev run db-up db-down db-role db-status db-logs import-icd11 \
	migrate-up generate test test-race coverage fmt fmt-check vet check ci \
	build clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Nodus backend tasks\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup: ## Download dependencies, start Postgres, create the runtime role, and apply migrations
	@$(MAKE) deps
	@$(MAKE) db-up
	@$(MAKE) db-role
	@$(MAKE) migrate-up

deps: ## Download Go module dependencies
	$(GO) mod download

tidy: ## Synchronize go.mod and go.sum with the source
	$(GO) mod tidy

dev: ## Run the API with Air live reload
	$(GO) tool air

run: ## Run the API directly
	$(GO) run ./cmd/api

db-up: ## Start Postgres and wait until it is healthy
	$(COMPOSE) up -d --wait db

db-down: ## Stop local services without deleting database data
	$(COMPOSE) down

db-role: ## Create/refresh the least-privileged runtime role RLS depends on
	@$(COMPOSE) exec -T db psql -v ON_ERROR_STOP=1 -U $(DB_OWNER) -d $(DB_NAME) -f - < deploy/app_role.sql

db-status: ## Show local service status
	$(COMPOSE) ps

db-logs: ## Follow Postgres logs
	$(COMPOSE) logs --follow db

migrate-up: ## Apply all pending database migrations
	@$(GO) run ./cmd/migrate

import-icd11: ## Validate/import WHO ICD-11 workbook (FILE=... COMMIT=1)
	@$(GO) run ./cmd/import-icd11 --file "$(FILE)" $(if $(COMMIT),--commit,)

generate: ## Regenerate sqlc database code
	$(GO) tool sqlc generate

test: ## Run all tests
	$(GO) test ./...

test-race: ## Run all tests with the race detector
	CGO_ENABLED=1 $(GO) test -race ./...

coverage: ## Generate coverage.out and coverage.html
	$(GO) test -coverprofile=$(COVERAGE_PROFILE) ./...
	$(GO) tool cover -html=$(COVERAGE_PROFILE) -o $(COVERAGE_HTML)

fmt: ## Format all Go source files
	gofmt -w $(GO_FILES)

fmt-check: ## Fail if any Go source file is not formatted
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [[ -n "$$unformatted" ]]; then \
		echo "The following files need gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet: ## Run Go static analysis
	$(GO) vet ./...

check: fmt-check vet test ## Run the standard local quality checks

ci: fmt-check vet test-race build ## Run the same quality gate used by CI

build: ## Build API and migration binaries under tmp/bin
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BUILD_DIR)/api ./cmd/api
	CGO_ENABLED=0 $(GO) build -o $(BUILD_DIR)/migrate ./cmd/migrate
	CGO_ENABLED=0 $(GO) build -o $(BUILD_DIR)/import-icd11 ./cmd/import-icd11

clean: ## Remove local build and coverage artifacts
	$(RM) -r tmp $(COVERAGE_PROFILE) $(COVERAGE_HTML)
