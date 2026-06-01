# EchoTrails Pre-Beta Threat Model

## Status

Pre-beta checklist artifact. Not a certification or external security review.

## Assets

- User accounts and session tokens.
- Location proof and private coordinates.
- Public Place/Echo content.
- Media uploads and moderation outcomes.
- Admin actions, reports, appeals, and audit trail.
- Backups and restore artifacts.

## Actors

- Anonymous public visitors.
- Beta users.
- Reported/blocked users.
- Moderators and admins.
- Service operators.
- Abusive scrapers or spam clients.

## Trust boundaries

- Public web to API.
- iOS client to API.
- API to database/object storage/Redis.
- Worker to media storage and moderation processors.
- Admin web to moderation APIs.
- Backup/restore operator to production data.

## Key risks and mitigations

- Exact private coordinates leak: never log exact private coordinates; public coordinates only.
- Token leakage: redact tokens in iOS contracts, API logs, and runbooks.
- Report retaliation: reports are anonymous to reported users; previews stay generic.
- Block bypass: blocked users cannot interact with blocker-owned Echoes in the API spine.
- Media abuse: processing and moderation remain conservative; unsafe/unknown requires review.
- Admin misuse: admin fixture requires role-gated shell and future audit labels.
- Backup exposure: restore drills use isolated resources; secrets stay outside git.
- Legal/policy mismatch: public policy pages are templates and require legal review.

## Pre-beta checks

- `make verify` passes.
- Policy pages exist and state legal review requirement.
- Privacy export/delete contracts exist.
- Analytics opt-out exists and rejects exact coordinates.
- Deployment, backup, and monitoring runbooks exist.
- TestFlight checklist documents external Apple/signing dependencies.
