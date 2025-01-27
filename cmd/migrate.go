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
			fmt.Printf("Database configured: %s:%d/%s\n",
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-name"),
			)
			return db.Migrate(
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-user"),
				ctx.String("db-password"),
				ctx.String("db-name"),
			)
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
			fmt.Printf("Database configured: %s:%d/%s\n",
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-name"),
			)
			return db.Rollback(
				ctx.String("db-host"),
				ctx.Int("db-port"),
				ctx.String("db-user"),
				ctx.String("db-password"),
				ctx.String("db-name"),
			)
		},
	}
}
