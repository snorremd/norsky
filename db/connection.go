package db

import (
	"database/sql"
	"fmt"
	"time"
)

func connection(database string) (*sql.DB, error) {
	// Enable foreign keys and WAL mode
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", database))
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1)            // SQLite only supports one writer at a time
	db.SetMaxIdleConns(1)            // Keep one connection in the pool
	db.SetConnMaxLifetime(time.Hour) // Recreate connections after an hour
	db.SetConnMaxIdleTime(time.Hour) // Close idle connections after an hour

	// Configure some additional pragmas for better performance
	if _, err := db.Exec(`
		PRAGMA busy_timeout = 5000;
		PRAGMA synchronous = NORMAL;
		PRAGMA cache_size = -32000; -- 32MB cache
		PRAGMA temp_store = MEMORY;
		PRAGMA page_size = 4096;     -- Optimal page size for most systems
		PRAGMA mmap_size = 268435456; -- 256MB memory mapped I/O
	`); err != nil {
		return nil, fmt.Errorf("failed to set pragmas: %w", err)
	}

	return db, nil
}
