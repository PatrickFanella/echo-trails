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
- pure Swift navigation route/state model
- deep-link parsing contracts for Places, Echoes, profiles, and upload status
- screen descriptors with titles, accessibility label seeds, required API paths, and auth metadata
- Linux-safe boundary checks that forbid SwiftUI/MapKit/SwiftData/AuthServices/Security imports until Xcode-scoped issues
- auth/session contract descriptors for email code, Apple Sign-In exchange, token redaction, and Keychain-ready storage seams
- Phase 3 contract descriptors for Map, Explore, create flow, SwiftData-ready cache/outbox schema, media composer, Profile/Activity, and accessibility audit surfaces
- Phase 4 social interaction descriptors for reactions, replies, stitches, reports, blocks, hides, and authenticated action metadata

No runtime auth UI, Keychain storage implementation, SwiftData, runtime networking, MapKit UI, media composer, or upload UI belongs in this baseline issue.

Phase 3 contracts are intentionally data-only. They describe eventual UI state,
validation, accessibility, and API paths without importing Apple UI/runtime
frameworks or touching the generated prototype under repository-level `ios/`.
Auth contracts are data-only: they reference email/Apple/session endpoints,
redact token previews, and describe Keychain-ready storage without importing
AuthenticationServices or Security.
Social contracts are data-only: they reference Phase 4 API paths and validation
rules for eventual UI without rendering SwiftUI/UIKit views or mutating network
state.

Verification:

```bash
make verify
```

Narrow iOS package check when Swift is installed:

```bash
swift test --package-path apps/ios
```

macOS/Xcode project generation remains a later machine-specific step; this package keeps the baseline deterministic in CI.
