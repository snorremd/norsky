ALTER TABLE posts 
ADD COLUMN author_did TEXT NOT NULL DEFAULT '';

-- Create index for author lookups
CREATE INDEX posts_author_did_idx ON posts(author_did); 