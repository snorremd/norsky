/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"norsky/db"

	"github.com/urfave/cli/v2"
)

func migrateCmd() *cli.Command {
	return &cli.Command{
		Name:        "migrate",
		Usage:       "Run database migrations",
		Description: `Runs database migrations on the configured database. Will create the database if it does not exist.`,
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
			err := db.Migrate(database)
			return err
		},
	}
}

func rollbackCmd() *cli.Command {
	return &cli.Command{
		Name:        "rollback",
		Usage:       "Rollback database migration",
		Description: `Rolls back the last database migration`,
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
			err := db.Rollback(database)
			return err
		},
	}
}
