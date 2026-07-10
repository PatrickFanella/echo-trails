# Downers Grove Demo Place Catalog

## Purpose and privacy boundary

This is the reviewed public-Place catalog for the invite-gated Downers Grove
beta. The residential address used to choose the general demo area is not an
input to the importer, is not stored in source or runtime data, and must not be
sent to clients, logs, analytics, or push payloads.

The catalog is deliberately finite. Production autocomplete and discovery use
EchoTrails-owned Postgres records; they do not query public Nominatim.

## Reviewed catalog

Reviewed 2026-07-09. OSM identifiers are the immutable inputs selected from a
regional extract. Public-source links confirm the identity and public nature of
each Place; the OSM extract supplies the geometry.

| Place | State | OSM identifier | Public identity source | Geometry policy |
| --- | --- | --- | --- | --- |
| Prince Pond | Active | `way/35362323` | [Downers Grove Park District](https://www.dgparks.org/location/prince-pond) | Imported polygon, 25 m access buffer |
| Downers Grove Main Street Metra | Active | `node/25779937` | [Metra ticket-vending station list](https://www.metra.com/tvm) | Point, reviewed 100 m radius |
| Downers Grove Public Library | Active | `way/200192088` | [Downers Grove Public Library](https://dglibrary.org/contact/) | Imported polygon, 25 m access buffer |
| Tivoli Theatre | Active | `way/200218663` | [Classic Cinemas](https://www.classiccinemas.com/tivoli/info) | Imported polygon, 25 m access buffer |
| Fishel Park | Active | `way/200054182` | [Downers Grove Park District](https://www.dgparks.org/location/fishel-park) | Imported polygon, 25 m access buffer |
| Washington Park | Active | `way/552662784` | [Downers Grove Park District](https://www.dgparks.org/location/washington-park) | Imported polygon, 25 m access buffer |
| Constitution Park and Lincoln Center | Active | `way/567113043` | [Constitution Park](https://www.dgparks.org/location/constitution-park) and [Lincoln Center](https://www.dgparks.org/lincoln-center) | Imported polygon, 25 m access buffer |
| Downers Grove Museum | Active | `way/684600471` | [Downers Grove Park District](https://www.dgparks.org/downers-grove-museum) | Imported polygon, 25 m access buffer |
| Downers Grove North High School | Restricted and sensitive | `way/548571172` | [Community High School District 99](https://www.csd99.org/dgn) | Never eligible for ordinary Echo creation; retained for the negative test path |

The catalog's stable EchoTrails IDs, names, aliases, categories, and fallback
radii are defined in `apps/api/internal/placeimport/importer.go`. Imported
polygons are authoritative for creation checks. The curated radius is consulted
only for a point Place or if a reviewed record has no polygon.

## Import

Use a dated regional `.osm.pbf` extract from a reproducible provider. Record the
extract version and its public download URL:

```bash
export ECHOTRAILS_DATABASE_URL='postgres://...'
./apps/api/scripts/import-downers-grove-places.sh \
  /secure/path/illinois-YYYYMMDD.osm.pbf \
  YYYY-MM-DD \
  https://provider.example/illinois-YYYYMMDD.osm.pbf
```

The script extracts only the reviewed identifiers, preserves referenced nodes,
converts them to GeoJSON Sequence, and invokes the transactional Go importer.
Every import records source version, URL, timestamp, and `© OpenStreetMap
contributors` attribution. The source extract is operational input and must not
be committed.

## Review and verification

Before promoting a new extract:

1. Confirm all nine reviewed OSM identifiers exist in that extract.
2. Confirm every geometry center falls inside the importer's narrow reviewed
   Downers Grove regional bounds.
3. Inspect polygon shapes and point centers in the admin Place boundary tool.
4. Confirm the eight active Places accept fresh, accurate creation proofs inside
   their boundaries and reject proofs outside them.
5. Confirm Downers Grove North High School remains sensitive and restricted and
   rejects ordinary creation even for an inside-boundary proof.
6. Confirm public API responses contain only Place/public jittered coordinates,
   never exact creation coordinates or private origin data.
7. Run `make integration-test` and retain the Place import/location-proof test
   output with the release evidence.

Geometry edits and Place merges require a moderator/admin reason and audit
record. Do not silently repoint an existing provider identifier to a different
physical Place.
