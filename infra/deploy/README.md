# EchoTrails Deployment Infrastructure

Production-capable container deployment for the invite-gated beta.

Keep production credentials, signing material, host-specific secrets, and deploy hooks out of git unless a later issue explicitly defines a safe, secret-free configuration boundary.

The stack includes ordered PostGIS migrations, Redis, private object storage,
ClamAV, the GPU moderation service, API, worker, and both web applications.
It is intended to sit behind a TLS ingress or load balancer.

- Copy `env.example` to `.env` and replace every `provided-*` value using the
  deployment secret store. Never commit `.env` or the APNs key.
- Set `ECHOTRAILS_APNS_PRIVATE_KEY_HOST_PATH` to the host-side `.p8` file.
- Run `docker compose --env-file .env -f infra/deploy/compose.yaml config`.
- Run `docker compose --env-file .env -f infra/deploy/compose.yaml up -d`.
- Confirm migrations complete, `/ready` succeeds, worker jobs drain, and both
  web applications respond through the TLS ingress.
- `monitoring-dashboard.fixture.json` defines the expected dashboard signals.

The release runbook records the required backup/restore, moderation-outage,
APNs, and TestFlight drills. Those environment-specific drills must be logged
for the actual release candidate; repository verification cannot substitute for
credentials, devices, or the production moderation host.

## Downers Grove field environment

The homelab field environment uses `compose.nuc.yaml` for the stateful product
stack and `compose.almaz-moderation.yaml` for the GPU service. Copy
`env.field.example` to `.env.field` on NUC and `env.moderation.example` to
`.env.moderation` on Almaz. Model weights and temporary files are bind-mounted
from `/mnt/impuls/echotrails`; they do not consume Docker root storage. Follow
`docs/runbooks/field-demo.md` for capacity, firewall, ingress, bootstrap, Place
import, verification, and rollback.
