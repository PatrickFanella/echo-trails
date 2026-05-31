# EchoTrails Admin Web

React/Vite admin web app for EchoTrails moderation and operations workflows.

Current Phase 4 scope is a non-mutating fixture workflow:

- authenticated shell shape with deterministic role guards
- moderation queue/detail/action-intent placeholders
- Places review list with boundary-sensitive override hints
- audit labels for future server-side moderation writes

It does not implement real auth, API writes, DB persistence, sanctions, appeals, or production moderation decisions yet.

## Install and run

Use either npm or pnpm from this app directory:

```bash
cd apps/admin-web
npm install
npm run dev
```

```bash
cd apps/admin-web
pnpm install
pnpm dev
```

Until dependencies are installed, `npm run lint`, `npm run build`, and `npm test -- --run` intentionally print a skip message so repo-level `make verify` can pass on scaffold-only checkouts.

## Smoke path

After installing dependencies, run:

```bash
npm run lint
npm run build
npm test -- --run
```

For a manual UI smoke check, start `npm run dev` and confirm the page renders “EchoTrails Admin”, role-gated navigation, “Moderation queue”, and “Places review”.
