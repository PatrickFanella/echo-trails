# EchoTrails Public Web

Target location for the minimal React/Vite public web app.

This app provides Linux-verifiable beta public fixtures:

- share preview content
- TestFlight / invite call to action
- terms, privacy, and hosting/safety disclosure page metadata
- explicit notice that full public browsing is not available during beta

Policy text is template-only and requires legal review before public launch.

## Install and run

Use either npm or pnpm from this app directory:

```bash
cd apps/public-web
npm install
npm run dev
```

```bash
cd apps/public-web
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

For a manual UI smoke check, start `npm run dev` and confirm the page renders “EchoTrails place-stories”, “Request TestFlight invite”, share preview copy, and the three policy cards.
