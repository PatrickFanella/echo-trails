# ADR-0002: Use a monorepo with an OpenAPI contract

## Status

Accepted.

## Context

EchoTrails will include an iOS app, Go API, Go worker, admin web app, public preview web app, shared API contracts, and local/deploy infrastructure. These components are tightly coupled during the early rebuild and should evolve from the PRD and technical plan together.

The iOS and TypeScript clients need stable request/response shapes, while the API must remain independently testable. The repository is currently in scaffold phase, so decisions should optimize for small, verifiable changes.

## Decision

Use a single monorepo for application code, packages, infrastructure, and documentation. Keep the API contract in `packages/openapi` and use OpenAPI as the canonical machine-readable interface between the Go API, Swift client, and TypeScript clients.

The PRD and `docs/technical-plan.md` remain canonical for product scope and system architecture. ADRs record why durable decisions were made; they do not introduce new product requirements.

## Consequences

- Cross-component changes can be reviewed and verified in one PR when the scope is intentionally small.
- OpenAPI generation and diff checks become part of the baseline quality gates once contract tooling is scaffolded.
- Client/server drift should be caught earlier through generated or checked clients.
- The repository must keep clear package boundaries so the monorepo does not become an implicit shared-state shortcut.
