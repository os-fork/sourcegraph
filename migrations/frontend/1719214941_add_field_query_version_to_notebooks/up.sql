DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'query_version_enum') THEN
        CREATE TYPE query_version_enum AS ENUM ('V1', 'V2', 'V3', 'V4');
    END IF;
END
$$;


ALTER TABLE notebooks ADD COLUMN IF NOT EXISTS query_version query_version_enum NOT NULL DEFAULT 'V3';
