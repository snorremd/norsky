package firehose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"norsky/models"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/events/schedulers/autoscaling"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/repo"
	"github.com/bluesky-social/indigo/repomgr"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	lingua "github.com/pemistahl/lingua-go"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

// Some constants to optimize the firehose

const (
	wsReadBufferSize  = 1024 * 16 // 16KB
	wsWriteBufferSize = 1024 * 16 // 16KB
	eventBufferSize   = 10000     // Increase from 1000
)

// We use all languages so as to reliably separate Norwegian from other European languages
var detector lingua.LanguageDetector

func InitDetector() {
	if detector == nil {
		detector = lingua.NewLanguageDetectorBuilder().FromLanguages(lingua.AllLanguages()...).WithMinimumRelativeDistance(0.25).Build()
	}
}

// Keep track of processed event and posts count to show stats in the web interface
var (
	processedEvents int64
	processedPosts  int64
)

// Add a pool for the FeedPost struct to reduce GC pressure
// Instead of allocating new FeedPost structs for every post,
// we can reuse structs from the pool to avoid unnecessary allocations
// This is neccessary as there are 1000s of posts per second
var feedPostPool = sync.Pool{
	New: func() interface{} {
		return &appbsky.FeedPost{
			Langs: make([]string, 0, 4),
		}
	},
}

// Add this helper function at package level
func HasEnoughNorwegianLetters(text string) bool {
	if len(text) == 0 {
		return false
	}

	// Count Norwegian alphabet letters (including æøå)
	letterCount := 0
	for _, char := range text {
		// a-z, A-Z, æøå, ÆØÅ
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			char == 'æ' || char == 'ø' || char == 'å' ||
			char == 'Æ' || char == 'Ø' || char == 'Å' {
			letterCount++
		}
	}

	// If less than 30% of the text is letters, skip it
	ratio := float64(letterCount) / float64(len(text))
	return ratio > 0.30
}

// Rename to public functions
func ContainsRepetitivePattern(text string) bool {
	// Convert to lowercase for consistent matching
	text = strings.ToLower(text)

	// Remove spaces for pattern detection
	text = strings.ReplaceAll(text, " ", "")

	if len(text) < 4 {
		return false
	}

	// Split text into grapheme clusters (complete Unicode symbols)
	clusters := []string{}
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError {
			i++
			continue
		}

		// Handle emoji modifiers and zero-width joiners
		cluster := string(r)
		i += size
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError {
				break
			}
			// Check if it's a modifier or zero-width joiner
			if unicode.Is(unicode.Mn, r) || // Modifier
				r == '\u200d' || // Zero-width joiner
				r == '\ufe0f' { // Variation selector
				cluster += string(r)
				i += size
				continue
			}
			break
		}
		clusters = append(clusters, cluster)
	}

	// Check for repeating clusters
	repeatingClusters := 0
	lastCluster := ""
	for _, cluster := range clusters {
		if cluster == lastCluster {
			repeatingClusters++
			if repeatingClusters >= 4 {
				return true
			}
		} else {
			repeatingClusters = 1
			lastCluster = cluster
		}
	}

	// Check for repeating patterns up to 8 clusters long
	for patternLen := 2; patternLen <= 8; patternLen++ {
		if len(clusters) < patternLen*2 {
			continue
		}

		// Look for patterns that repeat at least twice
		for i := 0; i <= len(clusters)-patternLen*2; i++ {
			pattern := clusters[i : i+patternLen]
			repeats := 1

			// Count how many times the pattern repeats
			for j := i + patternLen; j <= len(clusters)-patternLen; j += patternLen {
				matches := true
				for k := 0; k < patternLen; k++ {
					if clusters[j+k] != pattern[k] {
						matches = false
						break
					}
				}
				if matches {
					repeats++
					// Require fewer repeats for longer patterns
					minRepeats := 4
					if patternLen >= 4 {
						minRepeats = 2
					}
					if repeats >= minRepeats {
						log.Debugf("Found repeating pattern '%v' (%d times)", pattern, repeats)
						return true
					}
				} else {
					break
				}
			}
		}
	}

	return false
}

// Rename to public functions
func ContainsSpamContent(text string) bool {
	// Convert to lowercase for case-insensitive matching
	lowerText := strings.ToLower(text)

	// Common spam patterns
	spamPatterns := []string{
		"onlyfans.com",
		"join my vip",
		"subscribe to my",
		"check my profile",
		"check my bio",
		"link in bio",
		"link in profile",
		"follow me",
		"follow back",
		"follow for follow",
		"f4f",
	}

	// NSFW terms - keep this minimal to avoid false positives
	nsfwTerms := []string{
		"porn",
		"xxx",
		"nsfw",
		"18+",
	}

	// Check for spam patterns
	for _, pattern := range spamPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}

	// Check for NSFW terms
	for _, term := range nsfwTerms {
		if strings.Contains(lowerText, term) {
			return true
		}
	}

	// Check for excessive emoji spam (common in NSFW spam)
	emojiCount := 0
	for _, r := range text {
		if r >= 0x1F300 { // Start of emoji range
			emojiCount++
			if emojiCount > 8 { // Threshold for spam
				return true
			}
		}
	}

	// Count hashtags and mentions
	hashtagCount := strings.Count(text, "#")
	mentionCount := strings.Count(text, "@")

	// If more than 5 hashtags, consider it spam
	if hashtagCount > 5 {
		log.Infof("Skipping spam post with many hashtags: %s", text)
		return true
	}

	// If more than 5 mentions, consider it spam
	if mentionCount > 5 {
		log.Infof("Skipping spam post with many mentions: %s", text)
		return true
	}

	// Check for repeated hashtags or mentions (common spam pattern)
	if strings.Count(text, "##") > 0 || strings.Count(text, "@@") > 0 {
		log.Infof("Skipping spam post with repeated hashtags/mentions: %s", text)
		return true
	}

	// Check for hashtag and mention ratios
	words := strings.Fields(text)
	if len(words) > 0 {
		// Calculate combined ratio of hashtags and mentions
		symbolRatio := float64(hashtagCount+mentionCount) / float64(len(words))
		// If more than 50% of words are hashtags or mentions combined, consider it spam
		if symbolRatio > 0.5 {
			log.Infof("Skipping spam post with high hashtag/mention ratio: %s", text)
			return true
		}
	}

	return false
}

// Subscribe to the firehose using the Firehose struct as a receiver
func Subscribe(ctx context.Context, postChan chan interface{}, ticker *time.Ticker, seq int64, detectFalseNegatives bool, confidenceThreshold float64) {
	// Validate confidence threshold
	if confidenceThreshold < 0 || confidenceThreshold > 1 {
		log.Warnf("Invalid confidence threshold %f, using default 0.6", confidenceThreshold)
		confidenceThreshold = 0.6
	}

	InitDetector()

	address := "wss://bsky.network/xrpc/com.atproto.sync.subscribeRepos"
	headers := http.Header{}
	headers.Set("User-Agent", "NorSky: https://github.com/snorremd/norsky")

	if seq >= 0 {
		log.Info("Starting from sequence: ", seq)
		address = fmt.Sprintf("%s?cursor=%d", address, seq)
	}
	// Identify dialer with User-Agent header

	dialer := websocket.Dialer{
		ReadBufferSize:   wsReadBufferSize,
		WriteBufferSize:  wsWriteBufferSize,
		HandshakeTimeout: 30 * time.Second,
	}

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

			scheduler := autoscaling.NewScheduler(
				autoscaling.AutoscaleSettings{
					MaxConcurrency:           runtime.NumCPU(),
					Concurrency:              2,
					AutoscaleFrequency:       5 * time.Second,
					ThroughputBucketDuration: 1 * time.Second,
					ThroughputBucketCount:    10,
				},
				conn.RemoteAddr().String(),
				eventProcessor(postChan, ctx, ticker, detectFalseNegatives, confidenceThreshold).EventHandler)
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

// Add new types to help organize the code
type PostProcessor struct {
	postChan             chan interface{}
	context              context.Context
	ticker               *time.Ticker
	detectFalseNegatives bool
	confidenceThreshold  float64
}

// Move language detection logic to its own function
func (p *PostProcessor) DetectNorwegianLanguage(text string, currentLangs []string) (bool, []string) {
	detectedLang, exists := detector.DetectLanguageOf(text)
	if !exists || detectedLang == lingua.English || (detectedLang != lingua.Bokmal && detectedLang != lingua.Nynorsk) {
		return false, currentLangs
	}

	// Get confidence scores for norwegian languages between 0 and 1
	bokmalConf := detector.ComputeLanguageConfidence(text, lingua.Bokmal)
	nynorskConf := detector.ComputeLanguageConfidence(text, lingua.Nynorsk)

	log.Infof("Bokmal confidence: %.2f, Nynorsk confidence: %.2f (threshold: %.2f)",
		bokmalConf, nynorskConf, p.confidenceThreshold)

	if bokmalConf < p.confidenceThreshold && nynorskConf < p.confidenceThreshold {
		return false, currentLangs
	}

	// Create new slice to avoid modifying the input
	updatedLangs := make([]string, len(currentLangs))
	copy(updatedLangs, currentLangs)

	// Add detected language if not present
	if detectedLang == lingua.Bokmal && !lo.Contains(updatedLangs, "nb") {
		updatedLangs = append(updatedLangs, "nb")
	} else if detectedLang == lingua.Nynorsk && !lo.Contains(updatedLangs, "nn") {
		updatedLangs = append(updatedLangs, "nn")
	}

	return true, updatedLangs
}

// Handle post processing logic
func (p *PostProcessor) processPost(evt *atproto.SyncSubscribeRepos_Commit, op *atproto.SyncSubscribeRepos_RepoOp, record *appbsky.FeedPost) error {
	uri := fmt.Sprintf("at://%s/%s", evt.Repo, op.Path)

	// 1. Check word count first (cheapest operation - just string splitting)
	words := strings.Fields(record.Text)
	if len(words) < 4 {
		log.Debugf("Skipping short post with only %d words: %s", len(words), uri)
		return nil
	}

	// 2. Filter out posts tagged with other languages (simple slice operation)
	if len(record.Langs) > 0 && !lo.Some(record.Langs, []string{"no", "nb", "nn", "se", "en"}) {
		log.Debugf("Skipping post with languages: %v", record.Langs)
		return nil
	}

	// 3. Check letter ratio (fast character counting)
	if !HasEnoughNorwegianLetters(record.Text) {
		log.Debugf("Skipping post with insufficient letter ratio: %s", uri)
		return nil
	}

	// 4. Check for repetitive patterns (string analysis)
	if ContainsRepetitivePattern(record.Text) {
		log.Debugf("Skipping post with repetitive pattern: %s", uri)
		return nil
	}

	// 5. Check for spam content (string matching)
	if ContainsSpamContent(record.Text) {
		log.Debugf("Skipping spam post: %s", uri)
		return nil
	}

	// 6. Language detection (most expensive operation)
	shouldProcess := false
	langs := record.Langs

	if p.detectFalseNegatives {
		shouldProcess, langs = p.DetectNorwegianLanguage(record.Text, record.Langs)
	} else if lo.Some(record.Langs, []string{"no", "nb", "nn", "se"}) {
		shouldProcess, langs = p.DetectNorwegianLanguage(record.Text, record.Langs)
	}

	if !shouldProcess {
		return nil
	}

	// Process the post
	p.ticker.Reset(5 * time.Minute)

	// Send sequence event
	p.postChan <- models.ProcessSeqEvent{
		Seq: evt.Seq,
	}

	// Parse and send create post event
	createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse creation time: %w", err)
	}

	p.postChan <- models.CreatePostEvent{
		Post: models.Post{
			Uri:       uri,
			CreatedAt: createdAt.Unix(),
			Text:      record.Text,
			Languages: langs,
		},
	}

	return nil
}

// Main event processor function is now more focused
func eventProcessor(postChan chan interface{}, context context.Context, ticker *time.Ticker, detectFalseNegatives bool, confidenceThreshold float64) *events.RepoStreamCallbacks {
	processor := &PostProcessor{
		postChan:             postChan,
		context:              context,
		ticker:               ticker,
		detectFalseNegatives: detectFalseNegatives,
		confidenceThreshold:  confidenceThreshold,
	}

	return &events.RepoStreamCallbacks{
		RepoCommit: func(evt *atproto.SyncSubscribeRepos_Commit) error {
			atomic.AddInt64(&processedEvents, 1)

			rr, err := repo.ReadRepoFromCar(context, bytes.NewReader(evt.Blocks))
			if err != nil {
				return fmt.Errorf("failed to read repo from car: %w", err)
			}

			for _, op := range evt.Ops {
				// Skip non-post operations
				if strings.Split(op.Path, "/")[0] != "app.bsky.feed.post" {
					continue
				}

				if op.Action != string(repomgr.EvtKindCreateRecord) &&
					op.Action != string(repomgr.EvtKindUpdateRecord) {
					continue
				}

				atomic.AddInt64(&processedPosts, 1)

				// Get and decode record
				_, rec, err := rr.GetRecord(context, op.Path)
				if err != nil {
					log.Warnf("Failed to get record: %v", err)
					continue
				}

				post := feedPostPool.Get().(*appbsky.FeedPost)
				defer feedPostPool.Put(post)

				// Reset post to clean state
				*post = appbsky.FeedPost{
					Langs: make([]string, 0, 4),
				}

				// Decode record
				decoder := lexutil.LexiconTypeDecoder{Val: rec}
				jsonRecord, err := decoder.MarshalJSON()
				if err != nil {
					log.Warnf("Failed to marshal record: %v", err)
					continue
				}

				if err := json.Unmarshal(jsonRecord, post); err != nil {
					log.Warnf("Failed to unmarshal record: %v", err)
					continue
				}

				if err := processor.processPost(evt, op, post); err != nil {
					log.Warnf("Failed to process post: %v", err)
				}
			}
			return nil
		},
	}
}

// GetDetector returns the package-level detector for testing
func GetDetector() lingua.LanguageDetector {
	return detector
}
