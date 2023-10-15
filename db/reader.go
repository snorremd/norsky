package db

import (
	"database/sql"
	"norsky/models"
	"strings"
	"time"

	sqlbuilder "github.com/huandu/go-sqlbuilder"
	log "github.com/sirupsen/logrus"
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

// Returns the number of posts for each hour of the day from the last 24 hours
func (reader *Reader) GetPostCountPerHour(lang string) ([]models.PostsAggregatedByTime, error) {
	timeAgg := "STRFTIME('%Y-%m-%d-%H', created_at, 'unixepoch')"

	sb := sqlbuilder.NewSelectBuilder()
	sb.Select(timeAgg, "count(*) as count").From("posts").GroupBy(timeAgg)
	if lang != "" {
		sb.Join("post_languages", "posts.id = post_languages.post_id")
		sb.Where(sb.Equal("language", lang))
	}
	sb.GroupBy("strftime('%H', datetime(created_at, 'unixepoch'))")
	sb.OrderBy("created_at").Asc()

	sql, args := sb.BuildWithFlavor(sqlbuilder.Flavor(sqlbuilder.SQLite))

	log.WithFields(log.Fields{
		"sql":  sql,
		"args": args,
	}).Info("Get posts per hour")

	rows, err := reader.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postCounts []models.PostsAggregatedByTime

	for rows.Next() {
		var hour string
		var postCount models.PostsAggregatedByTime

		if err := rows.Scan(&hour, &postCount.Count); err != nil {
			continue // Skip this row
		}
		// Parse from YYYY-MM-DD-HH
		postTime, error := time.Parse("2006-01-02-15", hour)

		if error == nil {
			postCount.Time = postTime
		}
		postCounts = append(postCounts, postCount)
	}

	return postCounts, nil
}
