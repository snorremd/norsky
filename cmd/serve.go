/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"norsky/db"
	"norsky/firehose"
	"norsky/models"
	"norsky/server"
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
			// Create new child context for the firehose connection
			firehoseCtx := context.Context(ctx.Context)
			livenessTicker := time.NewTicker(5 * time.Minute)
			postChan := make(chan interface{})
			statisticsChan := make(chan models.StatisticsEvent, 1000)
			dbPostChan := make(chan interface{})   // Channel for writing posts to the database
			broadcaster := server.NewBroadcaster() // SSE broadcaster

			dbReader := db.NewReader(database)
			seq, err := dbReader.GetSequence()

			if err != nil {
				log.Warn("Failed to get sequence from database, starting from 0")
			}

			// Setup the server and firehose
			app := server.Server(&server.ServerConfig{
				Hostname:    hostname,
				Reader:      dbReader,
				Broadcaster: broadcaster,
			})

			// Some glue code to pass posts from the firehose to the database and/or broadcaster
			// Ideally one might want to do this in a more elegant way
			// TODO: Move broadcaster into server package, i.e. make server a receiver and handle broadcaster and fiber together
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in post broadcast routine: %v", r)
					}
				}()
				for post := range postChan {
					switch post := post.(type) {
					// Don't crash if broadcast fails
					case models.CreatePostEvent:
						dbPostChan <- post
						// Broadcast without blocking
						go broadcaster.BroadcastCreatePost(post) // Broadcast new post to SSE clients
					default:
						dbPostChan <- post
					}
				}
			}()

			// Glue code to pass statistics events to the broadcaster
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in statistics broadcast routine: %v", r)
					}
				}()

				for stats := range statisticsChan {
					broadcaster.BroadcastStatistics(stats)
				}
			}()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in firehose subscribe routine: %v", r)
					}
				}()
				fmt.Println("Subscribing to firehose...")
				firehose.Subscribe(firehoseCtx, postChan, livenessTicker, seq)
			}()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic in monitor firehose routine: %v", r)
					}
				}()
				fmt.Println("Starting statistics monitor...")
				firehose.MonitorFirehoseStats(ctx.Context, statisticsChan)
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
				for range livenessTicker.C {
					// If we get here the firehose stopped sending posts so we need to restart the connection
					log.Errorf("Firehose liveness probe failed, restarting connection")
					firehoseCtx.Done()
					firehoseCtx = context.Context(ctx.Context)
					firehose.Subscribe(firehoseCtx, postChan, livenessTicker, seq)
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

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Recovered from panic: %v", r)
					}
				}()
				fmt.Println("Starting database writer...")
				db.Subscribe(ctx.Context, database, dbPostChan)
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
