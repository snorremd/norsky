package db

import (
	"norsky/models"
	"time"

	"database/sql"

	sqlbuilder "github.com/huandu/go-sqlbuilder"
	log "github.com/sirupsen/logrus"
)

type Writer struct {
	db       *sql.DB
	postChan chan interface{}
	tidyChan *time.Ticker
}

func NewWriter(database string, postChan chan interface{}) *Writer {
	db, err := connection(database)
	if err != nil {
		panic("failed to connect database")
	}
	return &Writer{
		db:       db,
		postChan: postChan,
		// Create new tidy channel that is pinged every 5 minutes
		tidyChan: time.NewTicker(5 * time.Minute),
	}
}

func (writer *Writer) Subscribe() {
	// Tidy database immediately
	if err := tidy(writer.db); err != nil {
		log.Error("Error tidying database", err)
	}

	for {
		select {
		case <-writer.tidyChan.C:
			log.Info("Tidying database")
			if err := tidy(writer.db); err != nil {
				log.Error("Error tidying database", err)
			}

		case post := <-writer.postChan:
			switch event := post.(type) {
			case models.ProcessSeqEvent:
				processSeq(writer.db, event)
			case models.CreatePostEvent:
				createPost(writer.db, event.Post)
			case models.DeletePostEvent:
				deletePost(writer.db, event.Post)
			default:
				log.Info("Unknown post type")
			}
		}

	}
}

func processSeq(db *sql.DB, evt models.ProcessSeqEvent) error {
	log.Info("Processing sequence")
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

func createPost(db *sql.DB, post models.Post) error {
	log.WithFields(log.Fields{
		"uri": post.Uri,
	}).Info("Creating post")
	// Post insert query
	insertPost := sqlbuilder.NewInsertBuilder()
	sql, args := insertPost.InsertInto("posts").Cols("uri", "created_at").Values(post.Uri, post.CreatedAt).Build()

	// Spread args
	res, err := db.Exec(sql, args...)
	if err != nil {
		log.Error("Error inserting post", err)
		return err
	}

	// Get inserted id
	id, err := res.LastInsertId()
	if err != nil {
		log.Error("Error getting inserted id", err)
		return err
	}

	// Post languages insert query
	insertLangs := sqlbuilder.NewInsertBuilder()
	insertLangs.InsertInto("post_languages").Cols("post_id", "language")
	for _, lang := range post.Languages {
		insertLangs.Values(id, lang)
	}
	sql, args = insertLangs.Build()

	_, err = db.Exec(sql, args...)

	if err != nil {
		log.Error("Error inserting languages", err)
		return err
	}

	return nil
}

func deletePost(db *sql.DB, post models.Post) error {
	log.Info("Deleting post")
	_, err := db.Exec("DELETE FROM posts WHERE uri = ?", post.Uri)
	if err != nil {
		return err
	}
	return nil
}
