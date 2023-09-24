package db

import (
	"database/sql"
	"time"

	sb "github.com/huandu/go-sqlbuilder"
	log "github.com/sirupsen/logrus"
)

// Tidy removes posts that are older than 90 days from the database
func Tidy(database string) error {
	db, err := connection(database)
	if err != nil {
		return err
	}

	return tidy(db)
}

func tidy(db *sql.DB) error {
	ninetyDaysAgo := time.Now().Add(90 * 24 * time.Hour).Unix()
	deletePosts := sb.NewDeleteBuilder()
	sql, args := deletePosts.DeleteFrom("posts").Where(deletePosts.LessEqualThan("created_at", ninetyDaysAgo)).Build()

	log.WithFields(log.Fields{
		"sql":  sql,
		"args": args,
	}).Info("Tidying database")

	// _, err := db.Exec(sql, args...)
	// if err != nil {
	// 	return err
	// }

	return nil
}
