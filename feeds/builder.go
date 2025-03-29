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
	// Simple case: no scoring layers
	if len(b.scoringLayers) == 0 {
		return b.buildSimpleQuery(limit, cursor)
	}

	// With scoring layers: use a subquery approach
	return b.buildScoredQuery(limit, cursor)
}

// buildSimpleQuery builds a query with no scoring complexity
func (b *FeedQueryBuilder) buildSimpleQuery(limit int, cursor int64) (string, []interface{}) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()

	// Add base columns
	sb.Select("posts.id", "posts.uri")
	sb.SelectMore("1.0 AS score")
	sb.From("posts")

	// Apply all filters
	for _, filter := range b.filters {
		filter.ApplyFilter(sb)
	}

	// Add cursor condition
	if cursor != 0 {
		sb.Where(sb.LessThan("posts.id", cursor))
	}

	// Always order by score and then by ID
	sb.OrderBy("score DESC", "posts.id DESC")
	sb.Limit(limit)

	return sb.Build()
}

// buildScoredQuery builds a query with complex scoring using a subquery
func (b *FeedQueryBuilder) buildScoredQuery(limit int, cursor int64) (string, []interface{}) {
	// Build the inner query with all individual scores as columns
	subquery := sqlbuilder.PostgreSQL.NewSelectBuilder()
	subquery.Select("posts.id", "posts.uri")

	// Add each scoring layer as a separate column with descriptive names
	scoreComponents := make([]string, 0, len(b.scoringLayers))
	for _, layer := range b.scoringLayers {
		// Get a descriptive column name from the strategy
		scoreName := fmt.Sprintf("score_%s", layer.strategy.Name())

		// Add this column to our tracking for the final score calculation
		scoreComponents = append(scoreComponents, scoreName)

		// Apply scoring with the descriptive name
		layer.strategy.ApplyScoring(subquery, scoreName)
	}

	subquery.From("posts")

	// Apply all filters to the subquery
	for _, filter := range b.filters {
		filter.ApplyFilter(subquery)
	}

	// Add cursor condition
	if cursor != 0 {
		subquery.Where(subquery.LessThan("posts.id", cursor))
	}

	// Get the SQL for the subquery
	subSql, subArgs := subquery.Build()

	// Build the outer query using the SQL builder
	outerQuery := sqlbuilder.PostgreSQL.NewSelectBuilder()
	outerQuery.Select("sq.id", "sq.uri")

	// Calculate the total weighted score
	var scoreTerms []string
	for i, scoreName := range scoreComponents {
		scoreTerms = append(scoreTerms, fmt.Sprintf("(%f * sq.%s)", b.scoringLayers[i].weight, scoreName))
	}
	outerQuery.SelectMore(fmt.Sprintf("(%s) AS score", strings.Join(scoreTerms, " + ")))

	// Use the subquery in the FROM clause
	outerQuery.From(fmt.Sprintf("(%s) AS sq", subSql))

	// Order by score and then by ID
	outerQuery.OrderBy("score DESC", "sq.id DESC")
	outerQuery.Limit(limit)

	// Build and return the final SQL with parameters
	outerSql, _ := outerQuery.Build()

	// Since our outer query doesn't have parameters of its own,
	// we only need to return the subquery parameters
	return outerSql, subArgs
}
