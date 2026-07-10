-- Make the installed unaccent extension part of the authoritative search
-- pipeline for Places, aliases, Echo text/captions, and transcripts.

CREATE OR REPLACE FUNCTION echotrails_places_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.normalized_name := lower(unaccent(coalesce(NEW.name, '')));
    NEW.search_document := setweight(to_tsvector('simple', unaccent(coalesce(NEW.name, ''))), 'A') ||
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.description, ''))), 'C');
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION echotrails_echoes_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := setweight(to_tsvector('simple', unaccent(coalesce(NEW.title, ''))), 'A') ||
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.body_text, ''))), 'B') ||
        setweight(to_tsvector('simple', unaccent(coalesce(NEW.caption_text, ''))), 'B');
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION echotrails_transcripts_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := to_tsvector('simple', unaccent(coalesce(NEW.transcript_text, '')));
    RETURN NEW;
END;
$$;

CREATE FUNCTION echotrails_place_alias_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.normalized_alias := lower(unaccent(coalesce(NEW.alias, '')));
    RETURN NEW;
END;
$$;

CREATE TRIGGER place_alias_search_update BEFORE INSERT OR UPDATE OF alias
ON place_aliases FOR EACH ROW EXECUTE FUNCTION echotrails_place_alias_search_update();

UPDATE places SET name = name;
UPDATE place_aliases SET alias = alias;
UPDATE echoes SET title = title;
UPDATE media_transcripts SET transcript_text = transcript_text;
