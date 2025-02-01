package db

import (
	"context"
	"database/sql"
	"fmt"
	"norsky/models"
	"strings"
	"time"

	sqlbuilder "github.com/huandu/go-sqlbuilder"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// DB handles all database operations with a shared connection pool
type DB struct {
	db *sql.DB
}

// Add new function to build connection string
func buildConnectionString(host string, port int, user, password, dbname string) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)
}

// Update NewDB to accept individual parameters
func NewDB(host string, port int, user, password, dbname string) *DB {
	connString := buildConnectionString(host, port, user, password, dbname)
	db, err := sql.Open("postgres", connString)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}

	// Set connection pool settings
	db.SetMaxOpenConns(20)           // Allow multiple concurrent operations
	db.SetMaxIdleConns(10)           // Keep some connections ready
	db.SetConnMaxLifetime(time.Hour) // Recreate connections after an hour
	db.SetConnMaxIdleTime(time.Hour) // Close idle connections after an hour

	return &DB{db: db}
}

// Write operations

func (db *DB) CreatePost(ctx context.Context, post models.Post) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	log.WithFields(log.Fields{
		"uri":        post.Uri,
		"parent_uri": post.ParentUri,
		"languages":  post.Languages,
		"created_at": time.Unix(post.CreatedAt, 0).Format(time.RFC3339),
		"lagSeconds": time.Since(time.Unix(post.CreatedAt, 0)).Seconds(),
	}).Info("Creating post")

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO posts (uri, created_at, indexed_at, text, parent_uri, languages)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (uri) DO UPDATE SET
			indexed_at = $3,
			text = $4,
			parent_uri = $5,
			languages = $6`,
		post.Uri,
		time.Unix(post.CreatedAt, 0),
		time.Now(),
		post.Text,
		post.ParentUri,
		pq.Array(post.Languages),
	)
	if err != nil {
		return fmt.Errorf("insert error: %w", err)
	}

	return nil
}

func (db *DB) DeletePost(ctx context.Context, post models.Post) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	log.Info("Deleting post")
	_, err := db.db.ExecContext(ctx, "DELETE FROM posts WHERE uri = $1", post.Uri)
	if err != nil {
		return fmt.Errorf("delete error: %w", err)
	}
	return nil
}

// Read operations

func (db *DB) GetFeed(langs []string, keywords []string, excludeKeywords []string, limit int, postId int64, excludeReplies bool) ([]models.FeedPost, error) {
	log.WithFields(log.Fields{
		"langs":           langs,
		"keywords":        keywords,
		"excludeKeywords": excludeKeywords,
		"limit":           limit,
		"postId":          postId,
		"excludeReplies":  excludeReplies,
	}).Info("Getting feed")

	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("DISTINCT posts.id", "posts.uri").From("posts")

	if postId != 0 {
		sb.Where(sb.LessThan("posts.id", postId))
	}

	if excludeReplies {
		sb.Where(sb.IsNull("posts.parent_uri"))
	}

	if len(langs) > 0 {
		sb.Where(fmt.Sprintf("languages && %s", sb.Args.Add(pq.Array(langs))))
	}

	// Handle keywords and exclude keywords in a single query
	if len(keywords) > 0 || len(excludeKeywords) > 0 {
		var includeTerms []string
		var excludeTerms []string

		// Process include terms
		for _, query := range keywords {
			if strings.TrimSpace(query) == "" {
				continue
			}
			query = strings.ToLower(query)
			hasWildcard := strings.HasSuffix(query, "*")
			if hasWildcard {
				query = strings.TrimSuffix(query, "*")
			}
			if strings.Contains(query, " ") {
				query = fmt.Sprintf(`"%s"`, query)
			}
			if hasWildcard {
				query = query + "*"
			}
			includeTerms = append(includeTerms, query)
		}

		// Process exclude terms
		for _, query := range excludeKeywords {
			if strings.TrimSpace(query) == "" {
				continue
			}
			query = strings.ToLower(query)
			hasWildcard := strings.HasSuffix(query, "*")
			if hasWildcard {
				query = strings.TrimSuffix(query, "*")
			}
			if strings.Contains(query, " ") {
				query = fmt.Sprintf(`"%s"`, query)
			}
			if hasWildcard {
				query = query + "*"
			}
			excludeTerms = append(excludeTerms, query)
		}

		var query string

		if len(includeTerms) > 0 && len(excludeTerms) > 0 {
			query = "(" + strings.Join(includeTerms, " OR ") + " ) AND NOT (" + strings.Join(excludeTerms, " OR ") + " )"
		} else if len(includeTerms) > 0 {
			query = "(" + strings.Join(includeTerms, " OR ") + " )"
		} else if len(excludeTerms) > 0 {
			query = "NOT (" + strings.Join(excludeTerms, " OR ") + " )"
		}

		if query != "" {
			sb.Where(fmt.Sprintf("ts_vector @@ websearch_to_tsquery('simple', %s)",
				sb.Args.Add(query)))
		}
	}

	sb.OrderBy("posts.id").Desc()
	sb.Limit(limit)

	sql, args := sb.Build()
	log.WithFields(log.Fields{
		"sql":  sql,
		"args": args,
	}).Info("Generated SQL query")

	rows, err := db.db.Query(sql, args...)
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

func (db *DB) GetPostCountPerTime(lang string, timeAgg string) ([]models.PostsAggregatedByTime, error) {
	var sqlFormat string
	var timeParse func(string) (time.Time, error)

	switch timeAgg {
	case "hour":
		sqlFormat = "date_trunc('hour', created_at)"
	case "day":
		sqlFormat = "date_trunc('day', created_at)"
	case "week":
		sqlFormat = "date_trunc('week', created_at)"
	default:
		sqlFormat = "date_trunc('hour', created_at)"
	}

	timeParse = func(str string) (time.Time, error) {
		return time.Parse(time.RFC3339, str)
	}

	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select(sqlFormat, "count(*) as count").From("posts")

	if lang != "" {
		sb.Where(fmt.Sprintf("languages && %s", sb.Args.Add(pq.Array([]string{lang}))))
	}

	sb.GroupBy(sqlFormat)
	sb.OrderBy(sqlFormat).Asc()

	sql, args := sb.Build()
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postCounts []models.PostsAggregatedByTime
	for rows.Next() {
		var sqlTime string
		var postCount models.PostsAggregatedByTime

		if err := rows.Scan(&sqlTime, &postCount.Count); err != nil {
			continue
		}

		postTime, err := timeParse(sqlTime)
		if err == nil {
			postCount.Time = postTime
		}
		postCounts = append(postCounts, postCount)
	}

	return postCounts, nil
}

func (db *DB) GetLatestPostTimestamp(ctx context.Context) (time.Time, error) {
	var timestamp time.Time
	err := db.db.QueryRowContext(ctx, "SELECT created_at FROM posts ORDER BY created_at DESC LIMIT 1").Scan(&timestamp)
	if err == sql.ErrNoRows {
		return time.Time{}, nil // Return zero time if no posts
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("query error: %w", err)
	}
	return timestamp, nil
}
