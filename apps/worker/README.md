# EchoTrails Worker

Background worker service for asynchronous EchoTrails jobs.

Planned responsibilities include media processing, notifications, moderation queue work, analytics rollups, and other Redis-backed jobs described in `docs/technical-plan.md`.

Commands:

```bash
go test ./...
go run ./cmd/worker
```

Current skeleton job types:

- `process_media`
- `generate_transcript`
- `scan_media_safety`

The worker queue framework currently provides:

- enqueue/reserve/succeed/fail contracts
- idempotent enqueue by job key
- retry backoff and dead-letter state
- explicit Redis connection configuration validation

The executable still uses the deterministic in-memory queue. Real Redis stream
reservation is later work.

Current media processing skeleton provides deterministic interfaces and tests
for:

- metadata-safe processing results
- image thumbnails/previews
- audio waveforms
- video thumbnail/preview/waveform plans
- uploaded/processing → pending-review status transition

It does not run FFmpeg, ImageMagick, Whisper, or real object-storage reads yet.
Those integrations remain later isolated adapters.
