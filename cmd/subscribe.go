/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"norsky/firehose"
	"norsky/models"
	"os"
	"os/signal"
	"syscall"
	"time"

	"norsky/config"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// logCmd represents the log command
func subscribeCmd() *cli.Command {
	return &cli.Command{
		Name:  "subscribe",
		Usage: "Log all norwegian posts to the command line",
		Description: `Subscribe to the Bluesky firehose and log all posts written in
Norwegian bokmål, nynorsk and sami to the command line.

Can be used if you want to collect all posts written in Norwegian by
passing the output to a file or another application.

Returns each post as a JSON object on a single line. Use a tool like jq to process
the output.

Prints all other log messages to stderr.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "detect-false-negatives",
				Usage:   "Detect false negatives in language detection",
				EnvVars: []string{"NORSKY_DETECT_FALSE_NEGATIVES"},
			},
			&cli.Float64Flag{
				Name:    "confidence-threshold",
				Usage:   "Confidence threshold for language detection (0-1)",
				EnvVars: []string{"NORSKY_CONFIDENCE_THRESHOLD"},
				Value:   0.6,
			},
			&cli.StringSliceFlag{
				Name:    "jetstream-hosts",
				Usage:   "Jetstream hosts",
				EnvVars: []string{"NORSKY_JETSTREAM_HOSTS"},
			},
			&cli.BoolFlag{
				Name:    "jetstream-compress",
				Usage:   "Jetstream compress",
				EnvVars: []string{"NORSKY_JETSTREAM_COMPRESS"},
			},
			&cli.StringFlag{
				Name:    "user-agent",
				Usage:   "User agent",
				EnvVars: []string{"NORSKY_USER_AGENT"},
			},
			&cli.StringSliceFlag{
				Name:    "jetstream-wanted-collections",
				Usage:   "Jetstream wanted collections",
				EnvVars: []string{"NORSKY_JETSTREAM_WANTED_COLLECTIONS"},
			},
			&cli.BoolFlag{
				Name:    "run-language-detection",
				Usage:   "Run language detection on posts",
				EnvVars: []string{"NORSKY_RUN_LANGUAGE_DETECTION"},
				Value:   true,
			},
		},
		Action: func(ctx *cli.Context) error {
			// Get the context for this process to pass to firehose

			// Disable logging to stdout
			log.SetOutput(os.Stderr)

			// Load feed configuration
			cfg, err := config.LoadConfig("feeds.toml")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Get unique languages from all feeds
			languages := make(map[string]struct{})
			detectAllLanguages := false

			// First pass to check if any feed wants all languages
			for _, feed := range cfg.Feeds {
				hasLanguageFilter := false
				for _, filter := range feed.Filters {
					if filter.Type == "language" && len(filter.Languages) > 0 {
						hasLanguageFilter = true
						for _, lang := range filter.Languages {
							languages[lang] = struct{}{}
						}
					}
				}
				if !hasLanguageFilter {
					detectAllLanguages = true
					break
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

			// Channel for subscribing to bluesky posts
			postChan := make(chan interface{})

			ticker := time.NewTicker(5 * time.Minute)

			go func() {
				fmt.Println("Subscribing to firehose...")
				firehose.Subscribe(
					ctx.Context,
					postChan,
					ticker,
					nil,
					firehose.FirehoseConfig{
						RunLanguageDetection: ctx.Bool("run-language-detection"),
						ConfidenceThreshold:  ctx.Float64("confidence-threshold"),
						Languages:            targetLanguages,
						JetstreamHosts:       ctx.StringSlice("jetstream-hosts"),
						JetstreamCompress:    ctx.Bool("jetstream-compress"),
						UserAgent:            ctx.String("user-agent"),
						WantedCollections:    ctx.StringSlice("jetstream-wanted-collections"),
					},
				)
			}()

			go func() {
				// Subscribe to the post channel and log the posts
				// Stop if the context is cancelled
				for message := range postChan {
					select {
					case <-ctx.Context.Done():
						fmt.Println("Stopping subscription")
						return
					default:
						switch message := message.(type) {
						case models.CreatePostEvent:
							printStdout(&message.Post)
						case models.UpdatePostEvent:
							printStdout(&message.Post)
						case models.DeletePostEvent:
							printStdout(&message.Post)
						}
					}
				}
			}()

			// Create a signal channel to wait for interrupt
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			// Wait for either interrupt signal or context cancellation
			select {
			case <-sigChan:
				fmt.Println("Received interrupt signal, shutting down...")
			case <-ctx.Context.Done():
				fmt.Println("Context cancelled, shutting down...")
			}

			// Clean up
			ticker.Stop()
			close(postChan)

			return nil
		},
	}
}

func printStdout(post *models.Post) {
	// Print as single JSON string on a single line

	// Convert Post to JSON string
	postJson, err := json.Marshal(post)
	if err == nil {
		fmt.Println(string(postJson))
	}
}
