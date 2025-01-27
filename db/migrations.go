package db

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	log "github.com/sirupsen/logrus"
)

//go:embed migrations/*.sql
var fs embed.FS

// Migrate runs the PostgreSQL database migrations using golang-migrate
func Migrate(host string, port int, user, password, dbname string) error {
	log.Info("Running migrations...")

	// Create a new source instance using the embedded migrations
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// Build PostgreSQL connection URL
	connURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user, password, host, port, dbname)

	// Create a new migrate instance
	m, err := migrate.NewWithSourceInstance("iofs", d, connURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	// Run the migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info("Migrations completed successfully")
	return nil
}

// Rollback rolls back the last migration
func Rollback(host string, port int, user, password, dbname string) error {
	log.Info("Rolling back last migration...")

	// Create a new source instance using the embedded migrations
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// Build PostgreSQL connection URL
	connURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user, password, host, port, dbname)

	// Create a new migrate instance
	m, err := migrate.NewWithSourceInstance("iofs", d, connURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	// Roll back one step
	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	log.Info("Rollback completed successfully")
	return nil
}

// Force sets a specific version but doesn't run the migration
func Force(host string, port int, user, password, dbname string, version int) error {
	log.Infof("Forcing migration version to %d...", version)

	// Create a new source instance using the embedded migrations
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// Build PostgreSQL connection URL
	connURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user, password, host, port, dbname)

	// Create a new migrate instance
	m, err := migrate.NewWithSourceInstance("iofs", d, connURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	// Force version
	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force version: %w", err)
	}

	log.Infof("Successfully forced migration version to %d", version)
	return nil
}
