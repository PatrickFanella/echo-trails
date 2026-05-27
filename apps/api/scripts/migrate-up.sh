#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  printf 'DATABASE_URL is required\n' >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
migrations_dir="${script_dir}/../db/migrations"

for migration in "${migrations_dir}"/*.up.sql; do
  printf 'applying %s\n' "$(basename "${migration}")"
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 --single-transaction -f "${migration}"
done
