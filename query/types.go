package query

import (
	"github.com/huandu/go-sqlbuilder"
)

// Builder builds SQL queries for feed filtering and scoring
type Builder interface {
	Build(limit int, cursor int64) (string, []interface{})
}

// ScoringStrategy defines how posts should be scored/ranked
type ScoringStrategy interface {
	// ApplyScoring writes the scoring expression to the builder
	ApplyScoring(sb *sqlbuilder.SelectBuilder, alias string)
	// GetSort returns the ORDER BY clause
	GetSort() []string
	// Name returns a descriptive name for this scoring strategy
	Name() string
}

// FilterStrategy adds WHERE conditions to the query
type FilterStrategy interface {
	// ApplyFilter adds filter conditions to the query builder
	ApplyFilter(sb *sqlbuilder.SelectBuilder)
}

// KeywordConfig holds a named set of keywords
type KeywordConfig struct {
	Name     string
	Keywords []string
}
