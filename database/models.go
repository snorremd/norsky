package database

// define struct Post

type Post struct {
	Uri       string
	CreatedAt string
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
