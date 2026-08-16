#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 REGION.osm.pbf SOURCE_VERSION PUBLIC_SOURCE_URL" >&2
  exit 2
fi

for command in osmium go; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 2; }
done

extract=$1
source_version=$2
source_url=$3
[[ -f "$extract" ]] || { echo "regional OSM extract not found" >&2; exit 2; }
: "${ECHOTRAILS_DATABASE_URL:?ECHOTRAILS_DATABASE_URL is required}"

workdir=$(mktemp -d "${TMPDIR:-/tmp}/echotrails-osm-import.XXXXXX")
trap 'rm -rf "$workdir"' EXIT

osmium getid --add-referenced "$extract" \
  w35362323 n25779937 w200192088 w200218663 w200054182 \
  w552662784 w567113043 w684600471 w548571172 \
  -o "$workdir/curated.osm.pbf"
osmium export "$workdir/curated.osm.pbf" --geometry-types=point,polygon \
  --format=geojsonseq -o "$workdir/curated.geojsonseq"

go run ./apps/api/cmd/import-places \
  --geojson "$workdir/curated.geojsonseq" \
  --source-version "$source_version" \
  --source-url "$source_url"
