package feeds

import (
	"fmt"
	"strings"

	"norsky/config"
	"norsky/query"
)

// NoScoring simply orders by ID
type NoScoring struct{}

// ApplyScoring adds no scoring to the query, accepts, but ignores weight
func (s *NoScoring) ApplyScoring(sb *strings.Builder) {
	// We already have the post id and uri in the base query
}

func (s *NoScoring) GetSort() []string {
	return []string{"posts.id DESC"}
}

// TimeDecayScoring scores posts based on how recent they are
type TimeDecayScoring struct{}

func (s *TimeDecayScoring) ApplyScoring(sb *strings.Builder) {
	sb.WriteString("(1.0 + (EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400.0))^(-0.5)")
}

func (s *TimeDecayScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

// KeywordScoring scores posts based on keyword matches
type KeywordScoring struct {
	Keywords string
}

func (s *KeywordScoring) ApplyScoring(sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf(
		`ts_rank(ts_vector, websearch_to_tsquery('simple', '%s'))/(1 + ts_rank(ts_vector, websearch_to_tsquery('simple', '%s')))`,
		s.Keywords, s.Keywords,
	))
}

func (s *KeywordScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

// AuthorScoring scores posts based on author weights
type AuthorScoring struct {
	Authors []config.TomlAuthor
}

func (s *AuthorScoring) ApplyScoring(sb *strings.Builder) {
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
	sb.WriteString("(" + strings.Join(authorScores, " * ") + ")")
}

func (s *AuthorScoring) GetSort() []string {
	return []string{"score DESC", "posts.id DESC"}
}

var _ query.ScoringStrategy = (*NoScoring)(nil)
var _ query.ScoringStrategy = (*TimeDecayScoring)(nil)
var _ query.ScoringStrategy = (*KeywordScoring)(nil)
var _ query.ScoringStrategy = (*AuthorScoring)(nil)
