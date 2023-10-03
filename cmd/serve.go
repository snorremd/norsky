/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"norsky/db"
	"norsky/firehose"
	"norsky/server"
	"os"
	"os/signal"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

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
				Name:    "database",
				Aliases: []string{"d"},
				Value:   "feed.db",
				Usage:   "SQLite database file location",
				EnvVars: []string{"NORSKY_DATABASE"},
			},
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
		},
		Action: func(ctx *cli.Context) error {

			log.Info("Starting Norsky feed generator")

			database := ctx.String("database")
			hostname := ctx.String("hostname")
			host := ctx.String("host")
			port := ctx.Int("port")

			// Check if any of the required flags are missing
			if hostname == "" {
				return errors.New("missing required flag: --hostname")
			}

			err := db.Migrate(database)

			if err != nil {
				return fmt.Errorf("failed to run database migrations: %w", err)
			}

			// Channel for subscribing to bluesky posts
			postChan := make(chan interface{})

			// Setup the server and firehose
			app := server.Server(&server.ServerConfig{
				Hostname: hostname,
				Reader:   db.NewReader(database),
			})
			fh := firehose.New(postChan, ctx.Context)
			dbwriter := db.NewWriter(database, postChan)

			// Graceful shutdown via wait group
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			var wg sync.WaitGroup

			go func() {
				<-c
				fmt.Println("Gracefully shutting down...")
				app.ShutdownWithTimeout(60 * time.Second)
				defer wg.Add(-3) // Decrement the waitgroup counter by 2 after shutdown of server and firehose
				fh.Shutdown()
			}()

			go func() {
				fmt.Println("Subscribing to firehose...")
				fh.Subscribe()
			}()

			go func() {
				fmt.Println("Starting server...")
				if err := app.Listen(fmt.Sprintf("%s:%d", host, port)); err != nil {
					log.Error(err)
					c <- os.Interrupt
				}
			}()

			go func() {
				fmt.Println("Starting database writer...")
				dbwriter.Subscribe()
			}()

			// Wait for both the server and firehose to shutdown
			wg.Add(3)
			wg.Wait()

			log.Info("Norsky feed generator stopped")

			return nil

		},
	}
}
