# Open Pilot Readiness

Open Pilot can safely work in this repository when issues are specific, scoped, and verifiable.

## Required labels

Open Pilot only runs after both labels are present:

- `ready-for-agent`
- `agent:queued`

The issue template adds `ready-for-agent`. A human should add `agent:queued` only after reviewing the issue body.

## Issue requirements

Every agent-ready issue should include:

- Goal
- Context
- Acceptance Criteria
- Test Command
- Out of Scope
- Queue Checklist

## EchoTrails-specific context

Agent-ready issues should reference the relevant docs:

- Glossary: `CONTEXT.md`
- Product requirements: `docs/PRD.md`
- Technical plan: `docs/technical-plan.md`

Generated iOS files are reference-only unless an issue explicitly says to modify them.

## Test command guidance

Prefer the narrowest deterministic command that proves the requested change.
The repository baseline is `make verify`; the `.gitea` workflow runs the same command for push and pull request checks. The baseline is Linux-safe and skips unscaffolded Go, TypeScript, OpenAPI, and non-macOS Swift checks with explicit messages.

Examples:

```bash
make verify
cd apps/api && go test ./...
xcodebuild test -project apps/ios/EchoTrails.xcodeproj -scheme EchoTrails -destination 'platform=iOS Simulator,name=iPhone 16'
```

## PR review and merge

Open Pilot PR review expects the PR verification command to pass. If the deployed Open Pilot instance publishes commit statuses, merge gates should require the Open Pilot-owned status context before merging.
