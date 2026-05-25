SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

.PHONY: help verify fmt lint test docs-check open-pilot-check secrets-check xcode-note

help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"; printf "Commands:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

verify: fmt lint test docs-check open-pilot-check secrets-check xcode-note ## Run all checks

fmt: ## Format files where repo tooling is available
	@if [ -d apps/api ]; then cd apps/api && gofmt -w $$(find . -name '*.go' -type f); else echo 'skip gofmt: apps/api/ not present yet'; fi
	@if [ -d apps/worker ]; then cd apps/worker && gofmt -w $$(find . -name '*.go' -type f); else echo 'skip gofmt: apps/worker/ not present yet'; fi

lint: ## Run lightweight lint checks
	@if [ -d apps/api ]; then cd apps/api && go vet ./...; else echo 'skip go vet: apps/api/ not present yet'; fi
	@if [ -d apps/worker ]; then cd apps/worker && go vet ./...; else echo 'skip go vet: apps/worker/ not present yet'; fi
	@if [ -d apps/admin-web ]; then cd apps/admin-web && npm run lint; else echo 'skip admin-web lint: apps/admin-web/ not present yet'; fi
	@if [ -d apps/public-web ]; then cd apps/public-web && npm run lint; else echo 'skip public-web lint: apps/public-web/ not present yet'; fi

test: ## Run deterministic tests available on this host
	@if [ -d apps/api ]; then cd apps/api && go test ./...; else echo 'skip go test: apps/api/ not present yet'; fi
	@if [ -d apps/worker ]; then cd apps/worker && go test ./...; else echo 'skip go test: apps/worker/ not present yet'; fi
	@if [ -d apps/admin-web ]; then cd apps/admin-web && npm test -- --run; else echo 'skip admin-web tests: apps/admin-web/ not present yet'; fi
	@if [ -d apps/public-web ]; then cd apps/public-web && npm test -- --run; else echo 'skip public-web tests: apps/public-web/ not present yet'; fi

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

secrets-check: ## Check for common committed secret filenames
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		! git ls-files | grep -E '(^|/)(\.env|.*\.pem|.*\.key|.*\.p12|.*\.pfx|id_rsa|id_ed25519)$$'; \
	else \
		echo 'skip secrets-check: not a git repository'; \
	fi

xcode-note: ## Explain iOS verification status on non-macOS runners
	@echo 'iOS/Xcode tests are not part of Linux make verify yet; add xcodebuild checks when macOS runner is available.'
