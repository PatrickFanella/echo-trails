# EchoTrails OpenAPI contract

`openapi.yaml` is the baseline machine-readable API contract for Go, Swift, and TypeScript integration work and the target location for generated client checks.

The file is JSON-compatible YAML and is validated with a dependency-free lint script. Contract path constants are generated for TypeScript and Swift drift checks:

```bash
cd packages/openapi
npm run check
```

Generated files:

- `packages/shared-contracts/src/openapi-paths.ts`
- `apps/ios/Sources/EchoTrailsShell/Generated/OpenAPIPaths.swift`

The repository baseline `make verify` runs the same check whenever `packages/openapi/package.json` is present.
