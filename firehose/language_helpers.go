package firehose

import (
	"strings"

	lingua "github.com/pemistahl/lingua-go"
)

func NewLanguageDetector(targetLangs []lingua.Language) lingua.LanguageDetector {
	// Always include English plus target languages
	languages := lingua.AllLanguages()

	return lingua.NewLanguageDetectorBuilder().
		FromLanguages(languages...).
		WithMinimumRelativeDistance(0.25).
		Build()
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

func targetLanguagesToLingua(languages []string) []lingua.Language {
	linguaLanguages := []lingua.Language{}

	for _, lang := range languages {
		linguaLang, ok := isoToLingua(lang, getSupportedLanguages())
		if ok {
			linguaLanguages = append(linguaLanguages, linguaLang)
		}
	}

	return linguaLanguages
}
