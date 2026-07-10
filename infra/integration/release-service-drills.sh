#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_DIR="$ROOT/.artifacts/release-drills"
MODE=${1:-all}
mkdir -p "$EVIDENCE_DIR"

record_evidence() {
  local drill=$1
  local command=$2
  local output=$3
  local details=$4
  local started=$5
  local finished duration timestamp commit operator
  finished=$(date +%s)
  duration=$((finished - started))
  timestamp=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
  commit=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf 'unknown')
  operator=${USER:-unknown}
  grep -q 'DRILL_EVIDENCE' "$output"
  cat >"$EVIDENCE_DIR/$drill.json" <<JSON
{
  "drill": "$drill",
  "timestamp_utc": "$timestamp",
  "commit": "$commit",
  "operator": "$operator",
  "command": "$command",
  "duration_seconds": $duration,
  $details,
  "result": "passed"
}
JSON
  echo "$drill passed; evidence: $EVIDENCE_DIR/$drill.json"
}

moderation_outage() {
  local output started command
  output=$(mktemp)
  trap 'rm -f "$output"' RETURN
  started=$(date +%s)
  command='go test ./apps/worker/internal/media -run TestReleaseDrillModerationHostOutageFailsClosed -count=1 -v'
  (cd "$ROOT" && go test ./apps/worker/internal/media -run TestReleaseDrillModerationHostOutageFailsClosed -count=1 -v) | tee "$output"
  record_evidence "moderation-host-outage" "$command" "$output" '"media_status": "pending_review", "risk": "unknown", "media_public": false, "fail_closed": true' "$started"
  rm -f "$output"
  trap - RETURN
}

apns_mock() {
  local output started command
  output=$(mktemp)
  trap 'rm -f "$output"' RETURN
  started=$(date +%s)
  command='go test ./apps/worker/internal/push -run TestReleaseDrillAPNsSandboxProductionDispatch -count=1 -v'
  (cd "$ROOT" && go test ./apps/worker/internal/push -run TestReleaseDrillAPNsSandboxProductionDispatch -count=1 -v) | tee "$output"
  record_evidence "apns-sandbox-production-mock" "$command" "$output" '"sandbox_dispatched": true, "production_dispatched": true, "dispatch_started_under_seconds": 60, "invalid_token_disabled": true, "safe_payload_checked": true' "$started"
  rm -f "$output"
  trap - RETURN
}

case "$MODE" in
  moderation) moderation_outage ;;
  apns) apns_mock ;;
  all) moderation_outage; apns_mock ;;
  *) echo "usage: $0 [moderation|apns|all]" >&2; exit 2 ;;
esac
