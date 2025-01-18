package server

import (
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
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

//go:embed dist/*
var dist embed.FS

type ServerConfig struct {

	// The hostname to use for the server
	Hostname string

	// The reader to use for reading posts
	Reader *db.Reader

	// Add feeds to config
	Feeds map[string]feeds.Feed
}

var (
	// Define custom metrics
	customPostsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "norsky_posts_processed_total",
			Help: "Total number of posts processed by language",
		},
		[]string{"language"},
	)
)

func init() {
	// Register custom metrics
	prometheus.MustRegister(customPostsProcessed)
}

// Returns a fiber.App instance to be used as an HTTP server for the norsky feed
func Server(config *ServerConfig) *fiber.App {

	app := fiber.New()

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

	app.Use(requestid.New(requestid.ConfigDefault))
	app.Use(compress.New())

	// Setup CORS for localhost:3001
	app.Use(func(c *fiber.Ctx) error {
		corsConfig := cors.Config{
			AllowOrigins:     "http://localhost:3001",
			AllowHeaders:     "Cache-Control",
			AllowCredentials: true,
		}
		return cors.New(corsConfig)(c)
	})

	// Serve the assets

	// Setup cache
	app.Use(cache.New(cache.Config{
		Next: func(c *fiber.Ctx) bool {

			if c.Method() != "GET" {
				return true
			}

			// If the pathname ends with /sse, don't cache
			if strings.HasSuffix(c.Path(), "/sse") {
				return true
			}

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
		generatorFeeds := []*bsky.FeedDescribeFeedGenerator_Feed{}
		for feedId := range config.Feeds {
			generatorFeeds = append(generatorFeeds, &bsky.FeedDescribeFeedGenerator_Feed{
				Uri: "at://did:web:" + config.Hostname + "/app.bsky.feed.generator/" + feedId,
			})
		}

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

		if feed, ok := config.Feeds[feedName]; ok {
			posts, err := feed.Algorithm(config.Reader, cursor, int(limit))
			if err != nil {
				fmt.Println("Error calling algorithm", err)
				return c.Status(500).SendString("Error calling algorithm")
			}
			return c.JSON(posts)
		}

		return c.Status(400).SendString("Invalid feed")
	})

	app.Get("/dashboard/posts-per-time", func(c *fiber.Ctx) error {
		// Get the feed query parameters and parse the limit
		lang := c.Query("lang", "")
		time := c.Query("time", "")

		if time == "" {
			time = "hour"
		}

		// check if time is hour, day or week
		if time != "hour" && time != "day" && time != "week" {
			return c.Status(400).SendString("Invalid time")
		}

		// Get posts per time
		postsPerTime, err := config.Reader.GetPostCountPerTime(lang, time)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Error getting posts per time")

			return c.Status(500).SendString("Error getting posts per time")
		}

		log.WithFields(log.Fields{
			"lang":  lang,
			"count": len(postsPerTime),
		}).Info("Get posts per time")

		return c.Status(200).JSON(postsPerTime)
	})

	// Serve the Solid dashboard
	app.Use("/", filesystem.New(filesystem.Config{
		Browse:     false,
		Index:      "index.html",
		Root:       http.FS(dist),
		PathPrefix: "/dist",
	}))

	// Add Prometheus metrics endpoint
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	return app
}
