DROP TRIGGER IF EXISTS place_alias_search_update ON place_aliases;
DROP FUNCTION IF EXISTS echotrails_place_alias_search_update();

CREATE OR REPLACE FUNCTION echotrails_places_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.normalized_name := lower(coalesce(NEW.name, ''));
    NEW.search_document := setweight(to_tsvector('simple', coalesce(NEW.name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.description, '')), 'C');
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION echotrails_echoes_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.body_text, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.caption_text, '')), 'B');
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION echotrails_transcripts_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := to_tsvector('simple', coalesce(NEW.transcript_text, ''));
    RETURN NEW;
END;
$$;

UPDATE places SET name = name;
UPDATE place_aliases SET normalized_alias = lower(alias);
UPDATE echoes SET title = title;
UPDATE media_transcripts SET transcript_text = transcript_text;
