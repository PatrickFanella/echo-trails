package placeimport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationImportsCuratedOSMBoundariesAndProvenance(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	centers := map[string][2]float64{
		"way/35362323": {-88.0153297, 41.7975912}, "node/25779937": {-88.0086302, 41.7955440},
		"way/200192088": {-88.0114616, 41.7940943}, "way/200218663": {-88.0088001, 41.7960844},
		"way/200054182": {-88.0116448, 41.7928667}, "way/552662784": {-88.0057961, 41.7992530},
		"way/567113043": {-88.0089976, 41.7907635}, "way/684600471": {-88.0051168, 41.7925048},
		"way/548571172": {-88.0122581, 41.8065417},
	}
	features := map[string]Feature{}
	for _, entry := range DownersGroveCatalog {
		center := centers[entry.ExternalID]
		geometry := fmt.Sprintf(`{"type":"Point","coordinates":[%.7f,%.7f]}`, center[0], center[1])
		if entry.ExternalID[:4] == "way/" {
			geometry = fmt.Sprintf(`{"type":"Polygon","coordinates":[[[%.7f,%.7f],[%.7f,%.7f],[%.7f,%.7f],[%.7f,%.7f],[%.7f,%.7f]]]}`,
				center[0]-.0002, center[1]-.0002, center[0]+.0002, center[1]-.0002,
				center[0]+.0002, center[1]+.0002, center[0]-.0002, center[1]+.0002, center[0]-.0002, center[1]-.0002)
		}
		features[entry.ExternalID] = Feature{Type: "Feature", Geometry: json.RawMessage(geometry)}
	}
	const version = "integration-osm-boundaries-v1"
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE places SET boundary = NULL WHERE id = ANY($1::uuid[])`, catalogIDs())
		_, _ = pool.Exec(context.Background(), `DELETE FROM place_imports WHERE source_version = $1`, version)
	}()
	if err = Import(ctx, pool, features, Source{Version: version, URL: "https://download.geofabrik.de/north-america/us/illinois.html", ImportedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var boundaries, pointFallback, provenance int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE boundary IS NOT NULL), count(*) FILTER (WHERE boundary IS NULL)
		FROM places WHERE id = ANY($1::uuid[])`, catalogIDs()).Scan(&boundaries, &pointFallback); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM place_imports WHERE provider = 'openstreetmap' AND source_version = $1
		AND attribution = $2`, version, Attribution).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if boundaries != 8 || pointFallback != 1 || provenance != 1 {
		t.Fatalf("boundaries=%d point_fallback=%d provenance=%d", boundaries, pointFallback, provenance)
	}
}

func catalogIDs() []string {
	ids := make([]string, 0, len(DownersGroveCatalog))
	for _, entry := range DownersGroveCatalog {
		ids = append(ids, entry.ID)
	}
	return ids
}
