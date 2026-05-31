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

This is an in-memory contract only. Redis-backed reservation, media transforms,
transcription engines, and moderation scanners are later work.
