CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    uri TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    indexed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    text TEXT,
    parent_uri TEXT,
    languages TEXT[] NOT NULL DEFAULT '{}',  -- Array of language codes
    -- Use simple configuration which doesn't do language-specific processing
    -- This provides basic tokenization without stemming or stop words
    ts_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(text, ''))) STORED
);

-- Create indices for common query patterns
CREATE INDEX posts_created_at_idx ON posts(created_at DESC);
CREATE INDEX posts_indexed_at_idx ON posts(indexed_at DESC);
CREATE INDEX posts_uri_idx ON posts(uri);
CREATE INDEX posts_parent_uri_idx ON posts(parent_uri);
CREATE INDEX posts_ts_vector_idx ON posts USING GIN(ts_vector);
CREATE INDEX posts_languages_idx ON posts USING GIN(languages); -- GIN index for array searches