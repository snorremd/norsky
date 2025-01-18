package models

import "time"

// Post model with key fields from the post
type Post struct {
	Id        int64    `json:"id"`
	CreatedAt int64    `json:"createdAt"`
	Text      string   `json:"text"`
	Languages []string `json:"languages"`
	Uri       string   `json:"uri"`
	ParentUri string   `json:"parentUri,omitempty"`
}

// Omit all but the Uri field
type FeedPost struct {
	Id  int64  `json:"-"`
	Uri string `json:"post"`
}

type ProcessSeqEvent struct {
	Seq int64
}

// CreateEvent fired when a new post is created
type CreatePostEvent struct {
	Post Post
}

// UpdateEvent fired when a post is updated
type UpdatePostEvent struct {
	Post Post
}

// DeleteEvent fired when a post is deleted
type DeletePostEvent struct {
	Post Post
}

type FeedResponse struct {
	Feed   []FeedPost `json:"feed"`
	Cursor *string    `json:"cursor"`
}

type PostsAggregatedByTime struct {
	Time  time.Time `json:"time"`
	Count int64     `json:"count"`
}
