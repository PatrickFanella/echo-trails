# Contributing

This repository uses SUBCULT Open Pilot-ready conventions.

## Development flow

1. Create or pick an issue before making non-trivial changes.
2. Create a branch from `main`.
3. Make the smallest change that satisfies the issue.
4. Run verification:

   ```bash
   make verify
   ```

5. Open a pull request using the PR template.
6. Link the issue with `Closes #N` when appropriate.

## Commit style

Prefer concise conventional-style subjects:

```text
feat: add place search endpoint
docs: update API plan
fix: prevent exact coordinate exposure
chore: refresh repo metadata
```

## Open Pilot issues

Use `.gitea/ISSUE_TEMPLATE/open-pilot-task.yaml` for tasks that Open Pilot should implement.

Open Pilot requires:

- `ready-for-agent`
- `agent:queued`
- a deterministic `Test Command`

The template adds `ready-for-agent`; add `agent:queued` only after the issue is complete and ready for automation.

Good Open Pilot tasks are:

- one focused outcome
- clear context and constraints
- explicit out-of-scope notes
- verifiable from the repository root

## Verification commands

Use the narrowest deterministic command that proves the change.

Default full check:

```bash
make verify
```

Examples as the app grows:

```bash
make verify
cd apps/api && go test ./...
xcodebuild test -project apps/ios/EchoTrails.xcodeproj -scheme EchoTrails -destination 'platform=iOS Simulator,name=iPhone 16'
```

## Boundaries

- Do not commit secrets, signing keys, local databases, generated caches, or production credentials.
- Do not add production deploy actions without an explicit issue and human approval.
- Do not add `agent:queued` to vague, broad, or secret-dependent issues.
- Do not use generated iOS prototype files as implementation source of truth unless the issue explicitly asks for it.
