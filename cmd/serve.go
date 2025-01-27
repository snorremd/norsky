/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"norsky/config"
	"norsky/db"
	"norsky/feeds"
	"norsky/firehose"
	"norsky/models"
	"norsky/server"
	"sync/atomic"
	"time"

	"github.com/golang-migrate/migrate/v4"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type contextKey string

const cancelKey contextKey = "cancel"

func serveCmd() *cli.Command {

	return &cli.Command{
		Name:  "serve",
		Usage: "Serve the norsky feed",
		Description: `Starts the norsky feed HTTP server and firehose subscriber.

		Launches the HTTP server on the specified or default port and subscribes to the
		Bluesky firehose. All posts written in Norwegian bokmål, nynorsk and sami are
		written to the SQLite database and can be accessed via the HTTP API as specified
		in the Bluesky API specification.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "hostname",
				Aliases: []string{"n"},
				Usage:   "The hostname where the server is running",
				EnvVars: []string{"NORSKY_HOSTNAME"},
			},
			&cli.StringFlag{
				Name:    "host",
				Aliases: []string{"o"},
				Usage:   "Host of the server",
				EnvVars: []string{"NORSKY_HOST"},
				Value:   "0.0.0.0",
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Port of the server",
				EnvVars: []string{"NORSKY_PORT"},
				Value:   3000,
			},
			&cli.BoolFlag{
				Name:    "run-language-detection",
				Usage:   "Run language detection on all posts, even if they are not tagged with correct language",
				EnvVars: []string{"NORSKY_RUN_LANGUAGE_DETECTION"},
				Value:   false,
			},
			&cli.Float64Flag{
				Name:    "confidence-threshold",
				Usage:   "Minimum confidence threshold for language detection",
				EnvVars: []string{"NORSKY_CONFIDENCE_THRESHOLD"},
				Value:   0.6,
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config/feeds.toml",
				Usage:   "Path to feeds configuration file",
				EnvVars: []string{"NORSKY_CONFIG"},
			},
			&cli.StringSliceFlag{
				Name:    "jetstream-hosts",
				Usage:   "List of Jetstream hosts to connect to, fallbacks to next host in list if connection fails",
				EnvVars: []string{"NORSKY_JETSTREAM_HOSTS"},
				Value: cli.NewStringSlice(
					"wss://jetstream1.us-east.bsky.network",
					"wss://jetstream2.us-east.bsky.network",
					"wss://jetstream1.us-west.bsky.network",
					"wss://jetstream2.us-west.bsky.network",
				),
			},
			&cli.BoolFlag{
				Name:    "jetstream-compress",
				Usage:   "Enable zstd compression for Jetstream websocket connection",
				EnvVars: []string{"NORSKY_JETSTREAM_COMPRESS"},
				Value:   true,
			},
			&cli.StringFlag{
				Name:    "user-agent",
				Usage:   "User agent string for Jetstream connection (e.g. 'MyApp/1.0.0 (https://example.com)')",
				EnvVars: []string{"NORSKY_USER_AGENT"},
				Value:   "",
			},
			&cli.StringSliceFlag{
				Name:    "jetstream-wanted-collections",
				Usage:   "List of collections to subscribe to (e.g. app.bsky.feed.post)",
				EnvVars: []string{"NORSKY_JETSTREAM_WANTED_COLLECTIONS"},
				Value: cli.NewStringSlice(
					"app.bsky.feed.post", // Default to posts only
				),
			},
			&cli.StringFlag{
				Name:    "db-host",
				Usage:   "PostgreSQL host",
				EnvVars: []string{"NORSKY_DB_HOST"},
				Value:   "localhost",
			},
			&cli.IntFlag{
				Name:    "db-port",
				Usage:   "PostgreSQL port",
				EnvVars: []string{"NORSKY_DB_PORT"},
				Value:   5432,
			},
			&cli.StringFlag{
				Name:    "db-user",
				Usage:   "PostgreSQL user",
				EnvVars: []string{"NORSKY_DB_USER"},
				Value:   "norsky",
			},
			&cli.StringFlag{
				Name:    "db-password",
				Usage:   "PostgreSQL password",
				EnvVars: []string{"NORSKY_DB_PASSWORD"},
				Value:   "norsky",
			},
			&cli.StringFlag{
				Name:    "db-name",
				Usage:   "PostgreSQL database name",
				EnvVars: []string{"NORSKY_DB_NAME"},
				Value:   "norsky",
			},
		},

		Action: func(ctx *cli.Context) error {

			log.Info("Starting Norsky feed generator")

			// Run migrations first
			if err := db.Migrate(
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-user"),
				ctx.String("db-password"),
				ctx.String("db-name"),
			); err != nil && err != migrate.ErrNoChange {
				return fmt.Errorf("failed to run migrations: %w", err)
			}

			// Initialize single database connection for both reads and writes
			database := db.NewDB(
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-user"),
				ctx.String("db-password"),
				ctx.String("db-name"),
			)

			hostname := ctx.String("hostname")
			host := ctx.String("host")
			port := ctx.Int("port")
			confidenceThreshold := ctx.Float64("confidence-threshold")
			runLanguageDetection := ctx.Bool("run-language-detection")
			jetstreamHosts := ctx.StringSlice("jetstream-hosts")
			jetstreamCompress := ctx.Bool("jetstream-compress")
			userAgent := ctx.String("user-agent")
			wantedCollections := ctx.StringSlice("jetstream-wanted-collections")

			if hostname == "" {
				return errors.New("missing required flag: --hostname")
			}

			if confidenceThreshold < 0 || confidenceThreshold > 1.0 {
				return errors.New("confidence-threshold must be between 0 and 1")
			}

			// Channel for subscribing to bluesky posts
			firehoseCtx := context.Context(ctx.Context)
			livenessTicker := time.NewTicker(15 * time.Minute)
			postChan := make(chan interface{}, 1000)

			// Get initial sequence
			// seq, err := database.GetSequence()
			// if err != nil {
			// 	log.Warn("Failed to get sequence from database, starting from 0")
			// }

			// Setup the server and firehose
			cfg, err := config.LoadConfig(ctx.String("config"))
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Initialize feeds and pass to server
			feedMap := feeds.InitializeFeeds(cfg)

			// Get unique languages from all feeds
			languages := make(map[string]struct{})
			detectAllLanguages := false

			// First pass to check if any feed wants all languages
			for _, feed := range cfg.Feeds {
				if len(feed.Languages) == 0 {
					detectAllLanguages = true
					break
				}
			}

			// If no feed wants all languages, collect specified languages
			if !detectAllLanguages {
				for _, feed := range cfg.Feeds {
					for _, lang := range feed.Languages {
						languages[lang] = struct{}{}
					}
				}
			}

			// Convert to slice
			targetLanguages := make([]string, 0)
			if detectAllLanguages {
				// If any feed wants all languages, we'll pass an empty slice
				// which the firehose will interpret as "detect all languages"
				log.Info("Detecting all languages due to feed with empty language specification")
			} else {
				targetLanguages = make([]string, 0, len(languages))
				for lang := range languages {
					targetLanguages = append(targetLanguages, lang)
				}
				log.Infof("Detecting specific languages: %v", targetLanguages)
			}

			// Create the server with unified database connection
			app := server.Server(&server.ServerConfig{
				Hostname: hostname,
				DB:       database,
				Feeds:    feedMap,
			})

			// Process posts using the unified database connection
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in post processing routine: %v", r)
					}
				}()
				for post := range postChan {
					switch event := post.(type) {
					case models.CreatePostEvent:
						if err := database.CreatePost(ctx.Context, event.Post); err != nil {
							log.Errorf("Failed to create post: %v", err)
						}
					case models.DeletePostEvent:
						if err := database.DeletePost(ctx.Context, event.Post); err != nil {
							log.Errorf("Failed to delete post: %v", err)
						}
					default:
						log.Info("Unknown post type")
					}
				}
			}()

			go func() {
				fmt.Println("Subscribing to firehose...")
				firehose.Subscribe(firehoseCtx, postChan, livenessTicker, database, firehose.FirehoseConfig{
					RunLanguageDetection: runLanguageDetection,
					ConfidenceThreshold:  confidenceThreshold,
					Languages:            targetLanguages,
					JetstreamHosts:       jetstreamHosts,
					JetstreamCompress:    jetstreamCompress,
					UserAgent:            userAgent,
					WantedCollections:    wantedCollections,
				})
			}()

			go func() {
				<-ctx.Done()
				log.Info("Context canceled with reason:", ctx.Err())
			}()

			// Add liveness probe to the server, to check if we are still receiving posts on the web socket
			// If not we need to restart the firehose connection

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in firehose liveness ticker: %v", r)
					}
				}()

				var lastActivity atomic.Int64

				for range livenessTicker.C {
					select {
					case <-ctx.Done():
						return
					default:
						if time.Since(time.Unix(lastActivity.Load(), 0)) > 15*time.Minute {
							log.Warn("Firehose inactive for 15 minutes, restarting connection")

							// Cancel existing context
							if cancel, ok := firehoseCtx.Value(cancelKey).(context.CancelFunc); ok {
								cancel()
							}

							// Create new context with cancel
							firehoseCtx, cancel := context.WithCancel(ctx.Context)
							firehoseCtx = context.WithValue(firehoseCtx, cancelKey, cancel)

							// Restart subscription in new goroutine
							go firehose.Subscribe(firehoseCtx, postChan, livenessTicker, database, firehose.FirehoseConfig{
								RunLanguageDetection: ctx.Bool("run-language-detection"),
								ConfidenceThreshold:  ctx.Float64("confidence-threshold"),
								Languages:            targetLanguages,
								JetstreamHosts:       ctx.StringSlice("jetstream-hosts"),
								JetstreamCompress:    ctx.Bool("jetstream-compress"),
								UserAgent:            ctx.String("user-agent"),
								WantedCollections:    ctx.StringSlice("jetstream-wanted-collections"),
							})
						}
					}
				}
			}()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic: %v", r)
					}
				}()
				fmt.Println("Starting server...")

				if err := app.Listen(fmt.Sprintf("%s:%d", host, port)); err != nil {
					log.Error(err)
				}
			}()

			// Wait for SIGINT (Ctrl+C) or SIGTERM (docker stop) to stop the server

			<-ctx.Context.Done()

			log.Info("Stopping server")
			if err := app.ShutdownWithContext(ctx.Context); err != nil {
				log.Error(err)
			}
			log.Info("Norsky feed generator stopped")

			return nil
		},
	}
}
