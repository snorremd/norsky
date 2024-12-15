package firehose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"norsky/models"
	"strings"
	"sync/atomic"
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
	lingua "github.com/pemistahl/lingua-go"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

// Static list of languages to use for lingua-go language detection

var languages = []lingua.Language{
	lingua.Bokmal,
	lingua.Nynorsk,
}

var detector = lingua.NewLanguageDetectorBuilder().FromLanguages(languages...).Build()

// Keep track of processed event and posts count to show stats in the web interface

var (
	processedEvents int64
	processedPosts  int64
)

// Subscribe to the firehose using the Firehose struct as a receiver
func Subscribe(ctx context.Context, postChan chan interface{}, ticker *time.Ticker, seq int64) {

	address := "wss://bsky.network/xrpc/com.atproto.sync.subscribeRepos"
	headers := http.Header{}
	headers.Set("User-Agent", "NorSky: https://github.com/snorremd/norsky")

	if seq >= 0 {
		log.Info("Starting from sequence: ", seq)
		address = fmt.Sprintf("%s?cursor=%d", address, seq)
	}
	// Identify dialer with User-Agent header

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 30 * time.Second
	backoff := backoff.NewExponentialBackOff()
	backoff.InitialInterval = 1 * time.Second
	backoff.MaxInterval = 600 * time.Second
	backoff.Multiplier = 1.5
	backoff.MaxElapsedTime = 0

	// Check if context is cancelled, if so exit the connection loop
	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping firehose connect loop")
			return
		default:
			conn, _, err := dialer.Dial(address, headers)
			if err != nil {
				log.Errorf("Error connecting to firehose: %s", err)
				time.Sleep(backoff.NextBackOff())
				continue
			}

			// Reset backoff on successful connection
			backoff.Reset()

			// Set initial deadlines
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			// Start ping ticker
			pingTicker := time.NewTicker(60 * time.Second)
			defer pingTicker.Stop()

			// Start ping goroutine
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-pingTicker.C:
						log.Debug("Sending ping to check connection")
						if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
							log.Warn("Ping failed, closing connection for restart: ", err)
							conn.Close()
							return
						}
						// Reset read deadline after successful ping
						if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
							log.Warn("Failed to set read deadline, closing connection: ", err)
							conn.Close()
							return
						}
					}
				}
			}()

			// Remove pong handler since server doesn't respond
			// Keep ping handler for completeness
			conn.SetPingHandler(func(appData string) error {
				log.Debug("Received ping from server")
				return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			})

			scheduler := sequential.NewScheduler(conn.RemoteAddr().String(), eventProcessor(postChan, ctx, ticker).EventHandler)
			err = events.HandleRepoStream(ctx, conn, scheduler)

			// If error sleep
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Info("Websocket closed normally")
				} else if err == io.EOF {
					log.Warn("Connection closed by server")
				} else {
					log.Errorf("Error handling repo stream: %s", err)
				}
				conn.Close()
				// Use shorter backoff for normal closures
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					time.Sleep(time.Second)
				} else {
					time.Sleep(backoff.NextBackOff())
				}
				continue
			}
		}
	}
}

func MonitorFirehoseStats(ctx context.Context, statisticsChan chan models.StatisticsEvent) {
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			// Send statistics event
			statisticsChan <- models.StatisticsEvent{
				// Divide by 5 and round to get average per second
				EventsPerSecond: atomic.LoadInt64(&processedEvents) / 5,
				PostsPerSecond:  atomic.LoadInt64(&processedPosts) / 5,
			}
			// Reset processed events and posts
			atomic.StoreInt64(&processedEvents, 0)
			atomic.StoreInt64(&processedPosts, 0)
		case <-ctx.Done():
			log.Info("Stopping statistics ticker")
			return
		}
	}
}

func eventProcessor(postChan chan interface{}, context context.Context, ticker *time.Ticker) *events.RepoStreamCallbacks {
	streamCallbacks := &events.RepoStreamCallbacks{
		RepoCommit: func(evt *atproto.SyncSubscribeRepos_Commit) error {
			// Keep track of processed events
			atomic.AddInt64(&processedEvents, 1)

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
					// Keep track of processed posts
					atomic.AddInt64(&processedPosts, 1)

					ticker.Reset(5 * time.Minute)
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

					// Contains any of the languages in the post that are one of the following: nb, nn, se
					if lo.Some(post.Langs, []string{"no", "nb", "nn", "se"}) {

						// If tagged as no, nb, nn we need to detect the language to weed out false positives
						if lo.Some(post.Langs, []string{"no", "nb", "nn"}) {
							// Detect language
							_, exists := detector.DetectLanguageOf(post.Text)
							if !exists {
								log.Warn("Not norwegian, skipping")
								continue
							}

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
			}

			return nil
		},
	}

	return streamCallbacks
}
