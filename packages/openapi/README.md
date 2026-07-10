# EchoTrails OpenAPI contract

`openapi.yaml` is the baseline machine-readable API contract for Go, Swift, and TypeScript integration work and the target location for generated client checks.

Every route in the release contract is wired in the runnable API binary. The
`x-echotrails-runtime-status` extension is checked with an empty
`contractOnlyUntilLaterPass` list so contract-only release paths cannot be
mistaken for implemented behavior.

The file is JSON-compatible YAML and is validated with a dependency-free lint
script. Contract paths and operation-specific, path-parameter-encoding builders
are generated for TypeScript and Swift. Runtime clients are checked so they
cannot reintroduce handwritten API route strings:

```bash
cd packages/openapi
npm run check
```

Generated files:

- `packages/shared-contracts/src/openapi-paths.ts`
- `apps/admin-web/src/generated/openapi-paths.ts`
- `apps/ios/Sources/EchoTrailsShell/Generated/OpenAPIPaths.swift`

The repository baseline `make verify` runs the same check whenever `packages/openapi/package.json` is present.
