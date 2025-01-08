package db

import (
	"database/sql"
	"fmt"
	"norsky/models"
	"strconv"
	"time"

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

func (reader *Reader) GetFeed(langs []string, limit int, postId int64) ([]models.FeedPost, error) {
	sb := sqlbuilder.NewSelectBuilder()
	sb.Select("DISTINCT posts.id", "posts.uri").From("posts")

	if postId != 0 {
		sb.Where(sb.LessThan("posts.id", postId))
	}

	if len(langs) > 0 {
		sb.Join("post_languages", "posts.id = post_languages.post_id")
		// Build OR conditions for each language
		conditions := make([]string, len(langs))
		for i, lang := range langs {
			conditions[i] = sb.Equal("post_languages.language", lang)
		}
		sb.Where(sb.Or(conditions...))
	}

	sb.OrderBy("posts.id").Desc()
	sb.Limit(limit)

	sql, args := sb.BuildWithFlavor(sqlbuilder.Flavor(sqlbuilder.SQLite))

	rows, err := reader.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var posts []models.FeedPost
	for rows.Next() {
		var post models.FeedPost
		if err := rows.Scan(&post.Id, &post.Uri); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		posts = append(posts, post)
	}

	return posts, nil
}

// Returns the number of posts for each hour of the day from the last 24 hours
func (reader *Reader) GetPostCountPerTime(lang string, timeAgg string) ([]models.PostsAggregatedByTime, error) {
	var sqlFormat string
	var timeParse func(string) (time.Time, error)

	switch timeAgg {
	case "hour":
		sqlFormat = `STRFTIME('%Y-%m-%d-%H', created_at, 'unixepoch')`
		timeParse = func(str string) (time.Time, error) {
			return time.Parse("2006-01-02-15", str)
		}
	case "day":
		sqlFormat = `STRFTIME('%Y-%m-%d', created_at, 'unixepoch')`
		timeParse = func(str string) (time.Time, error) {
			return time.Parse("2006-01-02", str)
		}
	case "week":
		sqlFormat = "STRFTIME('%Y-%W', created_at, 'unixepoch')"
		timeParse = func(str string) (time.Time, error) {
			// Manually parse year and week number as separate integers
			year, err := time.Parse("2006", str[:4])
			if err != nil {
				return time.Time{}, err
			}
			week, err := strconv.ParseInt(str[5:], 10, 64)
			if err != nil {
				return time.Time{}, err
			}

			_, weekOffset := year.ISOWeek()
			weekOffset = weekOffset - 1
			firstDay := year.AddDate(0, 0, -int(year.Weekday())+weekOffset*7)

			// Add the number of weeks to the first day of the week
			return firstDay.AddDate(0, 0, int(week)*7), nil
		}
	default:
		sqlFormat = `STRFTIME('%Y-%m-%d-%H', created_at, 'unixepoch')`
		timeParse = func(str string) (time.Time, error) {
			return time.Parse("2006-01-02-15", str)
		}
	}

	sb := sqlbuilder.NewSelectBuilder()
	sb.Select(sqlFormat, "count(*) as count").From("posts").GroupBy(sqlFormat)
	if lang != "" {
		sb.Join("post_languages", "posts.id = post_languages.post_id")
		sb.Where(sb.Equal("language", lang))
	}
	sb.GroupBy(sqlFormat)
	sb.OrderBy("created_at").Asc()

	sql, args := sb.BuildWithFlavor(sqlbuilder.Flavor(sqlbuilder.SQLite))

	rows, err := reader.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postCounts []models.PostsAggregatedByTime

	for rows.Next() {
		var sqlTime string
		var postCount models.PostsAggregatedByTime

		if err := rows.Scan(&sqlTime, &postCount.Count); err != nil {
			continue // Skip this row
		}
		// Parse from YYYY-MM-DD-HH
		postTime, error := timeParse(sqlTime)

		if error == nil {
			postCount.Time = postTime
		}
		postCounts = append(postCounts, postCount)
	}

	return postCounts, nil
}

func (reader *Reader) GetSequence() (int64, error) {
	// Get sequence number
	selectSeq := sqlbuilder.NewSelectBuilder()
	sql, args := selectSeq.Select("seq").From("sequence").Where(selectSeq.Equal("id", 0)).Build()

	var seq int64
	err := reader.db.QueryRow(sql, args...).Scan(&seq)
	if err != nil {
		return 0, err
	}

	return seq, nil
}
