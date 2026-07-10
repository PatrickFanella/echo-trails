# EchoTrails Admin Web

React/Vite admin web app for EchoTrails moderation and operations workflows.

The release console provides:

- secure HTTP-only admin sessions, CSRF protection, and moderator/admin role guards
- report, media, Place, job, appeal, invite, and immutable audit workflows
- signed private media previews
- confirmation, reason-code, idempotency, and server-side audit enforcement
- Place approval, restriction, merge, Polygon/MultiPolygon boundary, and point-radius tools

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

Use `npm ci` for the locked release dependency set.

## Smoke path

After installing dependencies, run:

```bash
npm run lint
npm run build
npm test -- --run
```

For a manual UI smoke check, sign in with a moderator/admin bearer session and
verify queue pagination, signed preview redirects, audited mutations, Place
boundary tools, and invite creation.
