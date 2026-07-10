# EchoTrails API

Go REST API service for EchoTrails.

This directory contains the production API handlers, server primitives,
persistence repositories, migration tooling, and the curated Place importer.
Keep implementation aligned with `docs/technical-plan.md`; the top-level
generated iOS prototype is not an API source of truth.

The production binary is `cmd/api` and requires Postgres/PostGIS. Bearer access
is invite-, consent-, profile-, and account-state gated through
`GET /account/bootstrap`; public routes and role-protected admin-cookie routes
are enumerated and linted from OpenAPI. Search uses unaccented full text and
trigram Place matching plus caption/transcript-aware Echo ranking. Notification
and analytics preferences have side-effect-free reads as well as authenticated
updates.

## Curated OSM Place import

The beta catalog is refreshed from a downloaded regional OpenStreetMap PBF,
never from public Nominatim. From the repository root, provide an immutable
extract version/checksum and its public source URL:

```bash
ECHOTRAILS_DATABASE_URL='postgres://...' \
  apps/api/scripts/import-downers-grove-places.sh \
  /secure/imports/illinois-latest.osm.pbf \
  2026-07-01-sha256-EXTRACT_DIGEST \
  https://download.geofabrik.de/north-america/us/illinois.html
```

The script extracts only the reviewed public demo OSM objects, preserves OSM
attribution and source provenance in `place_imports`, imports Polygon and
MultiPolygon boundaries, and leaves the restricted school in the negative-path
catalog. It does not accept or store a residential origin address.
