# Phase 3 iOS Contracts Implementation Plan

> **For agentic workers:** Execute this plan task-by-task. Recommended path:
> dispatch a fresh subagent per task, review each result with `review-quality`,
> then continue. For complex multi-agent splits, use
> `parallel-feature-development`, `team-composition-patterns`, and
> `team-communication-protocols`. Steps use checkbox (`- [ ]`) syntax for
> tracking.

**Goal:** Close #37-#43 with Linux-verifiable iOS contract models, not runtime UI.

**Architecture:** Add pure Foundation Swift data contracts under `apps/ios/Sources/EchoTrailsShell` plus XCTest specs for hosts with Swift. Linux truth comes from `make verify` and `apps/ios/Scripts/check-navigation-boundaries.py`, which forbids Apple UI/runtime framework imports and checks required contract files/snippets.

**Tech Stack:** Swift Package, Foundation, XCTest, Python boundary checker, Makefile verify.

---

## Boundaries

- No SwiftUI, MapKit, SwiftData, AuthenticationServices, Security, CoreLocation, Photos, AVFoundation, UIKit.
- No runtime networking, media picker, location permission, Keychain, or generated prototype app edits.
- Implement #37-#43 as contract/data descriptors only.

## Tasks

- [ ] #37 Map contracts: coordinates, viewport, Place markers/detail descriptors, public precision, API path references.
- [ ] #38 Explore contracts: query/scope/section/ranking/empty states.
- [ ] #39 Create flow contracts: draft, content mode, validation, proof preflight, create steps.
- [ ] #40 Cache/outbox contracts: SwiftData-ready descriptors, outbox states, retry/conflict policy, schema version.
- [ ] #41 Media composer contracts: attachment, limits, validation, upload intent/status descriptors.
- [ ] #42 Profile/Activity contracts: public/private profile split, activity rows, privacy-safe preview policy.
- [ ] #43 Accessibility contracts: audit rules/results and surface descriptors for Phase 3 contracts.
- [ ] Verification: `rtk make fmt`, `rtk make verify`, `rtk git diff --check`; run `swift test --package-path apps/ios` only when Swift is installed.
