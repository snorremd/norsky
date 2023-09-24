package db

import (
	"database/sql"
	"norsky/models"
	"strings"

	sqlbuilder "github.com/huandu/go-sqlbuilder"
)

type Reader struct {
	db *sql.DB
}

func NewReader(database string) *Reader {
	// Open in read-only mode
	db, err := sql.Open("sqlite", database+"?mode=ro")
	if err != nil {
		panic("failed to connect database")
	}
	return &Reader{
		db: db,
	}
}

func (reader *Reader) GetFeed(lang string, limit int, postId int64) ([]models.Post, error) {

	// Return next limit posts after cursor, ordered by created_at and uri

	sb := sqlbuilder.NewSelectBuilder()
	sb.Select("id", "uri", "created_at", "group_concat(language)").From("posts")
	if postId != 0 {
		sb.Where(
			sb.LessThan("id", postId),
		)
	}
	if lang != "" {
		sb.Where(sb.Equal("language", lang))
	}
	sb.Join("post_languages", "posts.id = post_languages.post_id")
	sb.GroupBy("posts.id")
	sb.Limit(limit).OrderBy("id").Desc()

	sql, args := sb.BuildWithFlavor(sqlbuilder.Flavor(sqlbuilder.SQLite))

	rows, err := reader.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Scan rows, collapse languages into single post
	var posts []models.Post

	for rows.Next() {
		var post models.Post
		var langs string
		// Scan row and
		if err := rows.Scan(&post.Id, &post.Uri, &post.CreatedAt, &langs); err != nil {
			return nil, err
		}

		// Split languages into a slice and add to the post model
		post.Languages = strings.Split(langs, ",")
		posts = append(posts, post)
	}

	return posts, nil
}
