# Agent Instructions

This repository follows the SUBCULT Open Pilot-ready template conventions, adapted for EchoTrails.

## Project

EchoTrails is a public place-stories product: people leave short Echoes tied to real-world Places so others can discover the stories of those Places.

## Current status

The existing generated iOS app is prototype reference only. Development should rebuild cleanly from the docs and domain glossary.

## Priorities

1. Keep issues small, scoped, and independently verifiable.
2. Preserve Open Pilot issue and PR templates.
3. Keep generated caches, secrets, local data, signing material, and production credentials out of git.
4. Update docs when product, architecture, domain language, or verification conventions change.
5. Respect `CONTEXT.md` as glossary only; keep implementation decisions in docs/technical plans or ADRs.
6. Do not treat current generated iOS files as source of truth unless an issue explicitly says so.

## Key docs

- `CONTEXT.md`
- `docs/PRD.md`
- `docs/technical-plan.md`
- `docs/open-pilot.md`
- `docs/labels.md`

## Verification

Run:

```bash
make verify
```

Use narrower commands only when an issue explicitly asks for a smaller check.

Current baseline is Linux-friendly and does not require Xcode. Add Xcode, Go, TypeScript, and OpenAPI checks as implementation lands.

## Open Pilot

Open Pilot tasks must include:

- `ready-for-agent`
- deterministic `Test Command`
- specific acceptance criteria
- explicit out-of-scope notes

Do not add `agent:queued` until the issue is final and safe for automation.
