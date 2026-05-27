# ADR-0006: Use Redis for asynchronous jobs

## Status

Accepted.

## Context

EchoTrails requires asynchronous work for media processing, moderation scans, transcripts/captions, thumbnails/waveforms, notifications, analytics rollups, and operational alerts. The technical plan already includes a separate worker process/container and Redis in the beta service list.

Postgres will already exist and can support simple job tables, but long-running media and moderation workflows benefit from an explicit queue boundary between API requests and worker execution.

## Decision

Use Redis as the initial asynchronous job queue for beta, with a separate Go worker process/container consuming jobs from the same codebase. Keep durable domain state in Postgres; use Redis for queueing, retries, scheduling/backoff where appropriate, and transient worker coordination.

Document any acceptable queue-loss behavior before beta. If a job outcome affects user-visible or moderation state, store the authoritative state transition in Postgres.

## Consequences

- API requests can return quickly while media, moderation, notification, and processing tasks run out of band.
- Redis introduces another operational dependency that needs monitoring, persistence decisions, and failure-mode documentation.
- Workers must be idempotent because jobs may retry or be delivered after partial progress.
- Postgres remains the source of truth for Echo, media, moderation, notification, and audit state.
