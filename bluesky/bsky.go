package bluesky

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	util "github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/labstack/gommon/log"
)

const DefaultPDSHost = "https://bsky.social"

type Credentials struct {
	Identifier string
	Password   string
}

type Client struct {
	xrpc *xrpc.Client
}

func ClientFromCredentials(ctx context.Context, host string, creds *Credentials) (*Client, error) {
	auth, err := atproto.ServerCreateSession(ctx, &xrpc.Client{Host: host}, &atproto.ServerCreateSession_Input{
		Identifier: creds.Identifier,
		Password:   creds.Password,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	xrpcClient := &xrpc.Client{
		Host: host,
		Auth: &xrpc.AuthInfo{
			AccessJwt:  auth.AccessJwt,
			RefreshJwt: auth.RefreshJwt,
			Handle:     auth.Handle,
			Did:        auth.Did,
		},
		Client: http.DefaultClient,
	}

	return &Client{xrpc: xrpcClient}, nil
}

// UploadBlob uploads a blob (binary data like an image) to the Bluesky network.
// It takes a context and an io.Reader containing the blob data.
// Returns the uploaded blob's metadata or an error if the upload fails.
func (c *Client) UploadBlob(ctx context.Context, r io.Reader) (*lexutil.LexBlob, error) {
	resp, err := atproto.RepoUploadBlob(ctx, c.xrpc, r)
	if err != nil {
		return nil, fmt.Errorf("failed to upload blob: %w", err)
	}
	return resp.Blob, nil
}

// PutFeedGenerator creates a feed generator record for the current user.
// If the feed generator already exists, it will be updated.
// The rkey is the unique identifier for the feed generator in your own user repository.
func (c *Client) PutFeedGenerator(ctx context.Context, rkey string, record *bsky.FeedGenerator, cid *string) error {

	_, err := atproto.RepoPutRecord(ctx, c.xrpc, &atproto.RepoPutRecord_Input{
		Collection: "app.bsky.feed.generator",
		Repo:       c.xrpc.Auth.Did,
		Rkey:       rkey,
		SwapRecord: cid,
		Record: &lexutil.LexiconTypeDecoder{
			Val: record,
		},
	})
	if err != nil {
		// Display the entire http response error so we can see what went wrong
		log.Errorf("failed to put record: %s", err)
		return fmt.Errorf("failed to put record: %w", err)
	}
	return nil
}

func (c *Client) GetActorFeeds(ctx context.Context, actor string) (*bsky.FeedGetActorFeeds_Output, error) {
	return bsky.FeedGetActorFeeds(ctx, c.xrpc, actor, "", 100)
}

// DeleteAllFeeds deletes all feed generators for the current user.
// This can be useful if you want to remove feeds registered by a
// previous version of the app.
func (c *Client) DeleteAllFeeds(ctx context.Context) error {
	// List all feed generators
	resp, err := bsky.FeedGetActorFeeds(ctx, c.xrpc, c.xrpc.Auth.Did, "", 100)

	if err != nil {
		return fmt.Errorf("failed to list feeds: %w", err)
	}

	// Delete each feed generator
	for _, feed := range resp.Feeds {

		uri, err := util.ParseAtUri(feed.Uri)
		if err != nil {
			return fmt.Errorf("failed to parse at uri: %w", err)
		}

		_, err = atproto.RepoDeleteRecord(ctx, c.xrpc, &atproto.RepoDeleteRecord_Input{
			Collection: uri.Collection,
			Repo:       uri.Did,
			Rkey:       uri.Rkey,
		})
		if err != nil {
			return fmt.Errorf("failed to delete feed %s: %w", uri.Rkey, err)
		}
	}
	return nil
}

// FormatTime formats a time.Time into the format expected by AT Protocol
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000Z")
}
