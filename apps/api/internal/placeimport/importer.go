package placeimport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Attribution = "© OpenStreetMap contributors"

type CatalogEntry struct {
	ID, ExternalID, Name, Status, Category string
	RadiusMeters                           int
	Sensitive                              bool
	Aliases                                []string
}

var DownersGroveCatalog = []CatalogEntry{
	{ID: "019f486a-0001-7000-8000-000000000001", ExternalID: "way/35362323", Name: "Prince Pond", Status: "active", Category: "landmark", RadiusMeters: 100, Aliases: []string{"Prince Pond Park"}},
	{ID: "019f486a-0002-7000-8000-000000000002", ExternalID: "node/25779937", Name: "Downers Grove Main Street Metra", Status: "active", Category: "transit_station", RadiusMeters: 100, Aliases: []string{"Main Street Station", "Downers Grove Metra"}},
	{ID: "019f486a-0003-7000-8000-000000000003", ExternalID: "way/200192088", Name: "Downers Grove Public Library", Status: "active", Category: "library", RadiusMeters: 75, Aliases: []string{"DGPL"}},
	{ID: "019f486a-0004-7000-8000-000000000004", ExternalID: "way/200218663", Name: "Tivoli Theatre", Status: "active", Category: "theatre", RadiusMeters: 75},
	{ID: "019f486a-0005-7000-8000-000000000005", ExternalID: "way/200054182", Name: "Fishel Park", Status: "active", Category: "park", RadiusMeters: 150},
	{ID: "019f486a-0006-7000-8000-000000000006", ExternalID: "way/552662784", Name: "Washington Park", Status: "active", Category: "park", RadiusMeters: 200},
	{ID: "019f486a-0007-7000-8000-000000000007", ExternalID: "way/567113043", Name: "Constitution Park and Lincoln Center", Status: "active", Category: "park", RadiusMeters: 200, Aliases: []string{"Constitution Park", "Lincoln Center"}},
	{ID: "019f486a-0008-7000-8000-000000000008", ExternalID: "way/684600471", Name: "Downers Grove Museum", Status: "active", Category: "museum", RadiusMeters: 100, Aliases: []string{"Wandschneider Park"}},
	{ID: "019f486a-0009-7000-8000-000000000009", ExternalID: "way/548571172", Name: "Downers Grove North High School", Status: "restricted", Category: "school", RadiusMeters: 250, Sensitive: true},
}

type Feature struct {
	Type       string                     `json:"type"`
	Properties map[string]json.RawMessage `json:"properties"`
	Geometry   json.RawMessage            `json:"geometry"`
}

type geometryHeader struct {
	Type string `json:"type"`
}

type Source struct {
	Version, URL, Attribution string
	ImportedAt                time.Time
}

func DecodeFeatures(reader io.Reader) (map[string]Feature, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 256<<20))
	if err != nil {
		return nil, err
	}
	var collection struct {
		Type     string    `json:"type"`
		Features []Feature `json:"features"`
	}
	features := []Feature{}
	if json.Unmarshal(data, &collection) == nil && collection.Type == "FeatureCollection" {
		features = collection.Features
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), 16<<20)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 || line[0] == 0x1e {
				line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte{0x1e}))
			}
			if len(line) == 0 {
				continue
			}
			var feature Feature
			if err := json.Unmarshal(line, &feature); err != nil {
				return nil, fmt.Errorf("decode GeoJSON sequence: %w", err)
			}
			features = append(features, feature)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	result := make(map[string]Feature, len(features))
	for _, feature := range features {
		if feature.Type != "Feature" || len(feature.Geometry) == 0 {
			continue
		}
		id := featureID(feature.Properties)
		if id != "" {
			result[id] = feature
		}
	}
	return result, nil
}

func featureID(properties map[string]json.RawMessage) string {
	for _, key := range []string{"@id", "id", "osm_id"} {
		var value string
		if json.Unmarshal(properties[key], &value) == nil && value != "" {
			if key == "osm_id" && !strings.Contains(value, "/") {
				var kind string
				_ = json.Unmarshal(properties["osm_type"], &kind)
				if kind != "" {
					return strings.ToLower(kind) + "/" + value
				}
			}
			return strings.ToLower(value)
		}
	}
	return ""
}

func Import(ctx context.Context, pool *pgxpool.Pool, features map[string]Feature, source Source) error {
	if pool == nil || strings.TrimSpace(source.Version) == "" || strings.TrimSpace(source.URL) == "" {
		return errors.New("database, source version, and source URL are required")
	}
	if source.Attribution == "" {
		source.Attribution = Attribution
	}
	missing := []string{}
	for _, entry := range DownersGroveCatalog {
		if _, ok := features[entry.ExternalID]; !ok {
			missing = append(missing, entry.ExternalID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("extract is missing required public Places: %s", strings.Join(missing, ", "))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	importID, err := uuidv7.New()
	if err != nil {
		return err
	}
	if source.ImportedAt.IsZero() {
		source.ImportedAt = time.Now().UTC()
	}
	if _, err = tx.Exec(ctx, `INSERT INTO place_imports (id, provider, source_version, source_url, attribution, bounds, imported_at)
		VALUES ($1, 'openstreetmap', $2, $3, $4, jsonb_build_object('catalog', 'downers_grove_public_demo'), $5)`,
		importID.String(), source.Version, source.URL, source.Attribution, source.ImportedAt.UTC()); err != nil {
		return err
	}
	for _, entry := range DownersGroveCatalog {
		if err = importEntry(ctx, tx, entry, features[entry.ExternalID], importID.String(), source); err != nil {
			return fmt.Errorf("import %s: %w", entry.ExternalID, err)
		}
	}
	return tx.Commit(ctx)
}

func importEntry(ctx context.Context, tx pgx.Tx, entry CatalogEntry, feature Feature, importID string, source Source) error {
	var header geometryHeader
	if err := json.Unmarshal(feature.Geometry, &header); err != nil {
		return err
	}
	if header.Type != "Point" && header.Type != "Polygon" && header.Type != "MultiPolygon" {
		return fmt.Errorf("unsupported geometry %q", header.Type)
	}
	payload, _ := json.Marshal(map[string]any{"attribution": source.Attribution, "source_version": source.Version, "place_import_id": importID})
	command, err := tx.Exec(ctx, `WITH raw AS (
		SELECT ST_SetSRID(ST_GeomFromGeoJSON($8), 4326) AS geometry
	), normalized AS (
		SELECT geometry,
			CASE WHEN GeometryType(geometry) = 'POINT' THEN geometry ELSE ST_PointOnSurface(geometry) END AS center,
			CASE WHEN GeometryType(geometry) IN ('POLYGON', 'MULTIPOLYGON')
				THEN ST_Multi(ST_CollectionExtract(ST_MakeValid(geometry), 3)) ELSE NULL END AS boundary
		FROM raw
	), accepted AS (
		SELECT * FROM normalized WHERE ST_X(center) BETWEEN -88.15 AND -87.85 AND ST_Y(center) BETWEEN 41.70 AND 41.90
	)
	INSERT INTO places (id, name, normalized_name, status, location, boundary, default_radius_meters, category, sensitive, updated_at)
	SELECT $1, $2, lower($2), $3, center::geography, boundary::geography, $4, $5, $6, $7 FROM accepted
	ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, normalized_name = EXCLUDED.normalized_name,
		status = EXCLUDED.status, location = EXCLUDED.location, boundary = EXCLUDED.boundary,
		default_radius_meters = EXCLUDED.default_radius_meters, category = EXCLUDED.category,
		sensitive = EXCLUDED.sensitive, updated_at = EXCLUDED.updated_at, deleted_at = NULL`,
		entry.ID, entry.Name, entry.Status, entry.RadiusMeters, entry.Category, entry.Sensitive, source.ImportedAt.UTC(), string(feature.Geometry))
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("geometry center is outside the reviewed regional bounds")
	}
	mappingID, err := uuidv7.New()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO place_provider_mappings (id, place_id, provider, provider_place_id, provider_payload)
		VALUES ($1, $2, 'openstreetmap', $3, $4)
		ON CONFLICT (provider, provider_place_id) DO UPDATE SET place_id = EXCLUDED.place_id, provider_payload = EXCLUDED.provider_payload`,
		mappingID.String(), entry.ID, entry.ExternalID, payload); err != nil {
		return err
	}
	for _, alias := range entry.Aliases {
		aliasID, idErr := uuidv7.New()
		if idErr != nil {
			return idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO place_aliases (id, place_id, alias, normalized_alias)
			VALUES ($1, $2, $3, lower($3)) ON CONFLICT (place_id, normalized_alias) DO UPDATE SET alias = EXCLUDED.alias`, aliasID.String(), entry.ID, alias); err != nil {
			return err
		}
	}
	return nil
}
