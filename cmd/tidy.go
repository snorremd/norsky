/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"norsky/db"

	"github.com/urfave/cli/v2"
)

func tidyCmd() *cli.Command {
	return &cli.Command{
		Name:  "tidy",
		Usage: "Tidy up the database",
		Description: `Tidy up the database by removing posts that are old.
		
		Remove posts that are older than 90 days from the database.
		This is to keep the database size down and to keep the feed fresh.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "database",
				Aliases: []string{"d"},
				Value:   "feed.db",
				Usage:   "SQLite database file location",
				EnvVars: []string{"NORSKY_DATABASE"},
			},
		},
		Action: func(ctx *cli.Context) error {
			database := ctx.String("database")
			fmt.Println("Database configured: ", database)
			err := db.Tidy(database)
			return err
		},
	}
}
