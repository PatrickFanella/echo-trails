# ADR-0001: Use a custom Go REST API

## Status

Accepted.

## Context

EchoTrails needs server-side enforcement for place creation rules, location proof validation, moderation state transitions, media upload finalization, audit logging, and account/privacy workflows. The PRD and technical plan already identify a custom API as the target beta backend.

Managed backend products can accelerate prototypes, but EchoTrails has domain-specific safety and privacy requirements that should remain under direct application control. The existing generated iOS app is reference-only and is not a backend source of truth.

## Decision

Build EchoTrails around a custom Go REST/JSON API backed by Postgres/PostGIS and Redis-backed workers.

The API will expose the mobile, public web, and admin surfaces through documented HTTP resources. It will remain the source of truth for authorization, Place state, Echo state, moderation decisions, idempotency, and public/private location separation.

## Consequences

- The team owns API design, persistence boundaries, authorization, and operational behavior instead of delegating them to a managed backend.
- Go services keep the beta stack small, explicit, and deployable in containers.
- More scaffolding is required before feature delivery compared with a managed backend.
- API behavior must be captured in the OpenAPI contract and verified by local/CI checks as implementation lands.
