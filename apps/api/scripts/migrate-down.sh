#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  printf 'DATABASE_URL is required\n' >&2
  exit 2
fi

if [[ "${ECHOTRAILS_ALLOW_DESTRUCTIVE_MIGRATION_DOWN:-}" != "1" ]]; then
  printf 'ECHOTRAILS_ALLOW_DESTRUCTIVE_MIGRATION_DOWN=1 is required because down migrations drop core tables\n' >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
migrations_dir="${script_dir}/../db/migrations"

mapfile -t migrations < <(printf '%s\n' "${migrations_dir}"/*.down.sql | sort -r)
for migration in "${migrations[@]}"; do
  printf 'rolling back %s\n' "$(basename "${migration}")"
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 --single-transaction -f "${migration}"
done
