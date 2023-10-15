package server

import (
	"bufio"
	"embed"
	"fmt"
	"net/http"
	"norsky/db"
	"norsky/feeds"
	"strconv"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

//go:embed dist/*
var dist embed.FS

type ServerConfig struct {

	// The hostname to use for the server
	Hostname string

	// The reader to use for reading posts
	Reader *db.Reader
}

// Returns a fiber.App instance to be used as an HTTP server for the norsky feed
func Server(config *ServerConfig) *fiber.App {

	app := fiber.New()

	app.Use(compress.New())

	// Setup CORS for localhost:3001
	app.Use(func(c *fiber.Ctx) error {
		corsConfig := cors.Config{
			AllowOrigins:     "*",
			AllowCredentials: true,
		}
		return cors.New(corsConfig)(c)
	})

	// Serve the assets

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

	// Setup cache
	app.Use(cache.New(cache.Config{
		Next: func(c *fiber.Ctx) bool {
			// Only cache dashboard requests
			if strings.HasPrefix(c.Path(), "/dashboard") {
				log.WithFields(log.Fields{
					"path": c.Path(),
				}).Info("Cache request")
				return false
			}
			return true
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			// Get URL with query string to use as cache key
			url := c.Request().URI().String()
			// Include the query parameters in the cache key
			return url
		},
	}))

	// Well known
	app.Get("/.well-known/did.json", func(c *fiber.Ctx) error {
		// Return the DID document, using regular map[string]interface{} for now

		return c.JSON(map[string]interface{}{
			"@context": []string{"https://www.w3.org/ns/did/v1"},
			"id":       "did:web:" + config.Hostname,
			"service": []map[string]interface{}{
				{
					"id":              "#bsky_fg",
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

	app.Get("/dashboard/posts-per-hour", func(c *fiber.Ctx) error {
		// Get the feed query parameters and parse the limit
		lang := c.Query("lang", "")

		// Get the posts per hour
		postsPerHour, err := config.Reader.GetPostCountPerHour(lang)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error getting posts per hour")

			return c.Status(500).SendString("Error getting posts per hour")
		}

		log.WithFields(log.Fields{
			"lang":  lang,
			"count": len(postsPerHour),
		}).Info("Get posts per hour")

		return c.Status(200).JSON(postsPerHour)
	})

	app.Get("/dashboard/feed", func(c *fiber.Ctx) error {
		// Make a server sent event stream of the feed
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			var i int
			for {
				i++
				msg := fmt.Sprintf("%d", i)
				fmt.Fprintf(w, "data: Message: %s\n\n", msg)
				fmt.Println(msg)

				err := w.Flush()
				if err != nil {
					// Refreshing page in web browser will establish a new
					// SSE connection, but only (the last) one is alive, so
					// dead connections must be closed here.
					fmt.Printf("Error while flushing: %v. Closing http connection.\n", err)

					break
				}
				time.Sleep(2 * time.Second)
			}
		}))

		return nil

	})

	// Serve the Solid dashboard
	app.Use("/", filesystem.New(filesystem.Config{
		Browse:     false,
		Index:      "index.html",
		Root:       http.FS(dist),
		PathPrefix: "/dist",
	}))

	return app
}
