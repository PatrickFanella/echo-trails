package placeimport

import (
	"strings"
	"testing"
)

func TestDecodeFeaturesSupportsCollectionAndSequence(t *testing.T) {
	collection := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"@id":"way/1"},"geometry":{"type":"Polygon","coordinates":[[[-88,41],[-88,41.1],[-87.9,41.1],[-88,41]]]}}]}`
	features, err := DecodeFeatures(strings.NewReader(collection))
	if err != nil || len(features) != 1 || len(features["way/1"].Geometry) == 0 {
		t.Fatalf("collection features=%v err=%v", features, err)
	}
	sequence := "\x1e" + `{"type":"Feature","properties":{"osm_id":"2","osm_type":"Node"},"geometry":{"type":"Point","coordinates":[-88,41]}}` + "\n"
	features, err = DecodeFeatures(strings.NewReader(sequence))
	if err != nil || len(features) != 1 || len(features["node/2"].Geometry) == 0 {
		t.Fatalf("sequence features=%v err=%v", features, err)
	}
}

func TestCatalogContainsOnlyCuratedPublicDemoTargets(t *testing.T) {
	if len(DownersGroveCatalog) != 9 {
		t.Fatalf("catalog count = %d", len(DownersGroveCatalog))
	}
	seen := map[string]bool{}
	for _, entry := range DownersGroveCatalog {
		if entry.ID == "" || entry.ExternalID == "" || entry.Name == "" || seen[entry.ExternalID] {
			t.Fatalf("invalid catalog entry: %+v", entry)
		}
		seen[entry.ExternalID] = true
		if entry.Status != "active" && !(entry.Status == "restricted" && entry.Sensitive) {
			t.Fatalf("unsafe demo status: %+v", entry)
		}
	}
}
