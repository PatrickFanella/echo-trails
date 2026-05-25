# ADR-0004: Self-host the beta infrastructure

## Status

Accepted for beta.

## Context

The PRD and technical plan target an invite-gated beta with public API hosting on the `almaz` homelab public edge host, containers, Postgres/PostGIS, Redis, S3-compatible media storage, a separate moderation host, and explicit disclosure of hosting location. The plan also calls for backups, monitoring, and restore drills before beta testers.

Self-hosting reduces recurring platform costs and keeps operational control close during a small beta, but it adds availability, security, privacy disclosure, and abuse-response risk.

## Decision

Self-host the beta infrastructure on the documented homelab deployment path, without Kubernetes. Use containerized services with explicit resource limits, separate local/staging/prod-beta environments, and documented backup/restore and monitoring before inviting beta users.

Do not treat this as a permanent production hosting decision. Revisit hosting if scale, compliance, reliability, moderation, or abuse requirements exceed the beta operating model.

## Consequences

- Beta costs and operational surfaces stay small and inspectable.
- Privacy disclosures must clearly describe the hosting location and controls, especially before EU invites.
- Reliability, incident response, backups, patching, and monitoring are first-party responsibilities.
- Future migration to managed infrastructure remains possible but will require a deliberate ADR update or replacement.
