-- Drop triggers first
DROP TRIGGER IF EXISTS posts_ai;
DROP TRIGGER IF EXISTS posts_ad;
DROP TRIGGER IF EXISTS posts_au;

-- Drop FTS table
DROP TABLE IF EXISTS posts_fts;

-- Remove text column from posts
ALTER TABLE posts DROP COLUMN text;
