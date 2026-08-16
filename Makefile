SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO_APPS := apps/api apps/worker
TS_APPS := apps/admin-web apps/public-web
OPENAPI_DIR := packages/openapi
IOS_PROJECT := apps/ios/EchoTrails.xcodeproj
IOS_SCHEME := EchoTrails
IOS_DESTINATION ?=
LOCAL_COMPOSE := infra/local/compose.yaml

.PHONY: help verify fmt fmt-check lint test integration-test restore-drill moderation-outage-drill apns-mock-drill release-service-drills release-scale-test release-processing-test privacy-release-check moderation-check db-migration-check docs-check open-pilot-check secrets-check local-compose-check go-check ts-check openapi-check ios-boundary-check phase5-check swift-check ios-ui-test

help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"; printf "Commands:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

verify: fmt-check lint test privacy-release-check moderation-check db-migration-check docs-check open-pilot-check secrets-check local-compose-check openapi-check ios-boundary-check phase5-check swift-check ## Run all checks

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
			(cd "$$app" && npm ci && npm run lint && npm run build); \
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
			(cd "$$app" && npm ci && npm test -- --run); \
		else \
			echo "skip TypeScript tests: $$app/package.json not present yet"; \
		fi; \
	done

integration-test: ## Run disposable PostGIS, Redis, S3, SMTP, and moderation integration fixtures
	@docker compose -f infra/integration/compose.yaml up -d --wait postgres redis object-storage smtp moderation-mock
	@docker compose -f infra/integration/compose.yaml up -d migrations object-storage-init
	@docker compose -f infra/integration/compose.yaml wait migrations object-storage-init
	@ECHOTRAILS_INTEGRATION=1 ECHOTRAILS_INTEGRATION_DATABASE_URL='postgres://echotrails:integration-only@127.0.0.1:55432/echotrails_integration?sslmode=disable' ECHOTRAILS_INTEGRATION_REDIS_URL='redis://127.0.0.1:56379/0' go test ./apps/api/internal/api ./apps/api/internal/placeimport ./apps/worker/internal/jobs ./apps/worker/internal/media ./apps/worker/internal/privacy ./apps/worker/internal/push ./apps/worker/internal/ranking -run Integration -count=1 -v

restore-drill: ## Backup and restore disposable Postgres/PostGIS data and verify object inventory
	@bash infra/integration/restore-drill.sh

moderation-outage-drill: ## Prove scanner outage leaves media private and pending review
	@bash infra/integration/release-service-drills.sh moderation

apns-mock-drill: ## Prove sandbox/production routing, timing, and invalid-token handling
	@bash infra/integration/release-service-drills.sh apns

release-service-drills: ## Run moderation outage and APNs mock release drills
	@bash infra/integration/release-service-drills.sh all

release-scale-test: ## Prove search p95 with 10k Places and 100k Echoes
	@ECHOTRAILS_INTEGRATION=1 ECHOTRAILS_SCALE_TEST=1 ECHOTRAILS_INTEGRATION_DATABASE_URL='postgres://echotrails:integration-only@127.0.0.1:55432/echotrails_integration?sslmode=disable' go test ./apps/api/internal/api -run SearchScaleP95 -count=1 -v -timeout 4m

release-processing-test: ## Prove photo/audio/video normalization latency in the Linux worker toolchain
	@docker build -f infra/integration/Dockerfile.processing-latency -t echotrails-processing-latency .
	@docker run --rm echotrails-processing-latency

privacy-release-check: ## Recursively inspect public JSON, logs, analytics, push, and export fixtures for private data
	@go test ./apps/api/internal/api -run PrivacyReleaseGate -count=1 -v
	@go test ./apps/worker/internal/push ./apps/worker/internal/privacy -run PrivacyReleaseGate -count=1 -v

moderation-check: ## Check conservative moderation policy without downloading models
	@python3 -m py_compile services/moderation/app.py services/moderation/policy.py services/moderation/test_policy.py services/moderation/calibrate.py
	@cd services/moderation && python3 -m unittest -q test_policy.py
	@cd services/moderation && python3 calibrate.py

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
	@test -f docs/runbooks/service-outage-drills.md
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

local-compose-check: ## Validate local development compose config when scaffolded
	@if [ -f "$(LOCAL_COMPOSE)" ]; then \
		if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then \
			docker compose -f "$(LOCAL_COMPOSE)" config >/dev/null; \
		elif command -v docker-compose >/dev/null 2>&1; then \
			docker-compose -f "$(LOCAL_COMPOSE)" config >/dev/null; \
		else \
			python3 infra/local/validate-compose.py "$(LOCAL_COMPOSE)"; \
		fi; \
	else \
		echo 'skip local compose check: infra/local/compose.yaml not present yet'; \
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
			(cd "$$app" && npm ci && npm run lint && npm run build && npm test -- --run); \
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
			cd "$(OPENAPI_DIR)" && npm run check; \
		else \
			echo 'OpenAPI contract present; skip npm lint because package.json is not present yet'; \
		fi; \
	else \
		echo 'skip OpenAPI checks: packages/openapi/ not present yet'; \
	fi

ios-boundary-check: ## Run Linux-safe iOS source boundary checks
	@if [ -f apps/ios/Scripts/check-navigation-boundaries.py ]; then \
		python3 apps/ios/Scripts/check-navigation-boundaries.py; \
	else \
		echo 'skip iOS boundary checks: script not present yet'; \
	fi

phase5-check: ## Run Phase 5 readiness artifact checks
	@if [ -f docs/runbooks/phase5-check.py ]; then python3 docs/runbooks/phase5-check.py; else echo 'skip Phase 5 readiness check: script not present yet'; fi

swift-check: ## Run Swift/Xcode checks when local tooling is available
	@if [ -d "$(IOS_PROJECT)" ] && [ "$$(uname -s)" = "Darwin" ]; then \
		xcodebuild build-for-testing -quiet -project "$(IOS_PROJECT)" -scheme "$(IOS_SCHEME)" -destination 'generic/platform=iOS' CODE_SIGNING_ALLOWED=NO && \
		swift test --package-path apps/ios; \
	elif [ -f apps/ios/Package.swift ] && command -v swift >/dev/null 2>&1; then \
		swift test --package-path apps/ios; \
	else \
		echo 'skip Swift checks: Swift toolchain or iOS project not available'; \
	fi

ios-ui-test: ## Run app unit and UI tests on the configured iOS destination
	@test -d "$(IOS_PROJECT)" || { echo 'iOS project is missing'; exit 1; }
	@test "$$(uname -s)" = "Darwin" || { echo 'iOS UI tests require macOS and Xcode'; exit 1; }
	@destination='$(IOS_DESTINATION)'; \
	if [ -z "$$destination" ]; then \
		simulator_id=$$(xcrun simctl list devices available | sed -nE 's/^[[:space:]]*iPhone.*\(([0-9A-Fa-f-]{36})\).*/\1/p' | head -n 1); \
		test -n "$$simulator_id" || { echo 'No available iPhone simulator found. Install an iOS simulator runtime in Xcode Settings > Components, or set IOS_DESTINATION explicitly.'; exit 1; }; \
		destination="platform=iOS Simulator,id=$$simulator_id"; \
	fi; \
	echo "Using iOS test destination: $$destination"; \
	xcodebuild test -project "$(IOS_PROJECT)" -scheme "$(IOS_SCHEME)" -destination "$$destination" CODE_SIGNING_ALLOWED=NO
