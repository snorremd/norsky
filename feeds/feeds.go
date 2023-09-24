package feeds

import (
	"norsky/db"
	"norsky/models"
	"strconv"

	log "github.com/sirupsen/logrus"
)

type Algorithm func(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error)

func bokmaal(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error) {
	return genericAlgo(reader, cursor, limit, "nb")
}

func nynorsk(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error) {
	return genericAlgo(reader, cursor, limit, "nn")
}

func samisk(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error) {
	return genericAlgo(reader, cursor, limit, "smi")
}

func all(reader *db.Reader, cursor string, limit int) (*models.FeedResponse, error) {
	return genericAlgo(reader, cursor, limit, "")
}

// Reuse genericAlgo for all algorithms
func genericAlgo(reader *db.Reader, cursor string, limit int, lang string) (*models.FeedResponse, error) {
	postId := safeParseCursor(cursor)

	posts, err := reader.GetFeed(lang, limit+1, postId)
	if err != nil {
		log.Error("Error getting feed", err)
		return nil, err
	}

	if posts == nil {
		posts = []models.Post{}
	}

	var nextCursor *string
	nextCursor = nil

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
	Id          string
	DisplayName string
	Description string
	Algorithm   Algorithm
}

// List of available feeds
var Feeds = map[string]Feed{
	"bokmaal": {
		Id:          "bokmaal",
		DisplayName: "Bokmål",
		Description: "A feed of Bluesky posts written in Norwegian bokmål",
		Algorithm:   bokmaal,
	},
	"nynorsk": {
		Id:          "nynorsk",
		DisplayName: "Nynorsk",
		Description: "A feed of Bluesky posts written in Norwegian nynorsk",
		Algorithm:   nynorsk,
	},
	"sami": {
		Id:          "sami",
		DisplayName: "Sami",
		Description: "A feed of Bluesky posts written in Sami",
		Algorithm:   samisk,
	},
	"all": {
		Id:          "all",
		DisplayName: "Norwegian languages",
		Description: "A feed of Bluesky posts written in Norwegian bokmål, nynorsk and sami",
		Algorithm:   all,
	},
}
