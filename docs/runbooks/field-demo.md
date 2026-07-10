# Field Demo Runbook

## Architecture

NUC owns all stateful product services, while Almaz owns public ingress and GPU
moderation. Public traffic flows through Cloudflare to Almaz Caddy and then to
the NUC LAN bindings.

| Service | Host and port | Exposure |
| --- | --- | --- |
| API | `10.0.0.56:18080` | Almaz only |
| Admin web | `10.0.0.56:18082` | Almaz only; Authelia at ingress |
| Public web | `10.0.0.56:18083` | Almaz only |
| MinIO API | `10.0.0.56:19000` | Almaz moderation only |
| Moderation | `10.0.0.200:18081` | NUC worker only |

Postgres and Redis have no host publish. The public hostnames are
`api.echotrails.subcult.tv`, `app.echotrails.subcult.tv`, and
`admin.echotrails.subcult.tv`.

## Preconditions

1. Deploy only a tagged commit for which all release gates pass.
2. Confirm NUC Docker health and free space on `/srv`.
3. Confirm Almaz root has at least 100 GB free and is below 90% use;
   `/mnt/spektr` must also be below 90%.
4. Confirm `nvidia-smi` and an NVIDIA container smoke test succeed.
5. Create `/mnt/impuls/echotrails/{model-cache,tmp}` owned by UID 10001.
6. Copy the example environments to `.env.field` on NUC and `.env.moderation`
   on Almaz, replace every `provided-*` value, set mode `0600`, and keep them
   out of git.
7. Verify Resend for `echotrails.subcult.tv` and obtain immutable Hugging Face
   model revisions before starting services.

## Deploy NUC

```bash
docker compose --env-file infra/deploy/.env.field \
  -f infra/deploy/compose.nuc.yaml config
docker compose --env-file infra/deploy/.env.field \
  -f infra/deploy/compose.nuc.yaml up -d --build
curl --fail http://10.0.0.56:18080/ready
```

The NUC firewall must permit ports 18080, 18082, 18083, and 19000 from
`10.0.0.200` only. Do not weaken the default Docker guard for the rest of the
LAN.

## Deploy Almaz moderation

```bash
docker compose --env-file infra/deploy/.env.moderation \
  -f infra/deploy/compose.almaz-moderation.yaml config
docker compose --env-file infra/deploy/.env.moderation \
  -f infra/deploy/compose.almaz-moderation.yaml up -d --build
curl --fail http://10.0.0.200:18081/ready
```

Permit port 18081 from `10.0.0.56` only. Validate the deployed models with
`services/moderation/calibrate.py --service-url http://10.0.0.200:18081`.

## Bootstrap and Places

1. Request and verify an email code so the user and verified identity exist.
2. Run `/usr/local/bin/echotrails-bootstrap-admin --email <verified-email>` in
   the API container. It is idempotent and refuses promotion after another
   active admin exists.
3. Create the field invite through the audited admin interface.
4. Import a dated, checksummed Illinois PBF with the curated importer and review
   all nine Place records before field use.

## Field install and verification

Build `FieldTest` with a Personal Team. It uses email login and intentionally
has no push, associated-domain, or Apple-login entitlement. Activity remains
server-authoritative. Test all four media modes at two active Places over
cellular, background one upload, and verify outside-boundary and
restricted-school rejection.

## Rollback

Remove the Cloudflare hostname routes first, then stop the NUC application
services without deleting volumes. Stop Almaz moderation independently. Restore
from the pre-deploy database/object inventory when required; never use
`down -v` in this environment.
