# Architecture Decision Records

Architecture Decision Records capture durable implementation choices for EchoTrails. They explain why a direction was selected, but they do not replace the canonical product and technical planning docs.

Canonical sources:

- Product requirements: `../PRD.md`
- Technical architecture and phased plan: `../technical-plan.md`
- Domain glossary: `../../CONTEXT.md`

## Decisions

- [ADR-0001: Use a custom Go REST API](0001-custom-go-rest-api.md)
- [ADR-0002: Use a monorepo with an OpenAPI contract](0002-monorepo-openapi-contract.md)
- [ADR-0003: Keep Places provider-agnostic](0003-provider-agnostic-places.md)
- [ADR-0004: Self-host the beta infrastructure](0004-self-host-beta-infrastructure.md)
- [ADR-0005: Use local conservative moderation first](0005-local-conservative-moderation.md)
- [ADR-0006: Use Redis for asynchronous jobs](0006-redis-queue.md)
