/* Create a Posts table for the Bluesky feed post data:
 * - cid: string
 * - created_at: timestamp
 */

CREATE TABLE posts (
  uri TEXT PRIMARY KEY,
  created_at TIMESTAMP NOT NULL
);

