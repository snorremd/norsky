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
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"log/slog"

	"net"

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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

// Some constants to optimize the firehose

// Allow these to grow to 10MB
const (
	wsReadBufferSize  = 1024 * 1024 * 2  // 2MB
	wsWriteBufferSize = 1024 * 1024 * 2  // 2MB
	wsReadTimeout     = 90 * time.Second // Increased from 60s
	wsWriteTimeout    = 15 * time.Second // Increased from 10s
	wsPingInterval    = 30 * time.Second // Reduced from 60s
)

// Add these metrics
var (
	wsMessageProcessingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "norsky_ws_message_processing_seconds",
		Help:    "Time spent processing websocket messages",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
	})

	wsMessageBacklog = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "norsky_ws_message_backlog",
		Help: "Number of messages waiting to be processed",
	})
)

func init() {
	prometheus.MustRegister(wsMessageProcessingTime)
	prometheus.MustRegister(wsMessageBacklog)
}

func NewLanguageDetector(targetLangs []lingua.Language) lingua.LanguageDetector {
	// Always include English plus target languages
	languages := lingua.AllLanguages()

	return lingua.NewLanguageDetectorBuilder().
		FromLanguages(languages...).
		WithMinimumRelativeDistance(0.25).
		Build()
}

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
func HasEnoughLetters(text string) bool {
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
		return true
	}

	// If more than 5 mentions, consider it spam
	if mentionCount > 5 {
		return true
	}

	// Check for repeated hashtags or mentions (common spam pattern)
	if strings.Count(text, "##") > 0 || strings.Count(text, "@@") > 0 {
		return true
	}

	// Check for hashtag and mention ratios
	words := strings.Fields(text)
	if len(words) > 0 {
		// Calculate combined ratio of hashtags and mentions
		symbolRatio := float64(hashtagCount+mentionCount) / float64(len(words))
		// If more than 50% of words are hashtags or mentions combined, consider it spam
		if symbolRatio > 0.5 {
			return true
		}
	}

	return false
}

// FirehoseConfig holds configuration for the firehose processing
type FirehoseConfig struct {
	RunLanguageDetection bool
	ConfidenceThreshold  float64
	Languages            []string
}

// Subscribe to the firehose using the Firehose struct as a receiver
func Subscribe(ctx context.Context, postChan chan interface{}, ticker *time.Ticker, seq int64, config FirehoseConfig) {

	address := "wss://bsky.network/xrpc/com.atproto.sync.subscribeRepos"
	headers := http.Header{}
	headers.Set("User-Agent", "NorSky: https://github.com/snorremd/norsky")
	headers.Set("Accept-Encoding", "gzip")

	if seq >= 0 {
		log.Info("Starting from sequence: ", seq)
		address = fmt.Sprintf("%s?cursor=%d", address, seq)
	}
	// Identify dialer with User-Agent header

	dialer := websocket.Dialer{
		ReadBufferSize:   wsReadBufferSize,
		WriteBufferSize:  wsWriteBufferSize,
		HandshakeTimeout: 45 * time.Second,
		NetDialContext: (&net.Dialer{
			Timeout:   45 * time.Second,
			KeepAlive: 45 * time.Second,
		}).DialContext,
	}

	backoff := backoff.NewExponentialBackOff()
	backoff.InitialInterval = 100 * time.Millisecond
	backoff.MaxInterval = 30 * time.Second
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
			conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
			conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))

			// Start ping ticker with shorter interval
			pingTicker := time.NewTicker(wsPingInterval)
			defer pingTicker.Stop()

			// Update ping goroutine
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-pingTicker.C:
						log.Debug("Sending ping to check connection")
						if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(wsWriteTimeout)); err != nil {
							log.Warn("Ping failed, closing connection for restart: ", err)
							conn.Close()
							return
						}
						// Reset read deadline after successful ping
						if err := conn.SetReadDeadline(time.Now().Add(wsReadTimeout)); err != nil {
							log.Warn("Failed to set read deadline, closing connection: ", err)
							conn.Close()
							return
						}
					}
				}
			}()

			// Add connection close handler
			conn.SetCloseHandler(func(code int, text string) error {
				log.Infof("WebSocket connection closed with code %d: %s", code, text)
				return nil
			})

			// Remove pong handler since server doesn't respond
			// Keep ping handler for completeness
			conn.SetPingHandler(func(appData string) error {
				log.Debug("Received ping from server")
				return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			})

			scheduler := sequential.NewScheduler(
				//runtime.NumCPU(),
				//100,
				// autoscaling.AutoscaleSettings{

				// 	MaxConcurrency:           runtime.NumCPU() * 2,
				// 	Concurrency:              runtime.NumCPU() * 2,
				// 	AutoscaleFrequency:       10 * time.Second,
				// 	ThroughputBucketDuration: 2 * time.Second,
				// 	ThroughputBucketCount:    15,
				// },
				conn.RemoteAddr().String(),
				eventProcessor(postChan, ctx, ticker, config).EventHandler)

			err = events.HandleRepoStream(ctx, conn, scheduler, slog.Default())

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

// Add new types to help organize the code
type PostProcessor struct {
	postChan           chan interface{}
	context            context.Context
	ticker             *time.Ticker
	config             FirehoseConfig
	targetLanguages    []lingua.Language
	supportedLanguages map[lingua.Language]string
	languageDetector   lingua.LanguageDetector // We should have one detector per worker
}

// Rename to DetectLanguage since it's no longer Norwegian-specific
func (p *PostProcessor) DetectLanguage(text string, currentLangs []string, targetLangs []lingua.Language) (bool, []string) {
	var highestConf float64
	var detectedLang lingua.Language

	// Check confidence for English and all target languages
	for _, lang := range append([]lingua.Language{lingua.English}, targetLangs...) {
		conf := p.languageDetector.ComputeLanguageConfidence(text, lang)
		if conf > highestConf {
			highestConf = conf
			detectedLang = lang
		}
	}

	// If confidence is too low or detected language is English, skip
	if highestConf < p.config.ConfidenceThreshold || detectedLang == lingua.English {
		return false, currentLangs
	}

	log.Infof("%s confidence: %.2f (threshold: %.2f)",
		detectedLang.String(), highestConf, p.config.ConfidenceThreshold)

	// Create new slice to avoid modifying the input
	updatedLangs := make([]string, len(currentLangs))
	copy(updatedLangs, currentLangs)

	// Map lingua language to ISO code
	langCode := linguaToISO(detectedLang, p.supportedLanguages)
	if langCode != "" && !lo.Contains(updatedLangs, langCode) {
		updatedLangs = append(updatedLangs, langCode)
	}

	return true, updatedLangs
}

// Add helper function to map lingua languages to ISO codes
func linguaToISO(lang lingua.Language, languages map[lingua.Language]string) string {
	if code, ok := languages[lang]; ok {
		return code
	}
	return ""
}

// Add helper function to map ISO codes to lingua languages
func isoToLingua(code string, languages map[lingua.Language]string) (lingua.Language, bool) {
	for lang, isoCode := range languages {
		if isoCode == code {
			return lang, true
		}
	}
	return lingua.Unknown, false
}

// Handle post processing logic
func (p *PostProcessor) processPost(evt *atproto.SyncSubscribeRepos_Commit, op *atproto.SyncSubscribeRepos_RepoOp, record *appbsky.FeedPost) error {
	// Get URI
	uri := fmt.Sprintf("at://%s/%s", evt.Repo, op.Path)

	words := strings.Fields(record.Text)
	if len(words) < 4 {
		return nil
	}

	if !HasEnoughLetters(record.Text) {
		return nil
	}

	if ContainsRepetitivePattern(record.Text) {
		return nil
	}

	if ContainsSpamContent(record.Text) {
		return nil
	}

	// 6. Language detection (most expensive operation)
	shouldProcess := false
	langs := record.Langs

	if p.config.RunLanguageDetection {
		shouldProcess, langs = p.DetectLanguage(record.Text, record.Langs, p.targetLanguages)
	} else {
		// When not running language detection, check if:
		// 1. Post has no language tags (accept all) OR
		// 2. Post has language tags that match our target languages
		targetCodes := p.getTargetIsoCodes()
		shouldProcess = len(record.Langs) > 0 && lo.Some(record.Langs, targetCodes)
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

// Helper method to get ISO codes from target languages
func (p *PostProcessor) getTargetIsoCodes() []string {
	codes := make([]string, 0, len(p.targetLanguages))
	for _, lang := range p.targetLanguages {
		if code := linguaToISO(lang, p.supportedLanguages); code != "" {
			codes = append(codes, code)
		}
	}
	return codes
}

// Main event processor function is now more focused
func eventProcessor(postChan chan interface{}, context context.Context, ticker *time.Ticker, config FirehoseConfig) *events.RepoStreamCallbacks {
	var targetLangs []lingua.Language
	supportedLangs := getSupportedLanguages()

	if len(config.Languages) == 0 {
		// If no languages specified, use all supported languages
		targetLangs = make([]lingua.Language, 0, len(supportedLangs))
		for lang := range supportedLangs {
			targetLangs = append(targetLangs, lang)
		}
		log.Info("No specific languages configured, detecting all supported languages")
	} else {
		// Convert ISO codes to lingua languages
		targetLangs = make([]lingua.Language, 0, len(config.Languages))
		for _, code := range config.Languages {
			if lang, ok := isoToLingua(code, supportedLangs); ok {
				targetLangs = append(targetLangs, lang)
			}
		}
		log.Infof("Detecting configured languages: %v", targetLangs)
	}

	processor := &PostProcessor{
		postChan:           postChan,
		context:            context,
		ticker:             ticker,
		config:             config,
		targetLanguages:    targetLangs,
		supportedLanguages: supportedLangs,
		languageDetector:   NewLanguageDetector(targetLangs),
	}

	return &events.RepoStreamCallbacks{
		RepoCommit: func(evt *atproto.SyncSubscribeRepos_Commit) error {
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

// Rename and modify the function to just get supported languages
func getSupportedLanguages() map[lingua.Language]string {
	languages := make(map[lingua.Language]string)

	// Map all lingua languages to their ISO 639-1 codes
	for _, lang := range lingua.AllLanguages() {
		isoCode := strings.ToLower(lang.IsoCode639_1().String())
		languages[lang] = isoCode
	}

	return languages
}
