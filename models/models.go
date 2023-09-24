package models

// Post model with key fields from the post
type Post struct {
	Id        int64    `json:"id"`
	Uri       string   `json:"post"`
	CreatedAt int64    `json:"createdAt"` // Unix timestamp
	Text      string   `json:"text"`      // We don't want to store the text in the database
	Languages []string `json:"languages"`
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
	Feed   []Post  `json:"feed"`
	Cursor *string `json:"cursor"`
}
