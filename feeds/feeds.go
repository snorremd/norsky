package feeds

import (
	"norsky/db"
	"norsky/models"
	"norsky/query"
	"strconv"

	"norsky/config"

	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

// InitializeFeeds creates feeds from configuration
func InitializeFeeds(cfg *config.TomlConfig, db *db.DB) (map[string]*Feed, error) {
	feeds := make(map[string]*Feed)

	for _, feedConfig := range cfg.Feeds {
		builder := NewFeedQueryBuilder()

		// Add filters
		for _, filterConfig := range feedConfig.Filters {
			filter, err := createFilterStrategy(filterConfig, cfg.Keywords)
			if err != nil {
				return nil, fmt.Errorf("error creating filter for feed %s: %w", feedConfig.Id, err)
			}
			builder.AddFilter(filter)
		}

		// Add scoring layers
		for _, scoringConfig := range feedConfig.Scoring {
			strategy, err := createScoringStrategy(scoringConfig, cfg.Keywords)
			if err != nil {
				return nil, fmt.Errorf("error creating scoring for feed %s: %w", feedConfig.Id, err)
			}
			builder.AddScoringLayer(strategy, scoringConfig.Weight)
		}

		feeds[feedConfig.Id] = &Feed{
			ID:          feedConfig.Id,
			DisplayName: feedConfig.DisplayName,
			Description: feedConfig.Description,
			AvatarPath:  feedConfig.AvatarPath,
			DB:          db,
			builder:     builder,
		}
	}

	return feeds, nil
}

// createFilterStrategy creates a Filter from config
func createFilterStrategy(config config.TomlFilter, keywords config.TomlKeywords) (query.FilterStrategy, error) {
	switch config.Type {
	case "language":
		return &LanguageFilter{Languages: config.Languages}, nil
	case "keyword":
		// Combine keywords from referenced lists
		var includeKeywords, excludeKeywords []string
		for _, ref := range config.Include {
			if kw, ok := keywords[ref]; ok {
				includeKeywords = append(includeKeywords, kw...)
			}
		}
		for _, ref := range config.Exclude {
			if kw, ok := keywords[ref]; ok {
				excludeKeywords = append(excludeKeywords, kw...)
			}
		}
		return &KeywordFilter{
			IncludeKeywords: strings.Join(includeKeywords, " OR "),
			ExcludeKeywords: strings.Join(excludeKeywords, " OR "),
		}, nil
	case "exclude_replies":
		return &ExcludeRepliesFilter{}, nil
	default:
		return nil, fmt.Errorf("unknown filter type: %s", config.Type)
	}
}

// createScoringStrategy creates a ScoringStrategy from config
func createScoringStrategy(config config.TomlScoring, keywords config.TomlKeywords) (query.ScoringStrategy, error) {
	switch config.Type {
	case "time_decay":
		return &TimeDecayScoring{}, nil
	case "keyword":
		if kw, ok := keywords[config.Keywords]; ok {
			return &KeywordScoring{Keywords: strings.Join(kw, " OR ")}, nil
		}
		return nil, fmt.Errorf("keyword list not found: %s", config.Keywords)
	case "author":
		return &AuthorScoring{Authors: config.Authors}, nil
	default:
		return nil, fmt.Errorf("unknown scoring type: %s", config.Type)
	}
}

// GetFeedPosts retrieves posts for a feed with pagination
func (f *Feed) GetFeedPosts(cursor string, limit int) (*models.FeedResponse, error) {
	postId := safeParseCursor(cursor)
	posts, err := f.DB.GetFeedPosts(f.builder, limit+1, postId)
	if err != nil {
		log.Error("Error getting feed posts", err)
		return nil, err
	}

	return createPaginatedResponse(posts, limit)
}

// Helper functions for pagination

func safeParseCursor(cursor string) int64 {
	id, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func createPaginatedResponse(posts []models.FeedPost, limit int) (*models.FeedResponse, error) {
	if posts == nil {
		posts = []models.FeedPost{}
	}

	var nextCursor *string
	if len(posts) > limit {
		posts = posts[:len(posts)-1]
		parsed := strconv.FormatInt(posts[len(posts)-1].Id, 10)
		nextCursor = &parsed
	}

	return &models.FeedResponse{
		Feed:   posts,
		Cursor: nextCursor,
	}, nil
}
