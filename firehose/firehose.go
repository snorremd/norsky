package firehose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"norsky/database"
	"strings"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/events/schedulers/sequential"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/repo"
	"github.com/bluesky-social/indigo/repomgr"
	"github.com/gorilla/websocket"
	"github.com/samber/lo"
)

// Add a firehose model to use as a receiver pattern for the firehose

type Firehose struct {
	address   string                // The address of the firehose
	dialer    *websocket.Dialer     // The websocket dialer to use for the firehose
	conn      *websocket.Conn       // The websocket connection to the firehose
	scheduler *sequential.Scheduler // The scheduler to use for the firehose
	// A channel to write feed posts to
	postChan chan interface{}
}

func New(postChan chan interface{}) *Firehose {
	dialer := websocket.DefaultDialer
	firehose := &Firehose{
		address:  "wss://bsky.social/xrpc/com.atproto.sync.subscribeRepos",
		dialer:   dialer,
		postChan: postChan,
	}

	return firehose
}

// Subscribe to the firehose using the Firehose struct as a receiver
func (firehose *Firehose) Subscribe() error {
	conn, _, err := firehose.dialer.Dial(firehose.address, nil)
	if err != nil {
		fmt.Printf("Error connecting to firehose: %s\n", err)
		return err
	}

	firehose.conn = conn
	firehose.scheduler = sequential.NewScheduler(conn.RemoteAddr().String(), eventProcessor(firehose.postChan).EventHandler)
	events.HandleRepoStream(context.Background(), conn, firehose.scheduler)

	return nil
}

func (firehose *Firehose) Shutdown() {
	firehose.scheduler.Shutdown()
	firehose.conn.Close()
	fmt.Println("Firehose shutdown")
}

func eventProcessor(postChan chan interface{}) *events.RepoStreamCallbacks {
	streamCallbacks := &events.RepoStreamCallbacks{
		RepoCommit: func(evt *atproto.SyncSubscribeRepos_Commit) error {
			rr, err := repo.ReadRepoFromCar(context.TODO(), bytes.NewReader(evt.Blocks))
			if err != nil {
				fmt.Printf("Error reading repo from car: %s\n", err)
				return err
			}
			// Get operations by type
			for _, op := range evt.Ops {
				if strings.Split(op.Path, "/")[0] != "app.bsky.feed.post" {
					continue
				}

				uri := fmt.Sprintf("at://%s/%s", evt.Repo, op.Path)

				event_type := repomgr.EventKind(op.Action)
				switch event_type {
				case repomgr.EvtKindCreateRecord, repomgr.EvtKindUpdateRecord:
					_, rec, err := rr.GetRecord(context.TODO(), op.Path)
					if err != nil {
						fmt.Printf("Error getting record %s: %s\n", op.Path, err)
						continue
					}

					decoder := lexutil.LexiconTypeDecoder{
						Val: rec,
					}

					var post = appbsky.FeedPost{}

					marshaller, err := decoder.MarshalJSON()

					if err != nil {
						fmt.Println(err)
					}

					err = json.Unmarshal(marshaller, &post)
					if err != nil {
						fmt.Println(err)
					}

					if lo.Contains(post.Langs, "nb") {
						postChan <- database.CreatePostEvent{
							Post: database.Post{
								Uri:       uri,
								CreatedAt: post.CreatedAt,
							},
						}
					}

				}
			}

			return nil
		},
	}

	return streamCallbacks
}
