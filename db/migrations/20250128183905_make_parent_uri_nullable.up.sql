-- Convert empty strings to NULL
UPDATE posts 
SET parent_uri = NULL 
WHERE parent_uri = '';

-- Add a check constraint to prevent empty strings in the future
ALTER TABLE posts 
ADD CONSTRAINT parent_uri_not_empty 
CHECK (parent_uri IS NULL OR parent_uri <> '');