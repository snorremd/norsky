package firehose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"norsky/models"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/events/schedulers/sequential"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/repo"
	"github.com/bluesky-social/indigo/repomgr"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

// Subscribe to the firehose using the Firehose struct as a receiver
func Subscribe(ctx context.Context, postChan chan interface{}, seq int64) {
	address := "wss://bsky.network/xrpc/com.atproto.sync.subscribeRepos"
	if seq >= 0 {
		log.Info("Starting from sequence: ", seq)
		address = fmt.Sprintf("%s?cursor=%d", address, seq)
	}
	dialer := websocket.DefaultDialer
	backoff := backoff.NewExponentialBackOff()

	// Check if context is cancelled, if so exit the connection loop
	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping firehose connect loop")
			return
		default:
			conn, _, err := dialer.Dial(address, nil)
			if err != nil {
				log.Errorf("Error connecting to firehose: %s", err)
				time.Sleep(backoff.NextBackOff())
				// Increase backoff by factor of 1.3, rounded to nearest whole number
				continue
			}

			scheduler := sequential.NewScheduler(conn.RemoteAddr().String(), eventProcessor(postChan, ctx).EventHandler)
			err = events.HandleRepoStream(ctx, conn, scheduler)

			// If error sleep
			if err != nil {
				log.Errorf("Error handling repo stream: %s", err)
				time.Sleep(backoff.NextBackOff())
				continue
			}
		}
	}
}

func eventProcessor(postChan chan interface{}, context context.Context) *events.RepoStreamCallbacks {
	streamCallbacks := &events.RepoStreamCallbacks{
		RepoCommit: func(evt *atproto.SyncSubscribeRepos_Commit) error {
			rr, err := repo.ReadRepoFromCar(context, bytes.NewReader(evt.Blocks))
			if err != nil {
				log.Errorf("Error reading repo from car: %s", err)
				return nil
			}
			// Get operations by type
			for _, op := range evt.Ops {
				if strings.Split(op.Path, "/")[0] != "app.bsky.feed.post" {
					continue // Skip if not a post, e.g. like, follow, etc.
				}

				uri := fmt.Sprintf("at://%s/%s", evt.Repo, op.Path)
				event_type := repomgr.EventKind(op.Action)

				switch event_type {
				case repomgr.EvtKindCreateRecord, repomgr.EvtKindUpdateRecord:
					_, rec, err := rr.GetRecord(context, op.Path)
					if err != nil {
						continue
					}

					decoder := lexutil.LexiconTypeDecoder{
						Val: rec,
					}

					jsonRecord, err := decoder.MarshalJSON() // The LexiconTypeDecoder will decode the record into a JSON representation

					if err != nil {
						continue
					}

					var post = appbsky.FeedPost{} // Unmarshal JSON formatted record into a FeedPost
					err = json.Unmarshal(jsonRecord, &post)
					if err != nil {
						continue
					}

					// Contains any of the languages in the post that are one of the following: nb, nn, smi
					if lo.Some(post.Langs, []string{"no", "nb", "nn", "smi"}) {
						// Keep track of what commits we have processed
						postChan <- models.ProcessSeqEvent{
							Seq: evt.Seq,
						}
						createdAt, err := time.Parse(time.RFC3339, post.CreatedAt)
						if err == nil {
							postChan <- models.CreatePostEvent{
								Post: models.Post{
									Uri:       uri,
									CreatedAt: createdAt.Unix(),
									Text:      post.Text,
									Languages: post.Langs,
								},
							}
						}
					}

				}
			}

			return nil
		},
	}

	return streamCallbacks
}
