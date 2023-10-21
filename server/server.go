package server

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"norsky/db"
	"norsky/feeds"
	"norsky/models"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/google/uuid"
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

	// Broadcast channels to pass posts to SSE clients
	Broadcaster *Broadcaster
}

// Make it sync
type Broadcaster struct {
	sync.Mutex
	clients map[string]chan models.CreatePostEvent
}

// Constructor
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[string]chan models.CreatePostEvent),
	}
}

func (b *Broadcaster) Broadcast(post models.CreatePostEvent) {
	log.WithFields(log.Fields{
		"clients": len(b.clients),
	}).Info("Broadcasting post to SSE clients")

	b.Lock()
	defer b.Unlock()
	for _, client := range b.clients {
		client <- post
	}
}

// Function to add a client to the broadcaster
func (b *Broadcaster) AddClient(key string, client chan models.CreatePostEvent) {
	b.Lock()
	defer b.Unlock()
	b.clients[key] = client
	log.WithFields(log.Fields{
		"key":   key,
		"count": len(b.clients),
	}).Info("Adding client to broadcaster")
}

// Function to remove a client from the broadcaster
func (b *Broadcaster) RemoveClient(key string) {
	b.Lock()
	defer b.Unlock()
	if _, ok := b.clients[key]; !ok {
		close(b.clients[key])
		delete(b.clients, key)
	}

	log.WithFields(log.Fields{
		"key":   key,
		"count": len(b.clients),
	}).Info("Removed client from broadcaster")
}

func (b *Broadcaster) Shutdown() {
	log.Info("Shutting down broadcaster")
	b.Lock()
	defer b.Unlock()
	for _, client := range b.clients {
		close(client)
	}
}

// Returns a fiber.App instance to be used as an HTTP server for the norsky feed
func Server(config *ServerConfig) *fiber.App {

	bc := config.Broadcaster

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
			AllowOrigins:     "*",
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

	app.Delete("/dashboard/feed/sse", func(c *fiber.Ctx) error {
		// Get the feed query parameters and parse the limit
		key := c.Query("key", "")
		bc.RemoveClient(key)
		return c.Status(200).SendString("OK")
	})

	app.Get("/dashboard/feed/sse", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			// Register the channel to write posts to so server can write to it
			key := uuid.New().String()
			sseChannel := make(chan models.CreatePostEvent)
			aliveChan := time.NewTicker(5 * time.Second)
			bc.AddClient(key, sseChannel)

			// A function to cleanup
			cleanup := func() {
				log.Info("Cleaning up SSE stream ", key)
				// Remove sseChannel from the list of channels
				bc.RemoveClient(key)
				aliveChan.Stop()
			}

			// Send the SSE key first so the client can close the connection when it wants to
			fmt.Fprintf(w, "data: %s\n\n", key)
			w.Flush()

			for {
				select {
				case <-aliveChan.C:
					err := w.Flush()
					if err != nil {
						cleanup()
						return
					}
				case post := <-sseChannel:
					jsonPost, jsonErr := json.Marshal(post.Post)

					if jsonErr != nil {
						log.Error("Error marshalling post", jsonErr)
						break // Break out of the select statement as we have no post to write
					}

					fmt.Fprintf(w, "data: %s\n\n", jsonPost)
					err := w.Flush()
					if err != nil {
						cleanup()
						return
					}
				}
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
