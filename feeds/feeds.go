package feeds

import (
	"norsky/db"
	"norsky/models"
	"strconv"

	"norsky/config"

	log "github.com/sirupsen/logrus"
)

type Algorithm func(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error)

// Reuse genericAlgo for all algorithms
func genericAlgo(reader *db.Reader, cursor string, limit int, languages []string, keywords []string, excludeReplies bool) (*models.FeedResponse, error) {
	postId := safeParseCursor(cursor)

	posts, err := reader.GetFeed(languages, keywords, limit+1, postId, excludeReplies)
	if err != nil {
		log.Error("Error getting feed", err)
		return nil, err
	}

	if posts == nil {
		posts = []models.FeedPost{}
	}

	var nextCursor *string
	nextCursor = nil

	if len(posts) > 0 {
		if len(posts) > limit {
			posts = posts[:len(posts)-1]
		}

		parsed := strconv.FormatInt(posts[len(posts)-1].Id, 10)
		nextCursor = &parsed
	}

	return &models.FeedResponse{
		Feed:   posts,
		Cursor: nextCursor,
	}, nil
}

// safeParseCursor parses the cursor string and returns the unix time and post id
// If the cursor is invalid, it returns 0, 0, nil
func safeParseCursor(cursor string) int64 {
	id, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

type Feed struct {
	Id             string
	DisplayName    string
	Description    string
	AvatarPath     string
	Languages      []string
	Keywords       []string
	ExcludeReplies bool
	Algorithm      Algorithm
}

// Create a new function to initialize feeds from config
func InitializeFeeds(cfg *config.Config) map[string]Feed {
	feeds := make(map[string]Feed)

	for _, feedCfg := range cfg.Feeds {
		feeds[feedCfg.ID] = Feed{
			Id:             feedCfg.ID,
			DisplayName:    feedCfg.DisplayName,
			Description:    feedCfg.Description,
			AvatarPath:     feedCfg.AvatarPath,
			Languages:      feedCfg.Languages,
			Keywords:       feedCfg.Keywords,
			ExcludeReplies: feedCfg.ExcludeReplies,
			Algorithm:      createAlgorithm(feedCfg.Languages, feedCfg.Keywords, feedCfg.ExcludeReplies),
		}
	}

	return feeds
}

// Helper function to create an algorithm based on languages
func createAlgorithm(languages []string, keywords []string, excludeReplies bool) Algorithm {
	return func(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error) {
		return genericAlgo(reader, cursor, limit, languages, keywords, excludeReplies)
	}
}
