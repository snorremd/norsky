package firehose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bluesky-social/indigo/api/bsky"
	jetstream_models "github.com/bluesky-social/jetstream/pkg/models"
	"github.com/klauspost/compress/zstd"
	lingua "github.com/pemistahl/lingua-go"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"

	"norsky/db"
	norsky_models "norsky/models"
)

type PostProcessor struct {
	postChan           chan interface{}
	context            context.Context
	config             FirehoseConfig
	decoder            *zstd.Decoder
	targetLanguages    []lingua.Language
	supportedLanguages map[lingua.Language]string
	languageDetector   lingua.LanguageDetector
	db                 *db.DB
}

func NewPostProcessor(ctx context.Context, config FirehoseConfig, db *db.DB) *PostProcessor {
	pp := &PostProcessor{
		context:            ctx,
		config:             config,
		targetLanguages:    targetLanguagesToLingua(config.Languages),
		supportedLanguages: getSupportedLanguages(),
		languageDetector:   NewLanguageDetector(targetLanguagesToLingua(config.Languages)),
		db:                 db,
	}

	if config.JetstreamCompress {
		decoder, err := zstd.NewReader(nil, zstd.WithDecoderDicts(jetstream_models.ZSTDDictionary))
		if err != nil {
			log.Fatalf("Failed to create zstd decoder: %v", err)
		}
		pp.decoder = decoder
	}

	return pp
}

// Handle post processing logic
func (p *PostProcessor) processPost(msg *RawMessage) error {
	var data []byte
	var err error

	// If message is compressed (binary), decompress it first
	if p.decoder != nil {
		data, err = p.decoder.DecodeAll(msg.Data, nil)
		if err != nil {
			return fmt.Errorf("failed to decompress message: %w", err)
		}
	} else {
		data = msg.Data
	}

	// Parse the raw message into a Jetstream event
	var event jetstream_models.Event
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// If it is not a create post commit operation we skip it
	if event.Commit == nil ||
		event.Commit.Operation != jetstream_models.CommitOperationCreate ||
		event.Commit.Collection != "app.bsky.feed.post" {
		return nil
	}

	// Get the post record unmarshalled
	var record bsky.FeedPost
	if err := json.Unmarshal(event.Commit.Record, &record); err != nil {
		return fmt.Errorf("failed to unmarshal post: %w", err)
	}

	// Get URI
	uri := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", event.Did, event.Commit.RKey)

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

	// Parse and send create post event
	createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse creation time: %w", err)
	}

	// Extract parent URI if this is a reply
	var parentUri string
	if record.Reply != nil && record.Reply.Parent != nil {
		parentUri = record.Reply.Parent.Uri
	}

	log.WithFields(log.Fields{
		"uri":       uri,
		"createdAt": createdAt.Unix(),
		"text":      record.Text,
		"languages": langs,
		"parentUri": parentUri,
	}).Info("Adding post to database")

	// Create post object
	post := norsky_models.Post{
		Uri:       uri,
		CreatedAt: createdAt.Unix(),
		Text:      record.Text,
		Languages: langs,
		ParentUri: parentUri,
	}

	// Write directly to database instead of using channel
	if err := p.db.CreatePost(p.context, post); err != nil {
		return fmt.Errorf("failed to create post in database: %w", err)
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

// Rename to DetectLanguage since it's no longer Norwegian-specific
func (p *PostProcessor) DetectLanguage(text string, currentLangs []string, targetLangs []lingua.Language) (bool, []string) {
	// First check English confidence separately
	englishConf := p.languageDetector.ComputeLanguageConfidence(text, lingua.English)

	// If text is primarily English (high confidence), skip it unless English is a target language
	if englishConf > 0.8 && !lo.Contains(targetLangs, lingua.English) {
		return false, currentLangs
	}

	var highestConf float64
	var detectedLang lingua.Language

	// Only check target languages
	for _, lang := range targetLangs {
		conf := p.languageDetector.ComputeLanguageConfidence(text, lang)
		if conf > highestConf {
			highestConf = conf
			detectedLang = lang
		}
	}

	// If confidence is too low, skip
	if highestConf < p.config.ConfidenceThreshold {
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
