package db

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var fs embed.FS

// Migrate runs the SQLite database migrations using golang-migrate
func Migrate(dbPath string) error {
	fmt.Println("Running migrations...")
	// Create a new source instance using the embedded migrations
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}

	// Create a new migrate instance using the iofs source instance and our SQLite database
	m, err := migrate.NewWithSourceInstance("iofs", d, "sqlite://"+dbPath)
	if err != nil {
		fmt.Println("Error creating migrate instance", err)
		return err
	}

	// Run the migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
