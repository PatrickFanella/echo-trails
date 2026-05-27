SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO_APPS := apps/api apps/worker
TS_APPS := apps/admin-web apps/public-web
OPENAPI_DIR := packages/openapi
IOS_PROJECT := apps/ios/EchoTrails.xcodeproj
IOS_SCHEME := EchoTrails
IOS_DESTINATION := platform=iOS Simulator,name=iPhone 16

.PHONY: help verify fmt fmt-check lint test db-migration-check docs-check open-pilot-check secrets-check go-check ts-check openapi-check swift-check

help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"; printf "Commands:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

verify: fmt-check lint test db-migration-check docs-check open-pilot-check secrets-check openapi-check swift-check ## Run all checks

fmt: ## Format files where repo tooling is available
	@for app in $(GO_APPS); do \
		if [ -f "$$app/go.mod" ]; then \
			files=$$(find "$$app" -name '*.go' -type f); \
			if [ -n "$$files" ]; then gofmt -w $$files; else echo "skip gofmt: $$app has no Go files"; fi; \
		else \
			echo "skip gofmt: $$app/go.mod not present yet"; \
		fi; \
	done

fmt-check: ## Verify formatting without rewriting files
	@for app in $(GO_APPS); do \
		if [ -f "$$app/go.mod" ]; then \
			files=$$(find "$$app" -name '*.go' -type f); \
			if [ -n "$$files" ]; then \
				unformatted=$$(gofmt -l $$files); \
				if [ -n "$$unformatted" ]; then printf '%s\n' "$$unformatted"; exit 1; fi; \
			else \
				echo "skip gofmt check: $$app has no Go files"; \
			fi; \
		else \
			echo "skip gofmt check: $$app/go.mod not present yet"; \
		fi; \
	done

lint: ## Run lightweight lint checks
	@for app in $(GO_APPS); do \
		if [ -f "$$app/go.mod" ]; then \
			echo "go vet: $$app"; \
			(cd "$$app" && go vet ./...); \
		else \
			echo "skip go vet: $$app/go.mod not present yet"; \
		fi; \
	done
	@for app in $(TS_APPS); do \
		if [ -f "$$app/package.json" ]; then \
			echo "TypeScript lint/build: $$app"; \
			(cd "$$app" && npm run lint && npm run build); \
		else \
			echo "skip TypeScript lint/build: $$app/package.json not present yet"; \
		fi; \
	done

test: ## Run deterministic tests available on this host
	@for app in $(GO_APPS); do \
		if [ -f "$$app/go.mod" ]; then \
			echo "go test: $$app"; \
			(cd "$$app" && go test ./...); \
		else \
			echo "skip go test: $$app/go.mod not present yet"; \
		fi; \
	done
	@for app in $(TS_APPS); do \
		if [ -f "$$app/package.json" ]; then \
			echo "TypeScript test: $$app"; \
			(cd "$$app" && npm test -- --run); \
		else \
			echo "skip TypeScript tests: $$app/package.json not present yet"; \
		fi; \
	done

db-migration-check: ## Execute SQL migrations against embedded Postgres/PostGIS when scaffolded
	@if [ -f apps/api/package.json ]; then cd apps/api && npm ci && npm run test:migrations; else echo 'skip DB migration check: apps/api/package.json not present yet'; fi

docs-check: ## Verify required docs exist
	@test -f README.md
	@test -f AGENTS.md
	@test -f CONTRIBUTING.md
	@test -f SECURITY.md
	@test -f CONTEXT.md
	@test -f PLAN.md
	@test -f docs/README.md
	@test -f docs/PRD.md
	@test -f docs/technical-plan.md
	@test -f docs/open-pilot.md
	@test -f docs/labels.md
	@test -f docs/runbooks/README.md
	@test -f docs/adr/README.md
	@for adr in docs/adr/[0-9][0-9][0-9][0-9]-*.md; do \
		test -f "$$adr"; \
		grep -q '^## Status$$' "$$adr"; \
		grep -q '^## Context$$' "$$adr"; \
		grep -q '^## Decision$$' "$$adr"; \
		grep -q '^## Consequences$$' "$$adr"; \
	done

open-pilot-check: ## Verify Open Pilot templates exist
	@test -f .gitea/ISSUE_TEMPLATE/open-pilot-task.yaml
	@grep -q 'Test Command' .gitea/ISSUE_TEMPLATE/open-pilot-task.yaml
	@test -f .gitea/PULL_REQUEST_TEMPLATE.md
	@test -f .gitea/workflows/template-check.yml
	@grep -q 'make verify' .gitea/workflows/template-check.yml

secrets-check: ## Check for common committed secret filenames
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		! git ls-files | grep -E '(^|/)(\.env|.*\.pem|.*\.key|.*\.p12|.*\.pfx|id_rsa|id_ed25519)$$'; \
	else \
		echo 'skip secrets-check: not a git repository'; \
	fi

go-check: ## Run Go formatting, vet, and tests when Go modules are scaffolded
	@for app in $(GO_APPS); do \
		if [ -f "$$app/go.mod" ]; then \
			files=$$(find "$$app" -name '*.go' -type f); \
			if [ -n "$$files" ]; then \
				unformatted=$$(gofmt -l $$files); \
				if [ -n "$$unformatted" ]; then printf '%s\n' "$$unformatted"; exit 1; fi; \
			fi; \
			echo "go check: $$app"; \
			(cd "$$app" && go vet ./... && go test ./...); \
		else \
			echo "skip Go checks: $$app/go.mod not present yet"; \
		fi; \
	done

ts-check: ## Run TypeScript/web checks when package.json files are scaffolded
	@for app in $(TS_APPS); do \
		if [ -f "$$app/package.json" ]; then \
			echo "TypeScript check: $$app"; \
			(cd "$$app" && npm run lint && npm run build && npm test -- --run); \
		else \
			echo "skip TypeScript checks: $$app/package.json not present yet"; \
		fi; \
	done

openapi-check: ## Run OpenAPI checks when the contract package is scaffolded
	@if [ -d "$(OPENAPI_DIR)" ]; then \
		if [ ! -f "$(OPENAPI_DIR)/openapi.yaml" ] && [ ! -f "$(OPENAPI_DIR)/openapi.yml" ]; then \
			echo 'OpenAPI package exists but no openapi.yaml or openapi.yml was found'; exit 1; \
		fi; \
		if [ -f "$(OPENAPI_DIR)/package.json" ]; then \
			cd "$(OPENAPI_DIR)" && npm run lint; \
		else \
			echo 'OpenAPI contract present; skip npm lint because package.json is not present yet'; \
		fi; \
	else \
		echo 'skip OpenAPI checks: packages/openapi/ not present yet'; \
	fi

swift-check: ## Run Swift/Xcode checks on macOS and skip safely elsewhere
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		if [ -d "$(IOS_PROJECT)" ]; then \
			xcodebuild test -project "$(IOS_PROJECT)" -scheme "$(IOS_SCHEME)" -destination "$(IOS_DESTINATION)"; \
		else \
			echo 'skip Swift checks: iOS project not present yet'; \
		fi; \
	else \
		echo 'skip Swift/Xcode checks: non-macOS runner'; \
	fi
