/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"norsky/firehose"
	"norsky/models"
	"os"
	"os/signal"
	"sync"

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
			context := context.Background()

			// Disable logging to stdout
			log.SetOutput(os.Stderr)

			// Channel for subscribing to bluesky posts
			postChan := make(chan interface{})

			// Setup the server and firehose
			fh := firehose.New(postChan, context)

			// Graceful shutdown
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			var wg sync.WaitGroup

			go func() {
				<-c
				defer wg.Add(-1) // Decrement the waitgroup counter by 2 after shutdown of server and firehose
				fh.Shutdown()
			}()

			go func() {
				fmt.Println("Subscribing to firehose...")
				if err := fh.Subscribe(); err != nil {
					log.Panic(err)
				}
			}()

			go func() {
				// Subscribe to the post channel and log the posts
				for message := range postChan {
					switch message := message.(type) {
					case models.CreatePostEvent:
						printStdout(&message.Post)
					case models.UpdatePostEvent:
						printStdout(&message.Post)
					case models.DeletePostEvent:
						printStdout(&message.Post)
					}
				}
			}()

			// Wait for both the server and firehose to shutdown
			wg.Add(1)
			wg.Wait()

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
