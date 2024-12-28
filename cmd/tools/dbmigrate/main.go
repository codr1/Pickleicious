// cmd/tools/dbmigrate/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var (
		dbPath         = flag.String("db", "", "Path to SQLite database")
		migrationsPath = flag.String("migrations", "", "Path to migrations directory")
		command        = flag.String("command", "", "Command to run (up, down, version)")
	)
	flag.Parse()

	// Validate flags
	if *dbPath == "" || *migrationsPath == "" || *command == "" {
		log.Println("All flags are required:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Convert paths to absolute
	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		log.Fatalf("Invalid database path: %v", err)
	}

	absMigrations, err := filepath.Abs(*migrationsPath)
	if err != nil {
		log.Fatalf("Invalid migrations path: %v", err)
	}

	// Ensure migrations directory exists
	if _, err := os.Stat(absMigrations); os.IsNotExist(err) {
		log.Fatalf("Migrations directory does not exist: %s", absMigrations)
	}

	// Create database directory if it doesn't exist
	dbDir := filepath.Dir(absDB)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	// Create migration instance
	sourceURL := fmt.Sprintf("file://%s", absMigrations)
	databaseURL := fmt.Sprintf("sqlite3://%s", absDB)

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	// Execute command
	switch *command {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		log.Println("Successfully ran migrations up")

	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
		log.Println("Successfully ran migrations down")

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		log.Printf("Current version: %d, Dirty: %v\n", version, dirty)

	default:
		log.Fatalf("Unknown command: %s", *command)
	}
}
