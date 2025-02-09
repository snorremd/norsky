package feeds

import (
	"norsky/db"
	"norsky/models"
	"strconv"

	"norsky/config"

	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Algorithm func(db *db.DB, cursor string, limit int) (*models.FeedResponse, error)

// Add new types to store prepared queries
type PreparedQueries struct {
	IncludeQuery string
	ExcludeQuery string
}

// Reuse genericAlgo for all algorithms
func genericAlgo(db *db.DB, cursor string, limit int, languages []string, preparedKeywords string, preparedExcludeKeywords string, excludeReplies bool) (*models.FeedResponse, error) {
	postId := safeParseCursor(cursor)

	posts, err := db.GetFeed(languages, preparedKeywords, preparedExcludeKeywords, limit+1, postId, excludeReplies)
	if err != nil {
		log.Error("Error getting feed", err)
		return nil, err
	}

	if posts == nil {
		posts = []models.FeedPost{}
	}

	var nextCursor *string

	// Only set cursor if we have more results
	if len(posts) > limit {
		// Remove the extra post we fetched to check for more results
		posts = posts[:len(posts)-1]
		// Set the cursor to the last post's ID
		parsed := strconv.FormatInt(posts[len(posts)-1].Id, 10)
		nextCursor = &parsed
	}

	return &models.FeedResponse{
		Feed:   posts,
		Cursor: nextCursor, // Will be nil if no more results
	}, nil
}

// safeParseCursor parses the cursor string and returns the post id
// If the cursor is invalid, it returns 0
func safeParseCursor(cursor string) int64 {
	id, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

type Feed struct {
	Id                      string
	DisplayName             string
	Description             string
	AvatarPath              string
	Languages               []string
	Keywords                []string
	ExcludeKeywords         []string
	ExcludeReplies          bool
	Algorithm               Algorithm
	PreparedKeywords        string
	PreparedExcludeKeywords string
}

// Helper function to prepare keyword queries
func prepareKeywordQuery(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}

	var terms []string
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
		terms = append(terms, query)
	}

	if len(terms) == 0 {
		return ""
	}
	return strings.Join(terms, " OR ")
}

// Update InitializeFeeds to prepare queries
func InitializeFeeds(cfg *config.Config) map[string]Feed {
	feeds := make(map[string]Feed)

	for _, feedCfg := range cfg.Feeds {

		preparedKeywords := prepareKeywordQuery(feedCfg.Keywords)
		preparedExcludeKeywords := prepareKeywordQuery(feedCfg.ExcludeKeywords)

		feeds[feedCfg.ID] = Feed{
			Id:             feedCfg.ID,
			DisplayName:    feedCfg.DisplayName,
			Description:    feedCfg.Description,
			AvatarPath:     feedCfg.AvatarPath,
			Languages:      feedCfg.Languages,
			ExcludeReplies: feedCfg.ExcludeReplies,
			Algorithm:      createAlgorithm(feedCfg.Languages, preparedKeywords, preparedExcludeKeywords, feedCfg.ExcludeReplies),
		}
	}

	return feeds
}

// Update createAlgorithm to use prepared queries
func createAlgorithm(languages []string, preparedKeywords string, preparedExcludeKeywords string, excludeReplies bool) Algorithm {
	return func(db *db.DB, cursor string, limit int) (*models.FeedResponse, error) {
		return genericAlgo(db, cursor, limit, languages, preparedKeywords, preparedExcludeKeywords, excludeReplies)
	}
}
