# Moderation and APNs Release Drills

## Purpose

These drills provide repeatable local evidence for the two external-service
failure modes that block TestFlight release. Generated evidence is written to
`.artifacts/release-drills/` and is intentionally excluded from source control.

## Moderation-host outage

Run `make moderation-outage-drill`.

The drill removes a real local HTTP endpoint before scanning private media and
proves that the worker persists `pending_review` with unknown risk and
`scanner-unavailable` evidence. It also proves the asset is never marked
approved or public. Before release, repeat the outage against staging by
stopping or firewalling the moderation host and confirm queued media remains
private until service recovery.

## APNs sandbox and production mock

Run `make apns-mock-drill`.

The drill configures separate sandbox and production HTTP mocks and dispatches
through the real provider and dispatcher. It verifies environment routing,
token-authenticated safe payloads, collapse IDs, dispatch start in under one
second (well inside the 60-second release target), successful delivery state,
and disabling a production device after APNs returns `410 Unregistered`.

The local mock does not replace the required Apple sandbox and production
device test. Record those external APNs IDs and device results in the release
record before inviting testers.

## Combined command

Run `make release-service-drills` to execute both drills. Each successful drill
records the commit, operator, UTC timestamp, command, duration, assertions, and
result in its own JSON evidence file.
