package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/placeimport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var inputPath, sourceVersion, sourceURL, attribution string
	flag.StringVar(&inputPath, "geojson", "", "GeoJSON FeatureCollection or GeoJSON sequence exported from a regional OSM extract")
	flag.StringVar(&sourceVersion, "source-version", "", "immutable regional extract version or checksum")
	flag.StringVar(&sourceURL, "source-url", "", "public regional extract source URL")
	flag.StringVar(&attribution, "attribution", placeimport.Attribution, "required OpenStreetMap attribution")
	flag.Parse()
	if inputPath == "" || sourceVersion == "" || sourceURL == "" {
		flag.Usage()
		os.Exit(2)
	}
	file, err := os.Open(inputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	features, err := placeimport.DecodeFeatures(file)
	if err != nil {
		log.Fatalf("decode extract: %v", err)
	}
	databaseURL := os.Getenv("ECHOTRAILS_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		log.Fatal("ECHOTRAILS_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = placeimport.Import(ctx, pool, features, placeimport.Source{Version: sourceVersion, URL: sourceURL, Attribution: attribution, ImportedAt: time.Now().UTC()}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("imported %d curated public OSM Places\n", len(placeimport.DownersGroveCatalog))
}
