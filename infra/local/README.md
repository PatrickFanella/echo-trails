# Local development services

`infra/local/compose.yaml` starts disposable local dependencies for development:

- Postgres 16 with PostGIS on `localhost:5432`
- Redis 7 on `localhost:6379`
- MinIO S3-compatible object storage on `localhost:9000` with console on `localhost:9001`

Run from the repository root:

```bash
docker compose -f infra/local/compose.yaml up -d
docker compose -f infra/local/compose.yaml ps
docker compose -f infra/local/compose.yaml down
```

The checked-in usernames, passwords, bucket names, and ports are non-secret development defaults only. Do not reuse them for staging or production, and do not commit `.env` files, production credentials, signing keys, or object-storage access keys.

Useful local connection values:

- Database URL: `postgres://echotrails:echotrails_dev_password@localhost:5432/echotrails_dev?sslmode=disable`
- Redis URL: `redis://localhost:6379/0`
- S3 endpoint: `http://localhost:9000`
- S3 bucket: `echotrails-dev`
- S3 access key: `echotrails`
- S3 secret key: `echotrails_dev_password`

Optional port overrides are supported with `ECHOTRAILS_LOCAL_POSTGRES_PORT`, `ECHOTRAILS_LOCAL_REDIS_PORT`, `ECHOTRAILS_LOCAL_S3_PORT`, and `ECHOTRAILS_LOCAL_S3_CONSOLE_PORT`.

`make verify` validates the compose file with Docker Compose when it is installed. On Linux runners without Docker, it runs the repository's lightweight static compose check instead so verification remains host-friendly.
