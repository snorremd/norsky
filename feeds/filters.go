package feeds

import (
	"fmt"
	"strings"

	"norsky/query"

	"github.com/huandu/go-sqlbuilder"
)

// LanguageFilter filters posts by language
type LanguageFilter struct {
	Languages []string
}

func (f *LanguageFilter) ApplyFilter(sb *sqlbuilder.SelectBuilder) {
	if len(f.Languages) > 0 {
		// Create a PostgreSQL array literal string directly
		// This avoids the parameter binding issue with arrays and subqueries
		var escapedLangs []string
		for _, lang := range f.Languages {
			// Escape single quotes in language strings by doubling them
			escapedLang := strings.ReplaceAll(lang, "'", "''")
			escapedLangs = append(escapedLangs, fmt.Sprintf("'%s'", escapedLang))
		}

		// Create the array literal: ARRAY['lang1', 'lang2', ...]
		arrayLiteral := fmt.Sprintf("ARRAY[%s]", strings.Join(escapedLangs, ", "))

		// Add the condition with the literal array
		sb.Where(fmt.Sprintf("languages && %s", arrayLiteral))
	}
}

// ExcludeRepliesFilter filters out reply posts
type ExcludeRepliesFilter struct{}

func (f *ExcludeRepliesFilter) ApplyFilter(sb *sqlbuilder.SelectBuilder) {
	sb.Where(sb.IsNull("posts.parent_uri"))
}

// KeywordFilter filters posts based on included and excluded keywords
type KeywordFilter struct {
	IncludeKeywords string
	ExcludeKeywords string
}

func (f *KeywordFilter) ApplyFilter(sb *sqlbuilder.SelectBuilder) {
	// Add include keywords condition if specified
	if f.IncludeKeywords != "" {
		sb.Where(fmt.Sprintf(
			"ts_vector @@ websearch_to_tsquery('simple', '%s')",
			f.IncludeKeywords,
		))
	}

	// Add exclude keywords condition if specified
	if f.ExcludeKeywords != "" {
		sb.Where(fmt.Sprintf(
			"NOT (ts_vector @@ websearch_to_tsquery('simple', '%s'))",
			f.ExcludeKeywords,
		))
	}
}

var _ query.FilterStrategy = (*LanguageFilter)(nil)
var _ query.FilterStrategy = (*ExcludeRepliesFilter)(nil)
var _ query.FilterStrategy = (*KeywordFilter)(nil)
