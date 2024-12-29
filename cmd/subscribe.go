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
	"time"

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
		Action: func(ctx *cli.Context) error {
			// Get the context for this process to pass to firehose

			// Disable logging to stdout
			log.SetOutput(os.Stderr)

			// Channel for subscribing to bluesky posts
			postChan := make(chan interface{})

			ticker := time.NewTicker(5 * time.Minute)

			go func() {
				fmt.Println("Subscribing to firehose...")
				firehose.Subscribe(ctx.Context, postChan, ticker, -1, ctx.Bool("detect-false-negatives"))
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
