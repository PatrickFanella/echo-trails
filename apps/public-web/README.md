# EchoTrails Public Web

Target location for the minimal React/Vite public web app.

This app will provide share previews, legal pages, and public calls to action without becoming the primary EchoTrails client.

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

For a manual UI smoke check, start `npm run dev` and confirm the page renders the “EchoTrails place-stories” placeholder.
