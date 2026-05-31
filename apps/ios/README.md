# EchoTrails iOS App

Target location for the clean Swift/SwiftUI iOS app rebuild.

The generated prototype currently under the repository-level `ios/` directory is reference-only. New iOS implementation work should be scoped here unless a later issue explicitly says otherwise.

Current scope is a Swift Package shell only:

- stable app identity constants
- generated OpenAPI path constants
- Phase 3 navigation tab names as data
- environment selection for local/staging/production API base URLs
- pure Swift API client configuration seams
- bootstrap state for the eventual SwiftUI app target

No auth, Keychain storage, SwiftData, runtime networking, MapKit UI, media composer, or upload UI belongs in this baseline issue.

Verification:

```bash
make verify
```

Narrow iOS package check when Swift is installed:

```bash
swift test --package-path apps/ios
```

macOS/Xcode project generation remains a later machine-specific step; this package keeps the baseline deterministic in CI.
