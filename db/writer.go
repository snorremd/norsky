package db

import (
	"context"
	"fmt"
	"norsky/models"
	"time"

	"database/sql"

	sqlbuilder "github.com/huandu/go-sqlbuilder"
	log "github.com/sirupsen/logrus"
)

func Subscribe(ctx context.Context, database string, postChan chan interface{}) {
	tidyChan := time.NewTicker(5 * time.Minute)

	db, err := connection(database)
	if err != nil {
		log.Error("Error connecting to database", err)
		ctx.Done()
	}

	// Tidy database immediately
	if err := tidy(db); err != nil {
		log.Error("Error tidying database", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping database writer")
			return
		case <-tidyChan.C:
			log.Info("Tidying database")
			if err := tidy(db); err != nil {
				log.Error("Error tidying database", err)
			}

		case post := <-postChan:
			switch event := post.(type) {
			case models.ProcessSeqEvent:
				processSeq(db, event)
			case models.CreatePostEvent:
				createPost(ctx, db, event.Post)
			case models.DeletePostEvent:
				deletePost(db, event.Post)
			default:
				log.Info("Unknown post type")
			}
		}

	}
}

func processSeq(db *sql.DB, evt models.ProcessSeqEvent) error {
	// Add context with timeout and retry logic for sequence updates
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update sequence row with new seq number
	updateSeq := sqlbuilder.NewUpdateBuilder()
	sql, args := updateSeq.Update("sequence").Set(updateSeq.Assign("seq", evt.Seq)).Where(updateSeq.Equal("id", 0)).Build()

	// Add retry logic with exponential backoff
	var err error
	for retries := 0; retries < 3; retries++ {
		_, err = db.ExecContext(ctx, sql, args...)
		if err == nil {
			return nil
		}

		// If database is locked, wait and retry
		if err.Error() == "database is locked" {
			time.Sleep(time.Duration(retries+1) * 100 * time.Millisecond)
			continue
		}

		// For other errors, return immediately
		return fmt.Errorf("error updating sequence: %w", err)
	}

	return fmt.Errorf("error updating sequence after retries: %w", err)
}

func createPost(ctx context.Context, db *sql.DB, post models.Post) (int64, error) {
	// Add timeout for the entire operation
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Start transaction with context
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("transaction error: %w", err)
	}
	defer tx.Rollback()

	log.WithFields(log.Fields{
		"uri":        post.Uri,
		"languages":  post.Languages,
		"created_at": time.Unix(post.CreatedAt, 0).Format(time.RFC3339),
		// Record lag from when the post was created to when it was processed
		"lagSeconds": time.Since(time.Unix(post.CreatedAt, 0)).Seconds(),
	}).Info("Creating post")

	// Post insert query
	insertPost := sqlbuilder.NewInsertBuilder()
	insertPost.InsertInto("posts").Cols("uri", "created_at", "text")
	insertPost.Values(post.Uri, post.CreatedAt, post.Text)

	sql, args := insertPost.Build()

	result, err := tx.Exec(sql, args...)
	if err != nil {
		return 0, fmt.Errorf("insert error: %w", err)
	}

	postId, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id error: %w", err)
	}

	// Insert languages
	for _, lang := range post.Languages {
		insertLang := sqlbuilder.NewInsertBuilder()
		insertLang.InsertInto("post_languages").Cols("post_id", "language")
		insertLang.Values(postId, lang)

		sql, args := insertLang.Build()
		if _, err := tx.Exec(sql, args...); err != nil {
			return 0, fmt.Errorf("language insert error: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit error: %w", err)
	}

	return postId, nil
}

func deletePost(db *sql.DB, post models.Post) error {
	log.Info("Deleting post")
	_, err := db.Exec("DELETE FROM posts WHERE uri = ?", post.Uri)
	if err != nil {
		return err
	}
	return nil
}
