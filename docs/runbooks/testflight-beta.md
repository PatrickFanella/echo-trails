# TestFlight Beta Checklist

## Status

The repository-local engineering gates passed on 2026-07-09: `make verify`,
`make integration-test`, `make release-scale-test`,
`make release-processing-test`, `make restore-drill`, and
`make release-service-drills`. The generic-device `apps/ios` application also
builds for testing with code signing disabled. No TestFlight upload or Apple
signing action has been executed from this repository; those remain external
release operations.

## External prerequisites

- Apple Developer account access.
- Bundle ID and signing profiles.
- App Store Connect app record.
- macOS/Xcode runner or developer machine.
- Privacy nutrition label draft reviewed by product/legal.

## Build prerequisites

- `make verify` passes on `main`.
- Swift package checks pass where Swift is installed.
- The generic-device iOS application build and Swift package tests pass.
- `make ios-ui-test IOS_DESTINATION='platform=iOS Simulator,name=<available device>'`
  passes on an up-to-date simulator before the physical-device pass. The
  `EchoTrailsUITests` target is also included in the shared scheme so the same
  smoke suite runs on a connected beta device.
- Supply `ECHOTRAILS_API_BASE_URL` as the deployed HTTPS API origin and
  `ECHOTRAILS_ASSOCIATED_DOMAIN` as the deployed universal-link host. Release
  builds reject missing, non-HTTPS, and reserved `.example` values; only Debug
  builds default to the local API.
- `make integration-test`, `make restore-drill`, `make release-scale-test`, and
  `make release-processing-test`
  pass against the release candidate environment.
- The loaded moderation model calibration passes against the pinned model
  revisions used by the moderation host.
- No production secrets committed.

## Smoke matrix

- Apple and email code/universal-link login, token refresh, logout, and per-device revocation.
- Invite, consent, profile setup/edit, export, and deletion flows.
- Downers Grove map/search/ranked discovery and active/restricted/outside-boundary creation cases.
- Text, five-photo, two-minute audio, and one-minute video creation, background retry, processing, moderation, and accessible playback/transcripts.
- Place subscribe/unsubscribe, notification toggle, Place/thread mute, Activity pagination, APNs receipt, and deep links.
- Reactions, nested Reply create/edit/delete/history, Stitches, immutable Reply snapshots, reports, hides, blocks, appeals, and continuity placeholders.
- Moderator/admin role denial, report/media/Place/job/appeal queues, signed previews, confirmations, reasons, CSRF protection, and audit history.
- VoiceOver, Dynamic Type, contrast, reduced motion, offline, data saver, and Wi-Fi-only autoplay on multiple physical devices.

## Rollback / kill switch

- Remove external tester group access.
- Disable invite city gate.
- Disable media writes and social mutations at API gateway if abuse appears.
- Revert to previous TestFlight build in App Store Connect.

## Required service drills

- Run `make release-service-drills` and attach both JSON evidence files.
- Repeat the moderation-host outage against staging and confirm media stays
  private and pending review.
- Complete Apple-backed sandbox and production APNs device delivery tests;
  local HTTP mocks verify routing and failure semantics but do not replace Apple.

## Remaining engineering validation

The remaining gates require release credentials, deployed infrastructure, model
weights, or physical Apple devices; they cannot be satisfied by repository
fixtures alone:

- Run the multi-device physical-device smoke matrix and record VoiceOver,
  Dynamic Type, contrast, reduced-motion, background-transfer, and playback
  results with the signed release build.
- Record real Apple sandbox and production APNs delivery evidence.
- Repeat the moderation-host outage drill in staging and run calibration with
  the pinned, loaded model revisions on the moderation host.
- Run backup/restore against the beta environment and verify API `/ready` on
  the isolated restore.
- Attach the generated local privacy, scale, processing-latency, restore, and
  APNs/mock evidence plus the physical accessibility results to the release
  record.
