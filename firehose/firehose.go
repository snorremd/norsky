package firehose

import (
	"context"
	"time"
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
func Subscribe(ctx context.Context, postChan chan interface{}, ticker *time.Ticker, seq int64, config FirehoseConfig) {

	// Create a new parallel processor to deserialize and process incoming messages from the jetstream firehose
	pp := NewParallelProcessor(ctx, 10, 1000, config, postChan)

	// Subscribe to the jetstream firehose
	SubscribeJetstreamWithMessages(ctx, JetstreamConfig{
		Hosts:             config.JetstreamHosts,
		Compress:          config.JetstreamCompress,
		UserAgent:         config.UserAgent,
		WantedCollections: config.WantedCollections,
	}, pp.workerQueue)

	// Create a new parallel processor to process incoming messages from the jetstream firehose
	pp.start()

}
