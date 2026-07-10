-- A MultiPolygon cannot always be represented by the legacy Polygon type.
-- Refuse a lossy rollback if imported multi-part boundaries are present.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM places
        WHERE boundary IS NOT NULL AND ST_NumGeometries(boundary::geometry) > 1
    ) THEN
        RAISE EXCEPTION 'cannot downgrade Place boundaries while multi-part imports exist';
    END IF;
END;
$$;

ALTER TABLE places
    ALTER COLUMN boundary TYPE geography(Polygon, 4326)
    USING CASE WHEN boundary IS NULL THEN NULL ELSE ST_GeometryN(boundary::geometry, 1)::geography END;
