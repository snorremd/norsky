/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"norsky/bluesky"
	"os"
	"time"

	"norsky/config"

	"github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/util"
	"github.com/cqroot/prompt"
	"github.com/cqroot/prompt/input"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
)

// publishCmd represents the publish command
func publishCmd() *cli.Command {
	return &cli.Command{
		Name:  "publish",
		Usage: "Publish feeds on Bluesky",
		Description: `Publishes feeds on Bluesky.:

A Bluesky user account is required to publish feeds on Bluesky.
Registers the feed with your preferred name, description, etc.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "hostname",
				Aliases: []string{"n"},
				Usage:   "The hostname where the server is running",
				EnvVars: []string{"NORSKY_HOSTNAME"},
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config/feeds.toml",
				Usage:   "Path to feeds configuration file",
				EnvVars: []string{"NORSKY_CONFIG"},
			},
		},
		Action: func(ctx *cli.Context) error {
			// This command was made possible thanks to the appreciated work by the Bluesky Furry Feed team

			// Hostname of the Feed Generator
			hostname := ctx.String("hostname")

			if hostname == "" {
				return errors.New("please specify a hostname in the config")
			}

			handle, err := prompt.New().Ask("Handle:").Input("myname.bsky.social")
			if err != nil {
				return err
			}

			password, err := prompt.New().Ask("Password:").Input("", input.WithEchoMode(input.EchoNone))
			if err != nil {
				return err
			}

			client, err := bluesky.ClientFromCredentials(ctx.Context, bluesky.DefaultPDSHost, &bluesky.Credentials{
				Identifier: handle,
				Password:   password,
			})
			if err != nil {
				return fmt.Errorf("could not create client with provided credentials: %w", err)
			}

			actorFeeds, err := client.GetActorFeeds(ctx.Context, handle)
			if err != nil {
				return fmt.Errorf("could not get actor feeds: %w", err)
			}

			cfg, err := config.LoadConfig(ctx.String("config"))
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			for _, feed := range cfg.Feeds {
				existingFeed, ok := lo.Find(actorFeeds.Feeds, func(f *bsky.FeedDefs_GeneratorView) bool {
					parsed, err := util.ParseAtUri(f.Uri)
					if err != nil {
						return false
					}
					return parsed.Rkey == feed.Id
				})

				var cid *string
				if ok && existingFeed != nil {
					cid = &existingFeed.Cid
				}

				// Get the feed avatar from file
				var blob *lexutil.LexBlob
				if feed.AvatarPath != "" {
					f, err := os.Open(feed.AvatarPath)
					if err != nil {
						return fmt.Errorf("could not open avatar file for feed %s: %w", feed.Id, err)
					}
					defer f.Close()

					blob, err = client.UploadBlob(ctx.Context, f)
					if err != nil {
						return fmt.Errorf("could not upload avatar blob for feed %s: %w", feed.Id, err)
					}
				}

				err := client.PutFeedGenerator(ctx.Context, feed.Id, &bsky.FeedGenerator{
					Avatar:      blob,
					Did:         fmt.Sprintf("did:web:%s", hostname),
					CreatedAt:   bluesky.FormatTime(time.Now().UTC()),
					DisplayName: feed.DisplayName,
					Description: &feed.Description,
				}, cid)

				if err != nil {
					return fmt.Errorf("could not publish feed: %w", err)
				}
				fmt.Println("Published feed...", feed.DisplayName)
			}

			return nil
		},
	}
}

func unpublishCmd() *cli.Command {
	return &cli.Command{
		Name:  "unpublish",
		Usage: "Unpublish feeds from Bluesky",
		Description: `Unpublishes all feeds associated with account publisher on Bluesky.:

A Bluesky user account is required to unpublish feeds on Bluesky.`,

		Action: func(ctx *cli.Context) error {
			handle, err := prompt.New().Ask("Handle:").Input("myname.bsky.social")
			if err != nil {
				return err
			}

			password, err := prompt.New().Ask("Password:").Input("", input.WithEchoMode(input.EchoNone))
			if err != nil {
				return err
			}

			client, err := bluesky.ClientFromCredentials(ctx.Context, bluesky.DefaultPDSHost, &bluesky.Credentials{
				Identifier: handle,
				Password:   password,
			})

			if err != nil {
				return err
			}

			fmt.Println("Unpublishing feeds...")

			return client.DeleteAllFeeds(ctx.Context)

		},
	}
}
