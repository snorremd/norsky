package feeds

import (
	"fmt"
	"strings"

	"norsky/config"
	"norsky/query"

	"github.com/huandu/go-sqlbuilder"
)

// BaseScoringStrategy provides common functionality for all scoring strategies
type BaseScoringStrategy struct {
	typeName string
}

// Name returns the strategy type name
func (s *BaseScoringStrategy) Name() string {
	return s.typeName
}

// NoScoring simply orders by ID
type NoScoring struct {
	BaseScoringStrategy
}

// NewNoScoring creates a new NoScoring strategy
func NewNoScoring() *NoScoring {
	return &NoScoring{
		BaseScoringStrategy: BaseScoringStrategy{typeName: "no_scoring"},
	}
}

// ApplyScoring adds no scoring to the query, accepts, but ignores weight
func (s *NoScoring) ApplyScoring(sb *sqlbuilder.SelectBuilder, alias string) {
	// We already have the post id and uri in the base query
}

func (s *NoScoring) GetSort() []string {
	return []string{"posts.id DESC"}
}

// TimeDecayScoring scores posts based on how recent they are
type TimeDecayScoring struct {
	BaseScoringStrategy
}

// NewTimeDecayScoring creates a new TimeDecayScoring strategy
func NewTimeDecayScoring() *TimeDecayScoring {
	return &TimeDecayScoring{
		BaseScoringStrategy: BaseScoringStrategy{typeName: "time_decay"},
	}
}

func (s *TimeDecayScoring) ApplyScoring(sb *sqlbuilder.SelectBuilder, alias string) {
	sb.SelectMore(fmt.Sprintf("(1.0 + (EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400.0))^(-0.5) AS %s", alias))
}

func (s *TimeDecayScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

// KeywordScoring scores posts based on keyword matches
type KeywordScoring struct {
	BaseScoringStrategy
	Keywords string
}

// NewKeywordScoring creates a new KeywordScoring strategy
func NewKeywordScoring(keywords string) *KeywordScoring {
	return &KeywordScoring{
		BaseScoringStrategy: BaseScoringStrategy{typeName: "keyword"},
		Keywords:            keywords,
	}
}

func (s *KeywordScoring) ApplyScoring(sb *sqlbuilder.SelectBuilder, alias string) {
	sb.SelectMore(fmt.Sprintf(
		`ts_rank(ts_vector, websearch_to_tsquery('simple', '%s'))/(1 + ts_rank(ts_vector, websearch_to_tsquery('simple', '%s'))) AS %s`,
		s.Keywords, s.Keywords, alias,
	))
}

func (s *KeywordScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

// AuthorScoring scores posts based on author weights
type AuthorScoring struct {
	BaseScoringStrategy
	Authors []config.TomlAuthor
}

// NewAuthorScoring creates a new AuthorScoring strategy
func NewAuthorScoring(authors []config.TomlAuthor) *AuthorScoring {
	return &AuthorScoring{
		BaseScoringStrategy: BaseScoringStrategy{typeName: "author"},
		Authors:             authors,
	}
}

func (s *AuthorScoring) ApplyScoring(sb *sqlbuilder.SelectBuilder, alias string) {
	// Create CASE statement for author scoring where default score is 1.0
	authorScores := make([]string, len(s.Authors))
	for i, author := range s.Authors {
		authorScores[i] = fmt.Sprintf(
			"CASE WHEN author_did = '%s' THEN %f ELSE 1.0 END",
			author.DID,
			author.Weight,
		)
	}

	// Multiply all author factors together
	sb.SelectMore(fmt.Sprintf("(%s) AS %s", strings.Join(authorScores, " * "), alias))
}

func (s *AuthorScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

var _ query.ScoringStrategy = (*NoScoring)(nil)
var _ query.ScoringStrategy = (*TimeDecayScoring)(nil)
var _ query.ScoringStrategy = (*KeywordScoring)(nil)
var _ query.ScoringStrategy = (*AuthorScoring)(nil)
