# ADR-0003: Keep Places provider-agnostic

## Status

Accepted.

## Context

Places are central to EchoTrails. The PRD defines Places as named real-world points, venues, landmarks, addresses, or areas with coordinates and human-readable names. It also requires provider-agnostic internal Place IDs, external provider mappings, pending user-requested Places, duplicate merges, aliases, boundaries, and sensitive Place controls.

Depending directly on one external provider's identifiers or categories would simplify the first search flow but would make merges, provider replacement, sensitive-place handling, and auditability harder later.

## Decision

Use EchoTrails-owned Place records and stable internal Place IDs as the source of truth. Store external provider identifiers in mapping records rather than exposing provider IDs as primary product identifiers.

Place records may aggregate provider metadata, admin edits, aliases, merge redirects, boundaries/radii, and moderation/sensitivity state. Provider data can inform Place creation and updates, but it does not own the product identity of a Place.

## Consequences

- Echoes, subscriptions, reports, and moderation history can survive provider changes, duplicate merges, and external ID churn.
- The product can mix data from multiple providers or admin/user submissions over time.
- Initial Place search and reconciliation work is more complex than passing through a single provider result.
- Admin tooling and audit logs must support Place merges, aliases, provider mappings, and boundary edits.
