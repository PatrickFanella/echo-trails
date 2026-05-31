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
- best-effort audio/video transcript records
- conservative local moderation review records

Transcription currently uses an injectable `Transcriber` interface and deterministic
fixture transcriber. Image assets are skipped; transcriber failures are stored as
failed transcript records instead of crashing the worker.

Moderation currently uses an injectable `SafetyScanner` interface and deterministic
fixture scanner. Only explicit low risk is approved; unknown/medium/offline cases
go to review, and high risk is rejected.

It does not run FFmpeg, ImageMagick, Whisper, real ML moderation, or real
object-storage reads yet. Those integrations remain later isolated adapters.
