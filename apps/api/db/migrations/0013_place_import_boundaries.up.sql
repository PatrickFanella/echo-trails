-- Imported OSM Places may be Polygon or MultiPolygon features. Normalize both
-- to MultiPolygon so administrative tools and the creation-proof path share a
-- single authoritative PostGIS representation.

ALTER TABLE places
    ALTER COLUMN boundary TYPE geography(MultiPolygon, 4326)
    USING CASE WHEN boundary IS NULL THEN NULL ELSE ST_Multi(boundary::geometry)::geography END;
