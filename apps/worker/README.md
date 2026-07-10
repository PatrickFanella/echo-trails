# EchoTrails Worker

Background worker service for asynchronous EchoTrails jobs.

The worker executes media processing, moderation/transcription, immutable Reply
snapshots, ranking/Place alerts, APNs delivery, and privacy export/deletion jobs.

Commands:

```bash
go test ./...
go run ./cmd/worker
```

Job types include:

- `process_media`
- `generate_transcript`
- `scan_media_safety`

The worker queue framework currently provides:

- enqueue/reserve/succeed/fail contracts
- idempotent enqueue by job key
- retry backoff and dead-letter state
- explicit Redis connection configuration validation

The executable requires Redis Streams and Postgres by default. It creates a
consumer group, reclaims stale reservations, promotes exponentially delayed
retries, deduplicates completed work, and moves exhausted jobs to a dead-letter
stream. The executable has no in-memory startup mode; unit tests instantiate
test queues directly.

The production media pipeline provides:

- metadata-safe processing results
- private S3 original reads, byte-count/signature checks, and SHA-256 recording
- mandatory ClamAV scanning before processing
- EXIF/GPS/metadata stripping and image thumbnails/previews
- normalized AAC audio plus JSON waveforms
- normalized 1080p H.264/AAC MP4, thumbnail, and waveform derivatives
- uploaded/processing → pending-review status transition
- best-effort audio/video transcript records
- conservative local moderation review records

Production transcription uses the private moderation service's `faster-whisper`
adapter. Image assets are skipped; no-speech media completes with an accessible
fallback transcript, while service/processing failures remain private and move
the Echo to `pending_review` for fail-closed recovery.

Production moderation uses authenticated ShieldGemma, ShieldGemma 2, and
OpenNSFW2 adapters on the separate private model host. Only explicit low risk is
approved; unknown/medium/offline cases go to review, and high risk is rejected.

The Postgres catalog is authoritative for asset, derivative, transcript, and
moderation status. Production startup requires object storage, FFmpeg,
ImageMagick, ClamAV, and the authenticated private model-service configuration;
fixture adapters exist only in test code and the disposable integration stack.

Privacy jobs build sectioned exports without coordinates, credentials, raw push
tokens, or scanner evidence. Export objects expire after seven days. Account
deletion removes owned originals/derivatives, privacy exports, and affected
Reply quote snapshots before clearing object pointers and de-identifying the
authoritative rows.
