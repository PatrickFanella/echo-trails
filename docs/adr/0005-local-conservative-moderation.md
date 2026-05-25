# ADR-0005: Use local conservative moderation first

## Status

Accepted for beta.

## Context

EchoTrails is a moderated public app for users age 13+. The PRD requires automated checks before publication, local/open-source conservative moderation because moderation must be free, fail-closed media behavior, a separate moderation host if possible, reports, appeals, admin audit logs, and safe handling for sensitive Places and content.

Paid moderation APIs may improve accuracy and throughput, but they conflict with the current free/local constraint and may increase data sharing for private or pending media.

## Decision

Use local/open-source conservative moderation as the initial automated moderation path. Media remains pending if scans are unavailable or uncertain. Text may publish only when the text scanner path is available and policy checks pass. Moderator/admin review, audit logs, appeals, and state transitions remain controlled by the custom API and admin tooling.

The moderation approach must be revisited if local tools cannot meet beta safety, latency, accuracy, or operational needs.

## Consequences

- Pending/private media can be evaluated without sending it to third-party moderation APIs by default.
- Conservative scanning reduces unsafe publication risk but may increase false positives and manual review load.
- Moderation host availability becomes a release and operations concern; fail-closed behavior is required for media.
- The system needs clear user-facing status for pending, limited, removed, failed, and appeal states.
