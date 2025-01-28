-- Remove the check constraint
ALTER TABLE posts 
DROP CONSTRAINT parent_uri_not_empty;