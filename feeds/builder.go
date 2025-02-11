package feeds

import (
	"fmt"
	"norsky/query"
	"strings"

	"github.com/huandu/go-sqlbuilder"
)

// FeedQueryBuilder builds feed queries with scoring and filters
type FeedQueryBuilder struct {
	scoringLayers []scoringLayer
	filters       []query.FilterStrategy
}

type scoringLayer struct {
	strategy query.ScoringStrategy
	weight   float64
}

func NewFeedQueryBuilder() *FeedQueryBuilder {
	return &FeedQueryBuilder{
		scoringLayers: make([]scoringLayer, 0),
		filters:       make([]query.FilterStrategy, 0),
	}
}

func (b *FeedQueryBuilder) AddScoringLayer(strategy query.ScoringStrategy, weight float64) {
	b.scoringLayers = append(b.scoringLayers, scoringLayer{
		strategy: strategy,
		weight:   weight,
	})
}

func (b *FeedQueryBuilder) AddFilter(filter query.FilterStrategy) {
	b.filters = append(b.filters, filter)
}

func (b *FeedQueryBuilder) Build(limit int, cursor int64) (string, []interface{}) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()

	// Add base columns
	sb.Select("posts.id", "posts.uri")

	// Calculate final score if we have scoring layers
	if len(b.scoringLayers) > 0 {
		var scoreTerms []string

		// Get each scoring expression and apply weight
		for _, layer := range b.scoringLayers {
			// Get the scoring expression from the strategy without an alias
			var scoreExpr strings.Builder
			layer.strategy.ApplyScoring(&scoreExpr)

			// Add the weighted score term
			scoreTerms = append(scoreTerms, fmt.Sprintf("(%f * (%s))", layer.weight, scoreExpr.String()))
		}

		// Multiply all scores together for final score
		sb.SelectMore(fmt.Sprintf("(%s) AS score", strings.Join(scoreTerms, " + ")))
	}

	sb.From("posts")

	// Apply all filters
	for _, filter := range b.filters {
		filter.ApplyFilter(sb)
	}

	// Add cursor condition
	if cursor != 0 {
		sb.Where(sb.LessThan("posts.id", cursor))
	}

	// Order by score if we have scoring layers, otherwise by time
	if len(b.scoringLayers) > 0 {
		sb.OrderBy("score DESC", "posts.id DESC")
	} else {
		sb.OrderBy("posts.id DESC")
	}

	sb.Limit(limit)

	return sb.Build()
}
