# EchoTrails OpenAPI contract

`openapi.yaml` is the baseline machine-readable API contract for Go, Swift, and TypeScript integration work and the target location for generated client checks.

During scaffold phase the file is JSON-compatible YAML and is validated with a dependency-free lint script:

```bash
cd packages/openapi
npm run lint
```

The repository baseline `make verify` runs the same lint command whenever `packages/openapi/package.json` is present.
