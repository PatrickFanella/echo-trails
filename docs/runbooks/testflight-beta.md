# TestFlight Beta Checklist

## Status

Template only. No TestFlight upload or Apple signing action has been executed from this repository.

## External prerequisites

- Apple Developer account access.
- Bundle ID and signing profiles.
- App Store Connect app record.
- macOS/Xcode runner or developer machine.
- Privacy nutrition label draft reviewed by product/legal.

## Build prerequisites

- `make verify` passes on `main`.
- Swift package checks pass where Swift is installed.
- Known contract-only iOS surfaces are accepted as pre-runtime placeholders.
- No production secrets committed.

## Smoke matrix

- Launch app shell.
- Logged-out/auth-required state.
- Invite required and city unavailable states.
- Place discovery read-only state.
- Media upload status state.
- Social action descriptors for reactions, replies, stitches, reports, blocks, hides.
- Policy links visible in public web.
- Admin shell role-denied state.

## Rollback / kill switch

- Remove external tester group access.
- Disable invite city gate.
- Disable media writes and social mutations at API gateway if abuse appears.
- Revert to previous TestFlight build in App Store Connect.

## Known not implemented

- Real SwiftUI runtime screens from contract descriptors.
- Real Keychain/AuthServices implementation.
- Real MapKit/SwiftData/media picker integration.
- Real APNs delivery.
- Live production monitoring/backup/restore execution.
