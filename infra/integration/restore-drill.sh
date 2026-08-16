#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT/infra/integration/compose.yaml"
COMPOSE=(docker compose -f "$COMPOSE_FILE")
SOURCE_DB=echotrails_integration
RESTORE_DB=echotrails_restore_drill
DB_USER=echotrails
BACKUP_FILE=/tmp/echotrails-restore-drill.dump
SOURCE_BUCKET=echotrails-integration
BACKUP_BUCKET=echotrails-restore-drill-backup
RESTORE_BUCKET=echotrails-restore-drill-restored
OBJECT_CANARY=release-drills/restore-canary.txt
EVIDENCE_DIR="$ROOT/.artifacts/release-drills"
EVIDENCE_FILE="$EVIDENCE_DIR/restore-drill.json"

mkdir -p "$EVIDENCE_DIR"

cleanup() {
  "${COMPOSE[@]}" exec -T postgres dropdb --if-exists --force -U "$DB_USER" "$RESTORE_DB" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T postgres psql -U "$DB_USER" -d "$SOURCE_DB" -c "DELETE FROM places WHERE id = '01960f67-a100-7000-8000-000000000002'; DELETE FROM users WHERE id = '01960f67-a100-7000-8000-000000000001'" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T postgres rm -f "$BACKUP_FILE" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T object-storage mc rm --force "local/$SOURCE_BUCKET/$OBJECT_CANARY" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T object-storage mc rb --force "local/$BACKUP_BUCKET" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T object-storage mc rb --force "local/$RESTORE_BUCKET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${COMPOSE[@]}" up -d --wait postgres object-storage
"${COMPOSE[@]}" up -d migrations object-storage-init
"${COMPOSE[@]}" wait migrations object-storage-init
"${COMPOSE[@]}" exec -T object-storage mc alias set local http://127.0.0.1:9000 integration integration-secret >/dev/null

# Add canaries that exercise both ordinary relational data and a PostGIS value.
"${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$SOURCE_DB" <<'SQL'
INSERT INTO users (id, email, status, created_at, updated_at)
VALUES ('01960f67-a100-7000-8000-000000000001', 'restore-drill@example.invalid', 'active', now(), now())
ON CONFLICT (id) DO UPDATE SET updated_at = excluded.updated_at;

INSERT INTO places (id, name, normalized_name, status, location, category, sensitive, created_at, updated_at)
VALUES (
  '01960f67-a100-7000-8000-000000000002',
  'Restore Drill Place',
  'restore-drill-place',
  'active',
  ST_SetSRID(ST_MakePoint(-88.0100, 41.7950), 4326)::geography,
  'test-fixture',
  false,
  now(),
  now()
)
ON CONFLICT (id) DO UPDATE SET updated_at = excluded.updated_at;
SQL

printf 'EchoTrails restore drill media canary\n' | "${COMPOSE[@]}" exec -T object-storage mc pipe "local/$SOURCE_BUCKET/$OBJECT_CANARY" >/dev/null
"${COMPOSE[@]}" exec -T object-storage mc mb --ignore-existing "local/$BACKUP_BUCKET" >/dev/null
"${COMPOSE[@]}" exec -T object-storage mc mirror --overwrite "local/$SOURCE_BUCKET" "local/$BACKUP_BUCKET" >/dev/null

source_schema_count=$("${COMPOSE[@]}" exec -T postgres psql -At -U "$DB_USER" -d "$SOURCE_DB" -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'")
source_object_count=$("${COMPOSE[@]}" exec -T object-storage sh -c "mc find local/$SOURCE_BUCKET --name '*' 2>/dev/null | wc -l" | tr -d '[:space:]')
source_object_hash=$("${COMPOSE[@]}" exec -T object-storage sh -c "mc cat local/$SOURCE_BUCKET/$OBJECT_CANARY | sha256sum" | awk '{print $1}')

"${COMPOSE[@]}" exec -T postgres pg_dump -U "$DB_USER" -d "$SOURCE_DB" --format=custom --file="$BACKUP_FILE"
"${COMPOSE[@]}" exec -T postgres dropdb --if-exists --force -U "$DB_USER" "$RESTORE_DB"
"${COMPOSE[@]}" exec -T postgres createdb -U "$DB_USER" "$RESTORE_DB"
"${COMPOSE[@]}" exec -T postgres pg_restore -U "$DB_USER" -d "$RESTORE_DB" --exit-on-error "$BACKUP_FILE"

restore_schema_count=$("${COMPOSE[@]}" exec -T postgres psql -At -U "$DB_USER" -d "$RESTORE_DB" -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'")
canary_result=$("${COMPOSE[@]}" exec -T postgres psql -At -U "$DB_USER" -d "$RESTORE_DB" -c "SELECT u.email || '|' || p.name || '|' || round(ST_Y(p.location::geometry)::numeric, 4) FROM users u CROSS JOIN places p WHERE u.id = '01960f67-a100-7000-8000-000000000001' AND p.id = '01960f67-a100-7000-8000-000000000002'")
"${COMPOSE[@]}" exec -T object-storage mc mb --ignore-existing "local/$RESTORE_BUCKET" >/dev/null
"${COMPOSE[@]}" exec -T object-storage mc mirror --overwrite "local/$BACKUP_BUCKET" "local/$RESTORE_BUCKET" >/dev/null
restore_object_count=$("${COMPOSE[@]}" exec -T object-storage sh -c "mc find local/$RESTORE_BUCKET --name '*' 2>/dev/null | wc -l" | tr -d '[:space:]')
restore_object_hash=$("${COMPOSE[@]}" exec -T object-storage sh -c "mc cat local/$RESTORE_BUCKET/$OBJECT_CANARY | sha256sum" | awk '{print $1}')

test "$source_schema_count" -gt 0
test "$source_schema_count" = "$restore_schema_count"
test "$canary_result" = 'restore-drill@example.invalid|Restore Drill Place|41.7950'
test "$source_object_count" -gt 0
test "$source_object_count" = "$restore_object_count"
test "$source_object_hash" = "$restore_object_hash"

timestamp=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
commit=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf 'unknown')
backup_id=$("${COMPOSE[@]}" exec -T postgres sha256sum "$BACKUP_FILE" | awk '{print $1}')
operator=${USER:-unknown}

cat >"$EVIDENCE_FILE" <<JSON
{
  "drill": "postgres-postgis-and-object-restore",
  "timestamp_utc": "$timestamp",
  "commit": "$commit",
  "backup_sha256": "$backup_id",
  "operator": "$operator",
  "source_database": "$SOURCE_DB",
  "isolated_restore_database": "$RESTORE_DB",
  "public_schema_tables": $restore_schema_count,
  "restored_object_count": $restore_object_count,
  "restored_object_canary_sha256": "$restore_object_hash",
  "result": "passed"
}
JSON

echo "restore drill passed; evidence: $EVIDENCE_FILE"
