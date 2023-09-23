package database

import (
	"embed"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var fs embed.FS

// Migrate runs the SQLite database migrations using golang-migrate
func Migrate() error {
	// Create a new source instance using the embedded migrations
	d, err := iofs.New(fs, "testdata/migrations")
	if err != nil {
		return err
	}

	// Create a new migrate instance using the iofs source instance and our SQLite database
	m, err := migrate.NewWithSourceInstance("iofs", d, "sqlite://feed.db")
	if err != nil {
		return err
	}

	// Run the migrations
	if err := m.Up(); err != nil {
		return err
	}

	return nil

}
