package db

import (
	"database/sql"
	"fmt"
)

func connection(database string) (*sql.DB, error) {
	return sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", database))
}
