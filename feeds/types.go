// Package feeds provides structured feed types and their implementations
package feeds

import (
	"norsky/db"
)

// FeedMap maps feed IDs to their Feed instances
type FeedMap map[string]*Feed

// Feed represents a runtime feed instance
type Feed struct {
	// Metadata
	ID          string
	DisplayName string
	Description string
	AvatarPath  string

	// Runtime dependencies
	DB      *db.DB
	builder *FeedQueryBuilder
}
