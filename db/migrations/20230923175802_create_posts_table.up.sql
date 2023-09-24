/* Create a Posts table for the Bluesky feed post data:
 * - cid: string
 * - created_at: timestamp
 */

CREATE TABLE posts (
  id INTEGER PRIMARY KEY AUTOINCREMENT, -- We need an id to implement deterministic cursor based pagination
  uri TEXT, -- The URI of the post
  created_at INTEGER NOT NULL -- The time the post was created as a Unix timestamp
);

-- Table to hold the languages that a post is written in

-- Path: db/migrations/20230923175803_create_post_languages_table.up.sql
/* Create a PostLanguages table for the Bluesky feed post data:
 * - cid: string
 * - language: string
 */

CREATE TABLE post_languages (
  post_id INTEGER NOT NULL,
  language TEXT NOT NULL,
  PRIMARY KEY (post_id, language),
  FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
);