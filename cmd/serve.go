/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"norsky/firehose"
	"norsky/server"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the norsky feed",
	Long: `Starts the norsky feed HTTP server and firehose subscriber.

Launches the HTTP server on the specified or default port and subscribes to the
Bluesky firehose. All posts written in Norwegian bokmål, nynorsk and sami are
written to the SQLite database and can be accessed via the HTTP API as specified
in the Bluesky API specification.`,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("Starting norsky...")

		// Channel for subscribing to bluesky posts
		postChan := make(chan interface{})

		// Setup the server and firehose
		app := server.Server()
		fh := firehose.New(postChan)

		// Graceful shutdown
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		var wg sync.WaitGroup

		go func() {
			<-c
			fmt.Println("Gracefully shutting down...")
			app.ShutdownWithTimeout(60 * time.Second)
			defer wg.Add(-2) // Decrement the waitgroup counter by 2 after shutdown of server and firehose
			fh.Shutdown()
		}()

		go func() {
			fmt.Println("Subscribing to firehose...")
			if err := fh.Subscribe(); err != nil {
				log.Panic(err)
			}
		}()

		go func() {
			fmt.Println("Starting server...")
			if err := app.Listen(":3000"); err != nil {
				log.Panic(err)
			}
		}()

		go func() {
			// Subscribe to the post channel and log the posts
			for post := range postChan {
				fmt.Println(post)
			}
		}()

		// Wait for both the server and firehose to shutdown
		wg.Add(2)
		wg.Wait()

		fmt.Println("Done!")

	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
