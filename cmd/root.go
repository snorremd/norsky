/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/urfave/cli/v2"
)

func RootApp() *cli.App {
	return &cli.App{
		Name:  "norsky",
		Usage: "A Bluesky feed for Norwegian Bluesky posts",
		Description: `A Bluesky feed that only shows posts written in
		Norwegian bokmål, nynorsk and sami.

		Norsky works by subscribing to the Bluesky firehose and filtering
		down the result to posts written in Norwegian bokmål, nynorsk and sami.
		The result is written to an SQLite database and can be accessed via an
		HTTP API as specified in the Bluesky API specification.

		Flags can generally be set via environment variables, e.g.:

		--database => NORSKY_DATABASE=feed.db
		--port => NORSKY_PORT=8080
		`,
		Commands: []*cli.Command{
			serveCmd(),
			migrateCmd(),
			rollbackCmd(),
			tidyCmd(),
			subscribeCmd(),
			publishCmd(),
			unpublishCmd(),
		},
		Action: func(ctx *cli.Context) error {
			// Show help if no command is specified
			return ctx.App.Run([]string{"", "help"})
		},
	}
}
