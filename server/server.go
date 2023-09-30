package server

import (
	_ "embed"
	"fmt"
	"norsky/db"
	"norsky/feeds"
	"strconv"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	log "github.com/sirupsen/logrus"
)

//go:embed assets/favicon.ico
var faviconFile []byte

type ServerConfig struct {

	// The hostname to use for the server
	Hostname string

	// The reader to use for reading posts
	Reader *db.Reader
}

// Returns a fiber.App instance to be used as an HTTP server for the norsky feed
func Server(config *ServerConfig) *fiber.App {

	app := fiber.New()

	// app.Use(cache.New())

	// Middleware to track the latency of each request
	app.Use(func(c *fiber.Ctx) error {
		// start timer
		start := time.Now()

		// next routes
		err := c.Next()

		// stop timer
		stop := time.Now()

		// Diff
		log.WithFields(log.Fields{
			"method":  c.Method(),
			"route":   c.Route().Path,
			"latency": stop.Sub(start),
		}).Info("Request")
		return err
	})

	app.Use(favicon.New(favicon.Config{
		Data: faviconFile,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("This is the Norsky feed generator for listing Norwegian posts on Bluesky.")
	})

	// Well known
	app.Get("/.well-known/did.json", func(c *fiber.Ctx) error {
		// Return the DID document, using regular map[string]interface{} for now

		return c.JSON(map[string]interface{}{
			"@context": []string{"https://www.w3.org/ns/did/v1"},
			"id":       "did:web:" + config.Hostname,
			"service": []map[string]interface{}{
				{
					"id":              "#norsky",
					"type":            "BskyFeedGenerator",
					"serviceEndpoint": "https://" + config.Hostname,
				},
			},
		})
	})

	// Endpoint to describe the feed
	app.Get("/xrpc/app.bsky.feed.describeFeedGenerator", func(c *fiber.Ctx) error {

		// Map over algorithms.Algorithms keys and return a bsky.FeedDescribeFeedGenerator_Output
		generatorFeeds := []*bsky.FeedDescribeFeedGenerator_Feed{}
		for algorithm := range feeds.Feeds {
			generatorFeeds = append(generatorFeeds, &bsky.FeedDescribeFeedGenerator_Feed{
				Uri: "at://did:web:" + config.Hostname + "/app.bsky.feed.generator/" + algorithm,
			})
		}

		// Return the list of feeds
		return c.JSON(bsky.FeedDescribeFeedGenerator_Output{
			Did:   "did:web:" + config.Hostname,
			Feeds: generatorFeeds,
		})
	})

	app.Get("/xrpc/app.bsky.feed.getFeedSkeleton", func(c *fiber.Ctx) error {
		// Get the feed query parameters and parse the limit
		feed := c.Query("feed", "at://did:web:"+config.Hostname+"/app.bsky.feed.generator/all")
		cursor := c.Query("cursor", "")
		limit, err := strconv.ParseInt(c.Query("limit", "20"), 0, 32)
		if err != nil || limit < 1 || limit > 100 {
			limit = 20
		}

		// Parse the feed URI
		uri, err := syntax.ParseATURI(feed)
		if err != nil {
			fmt.Println("Error parsing URI", err)
			return c.Status(400).SendString("Invalid feed URI")
		}

		feedName := uri.RecordKey().String()

		log.WithFields(log.Fields{
			"feed":   feedName,
			"cursor": cursor,
			"limit":  limit,
		}).Info("Generate feed skeleton with parameters")

		if feed, ok := feeds.Feeds[feedName]; ok {
			// Call the algorithm
			posts, err := feed.Algorithm(config.Reader, cursor, int(limit))
			if err != nil {
				fmt.Println("Error calling algorithm", err)
				return c.Status(500).SendString("Error calling algorithm")
			}
			return c.JSON(posts)
		}

		return c.Status(400).SendString("Invalid feed")

	})

	return app
}
