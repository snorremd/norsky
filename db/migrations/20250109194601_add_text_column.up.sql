ALTER TABLE posts ADD COLUMN text TEXT; 

-- Create virtual FTS table
CREATE VIRTUAL TABLE posts_fts USING fts5(
    text,
    content='posts',
    content_rowid='id',
    tokenize='unicode61'
);

-- Trigger to keep FTS table in sync on insert
CREATE TRIGGER posts_ai AFTER INSERT ON posts BEGIN
    INSERT INTO posts_fts(rowid, text) VALUES (new.id, new.text);
END;

-- Trigger to keep FTS table in sync on delete
CREATE TRIGGER posts_ad AFTER DELETE ON posts BEGIN
    INSERT INTO posts_fts(posts_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

-- Trigger to keep FTS table in sync on update
CREATE TRIGGER posts_au AFTER UPDATE ON posts BEGIN
    INSERT INTO posts_fts(posts_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO posts_fts(rowid, text) VALUES (new.id, new.text);
END; 