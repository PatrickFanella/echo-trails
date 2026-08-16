# EchoTrails iOS app

This is the authoritative iOS 17+ SwiftUI application. The repository-level
`ios/` directory remains untouched prototype reference material.

The app target includes:

- passwordless email and Sign in with Apple with Keychain session storage
- invite and consent onboarding
- SwiftData Place/Echo/profile caches plus a durable outbox/upload ledger
- Map, Explore, Activity, Profile, Place detail, and Echo detail surfaces
- GPS creation proof without persisting or logging exact coordinates
- text, 1–5 photo, two-minute AAC audio, and one-minute video composition
- photo/video picker and camera capture
- a single file-backed background `URLSession` for uploads and relaunch recovery
- reactions, nested Replies, full Stitches, immutable Reply snapshot warnings,
  reports/hides, subscriptions, notification preferences, export, and deletion
- APNs registration, safe Activity refresh, and deep-link intake
- a server-authoritative release-access bootstrap and an upload-status dashboard
  covering background transfer, outbox retry, processing, and moderation state
- Dynamic Type-native SwiftUI controls, VoiceOver labels, offline/outbox states,
  reduced-motion system behavior, data saver, and Wi-Fi autoplay preferences

The pure `EchoTrailsShell` Swift package remains as the generated-contract and
Linux-friendly domain layer used by the app.

Generate the Xcode project after changing `project.yml`:

```bash
cd apps/ios
xcodegen generate --spec project.yml
```

Compile the app and tests without signing:

```bash
xcodebuild build-for-testing -project apps/ios/EchoTrails.xcodeproj \
  -scheme EchoTrails -destination 'generic/platform=iOS' CODE_SIGNING_ALLOWED=NO
```

`make verify` runs that compile plus the portable Swift package tests. Running
the iOS tests requires an installed compatible simulator runtime or a signed
physical test device.

## Personal-Team field build

The `FieldTest` configuration targets `https://api.echotrails.subcult.tv` with
bundle ID `tv.subcult.EchoTrails.fieldtest`. It deliberately omits APNs,
Associated Domains, and Sign in with Apple so a Personal Team can install it
for the first field walk. Email authentication, GPS proof, media capture,
background upload, and server-backed Activity remain enabled. Release keeps the
complete production entitlement and configuration checks.
