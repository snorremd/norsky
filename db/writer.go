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
				createPost(db, event.Post)
			case models.DeletePostEvent:
				deletePost(db, event.Post)
			default:
				log.Info("Unknown post type")
			}
		}

	}
}

func processSeq(db *sql.DB, evt models.ProcessSeqEvent) error {
	// Update sequence row with new seq number
	updateSeq := sqlbuilder.NewUpdateBuilder()
	sql, args := updateSeq.Update("sequence").Set(updateSeq.Assign("seq", evt.Seq)).Where(updateSeq.Equal("id", 0)).Build()

	_, err := db.Exec(sql, args...)
	if err != nil {
		log.Error("Error updating sequence", err)
		return err
	}

	return nil
}

func createPost(db *sql.DB, post models.Post) (int64, error) {
	log.WithFields(log.Fields{
		"uri":        post.Uri,
		"languages":  post.Languages,
		"created_at": time.Unix(post.CreatedAt, 0).Format(time.RFC3339),
		// Record lag from when the post was created to when it was processed
		"lagSeconds": time.Since(time.Unix(post.CreatedAt, 0)).Seconds(),
	}).Info("Creating post")

	// Start a transaction since we need to insert into multiple tables
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("transaction error: %w", err)
	}
	defer tx.Rollback()

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
