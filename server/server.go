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

	// Add feeds to config
	Feeds map[string]feeds.Feed
}

// Make it sync
type Broadcaster struct {
	sync.RWMutex
	createPostClients map[string]chan models.CreatePostEvent
	statisticsClients map[string]chan models.StatisticsEvent
}

// Constructor
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		createPostClients: make(map[string]chan models.CreatePostEvent, 10000),
		statisticsClients: make(map[string]chan models.StatisticsEvent, 10000),
	}
}

func (b *Broadcaster) BroadcastCreatePost(post models.CreatePostEvent) {
	for id, client := range b.createPostClients {
		select {
		case client <- post: // Non-blocking send
		default:
			log.Warnf("Client channel full, skipping stats for client: %v", id)
		}
	}
}

func (b *Broadcaster) BroadcastStatistics(stats models.StatisticsEvent) {
	b.RLock() // Assuming you have a mutex for client safety
	defer b.RUnlock()

	for id, client := range b.statisticsClients {
		select {
		case client <- stats: // Non-blocking send
		default:
			log.Warnf("Client channel full, skipping stats for client: %v", id)
		}
	}
}

// Function to add a client to the broadcaster
func (b *Broadcaster) AddClient(key string, createPostClient chan models.CreatePostEvent, statisticsClient chan models.StatisticsEvent) {
	b.Lock()
	defer b.Unlock()
	b.createPostClients[key] = createPostClient
	b.statisticsClients[key] = statisticsClient
	log.WithFields(log.Fields{
		"key":   key,
		"count": len(b.createPostClients),
	}).Info("Adding client to broadcaster")
}

// Function to remove a client from the broadcaster
func (b *Broadcaster) RemoveClient(key string) {
	b.Lock()
	defer b.Unlock()

	// Remove from createPostClients
	if client, ok := b.createPostClients[key]; ok { // Check if the client exists
		close(client)                    // Safely close the channel
		delete(b.createPostClients, key) // Remove from the map
	}

	// Remove from statisticsClients
	if client, ok := b.statisticsClients[key]; ok { // Check if the client exists
		close(client)                    // Safely close the channel
		delete(b.statisticsClients, key) // Remove from the map
	}

	log.WithFields(log.Fields{
		"key":   key,
		"count": len(b.createPostClients),
	}).Info("Removed client from broadcaster")
}

func (b *Broadcaster) Shutdown() {
	log.Info("Shutting down broadcaster")
	b.Lock()
	defer b.Unlock()
	for _, client := range b.createPostClients {
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

		// Unique client key
		key := uuid.New().String()
		sseCreatePostChannel := make(chan models.CreatePostEvent, 10) // Buffered channel
		sseStatisticsChannel := make(chan models.StatisticsEvent, 10)
		aliveChan := time.NewTicker(5 * time.Second)

		defer aliveChan.Stop()

		// Register the client
		bc.AddClient(key, sseCreatePostChannel, sseStatisticsChannel)

		// Cleanup function
		cleanup := func() {
			log.Infof("Cleaning up SSE stream for client: %s", key)
			bc.RemoveClient(key)
		}

		// Use StreamWriter to manage SSE streaming
		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			defer cleanup()

			// Send initial event with client key
			fmt.Fprintf(w, "event: init\ndata: %s\n\n", key)
			if err := w.Flush(); err != nil {
				log.Errorf("Failed to send init event: %v", err)
				return
			}

			// Start streaming loop
			for {
				select {
				case <-aliveChan.C:
					// Send keep-alive pings
					if _, err := fmt.Fprintf(w, "event: ping\ndata: \n\n"); err != nil {
						log.Warnf("Failed to send ping to client %s: %v", key, err)
						return
					}
					if err := w.Flush(); err != nil {
						log.Warnf("Failed to flush ping for client %s: %v", key, err)
						return
					}

				case post, ok := <-sseCreatePostChannel:
					if !ok {
						log.Warnf("CreatePostChannel closed for client %s", key)
						return
					}
					jsonPost, err := json.Marshal(post.Post)
					if err != nil {
						log.Errorf("Error marshalling post for client %s: %v", key, err)
						continue
					}
					if _, err := fmt.Fprintf(w, "event: create-post\ndata: %s\n\n", jsonPost); err != nil {
						log.Warnf("Failed to send create-post event to client %s: %v", key, err)
						return
					}
					if err := w.Flush(); err != nil {
						log.Warnf("Failed to flush create-post event for client %s: %v", key, err)
						return
					}

				case stats, ok := <-sseStatisticsChannel:
					if !ok {
						log.Warnf("StatisticsChannel closed for client %s", key)
						return
					}
					jsonStats, err := json.Marshal(stats)
					if err != nil {
						log.Errorf("Error marshalling stats for client %s: %v", key, err)
						continue
					}
					if _, err := fmt.Fprintf(w, "event: statistics\ndata: %s\n\n", jsonStats); err != nil {
						log.Warnf("Failed to send statistics event to client %s: %v", key, err)
						return
					}
					if err := w.Flush(); err != nil {
						log.Warnf("Failed to flush statistics event for client %s: %v", key, err)
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
