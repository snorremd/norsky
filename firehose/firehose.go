package firehose

import (
	"context"
	"norsky/db"
	"time"

	log "github.com/sirupsen/logrus"
)

// FirehoseConfig holds configuration for the firehose processing
type FirehoseConfig struct {
	RunLanguageDetection bool
	ConfidenceThreshold  float64
	Languages            []string
	JetstreamHosts       []string
	JetstreamCompress    bool
	UserAgent            string
	WantedCollections    []string
}

// Subscribe to the firehose using the Firehose struct as a receiver
func Subscribe(ctx context.Context, postChan chan interface{}, ticker *time.Ticker, db *db.DB, config FirehoseConfig) {
	// Get latest post timestamp
	latestTime, err := db.GetLatestPostTimestamp(ctx)
	if err != nil {
		log.Errorf("Failed to get latest post timestamp: %v", err)
	}

	// If we have a latest post, start 10 seconds before it
	var cursor int64
	if !latestTime.IsZero() {
		cursor = latestTime.Add(-10 * time.Second).UnixMicro()
	}

	// Create a new parallel processor
	pp := NewParallelProcessor(ctx, 10, 1000, db, config)

	// Subscribe to the jetstream firehose with cursor
	SubscribeJetstreamWithMessages(ctx, JetstreamConfig{
		Hosts:             config.JetstreamHosts,
		Compress:          config.JetstreamCompress,
		UserAgent:         config.UserAgent,
		WantedCollections: config.WantedCollections,
		Cursor:            cursor, // Add the cursor
	}, pp.workerQueue)

	// Start the parallel processor
	pp.start()
}
