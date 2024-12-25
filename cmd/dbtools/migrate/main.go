// cmd/dbtools/migrate/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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

	if *dbPath == "" || *migrationsPath == "" || *command == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Ensure migrations directory exists
	if err := os.MkdirAll(*migrationsPath, 0755); err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	// Create database URL
	dbURL := fmt.Sprintf("sqlite3://%s", *dbPath)

	// Initialize migrate
	m, err := migrate.New(
		fmt.Sprintf("file://%s", *migrationsPath),
		dbURL,
	)
	if err != nil {
		log.Fatalf("Migration init failed: %v", err)
	}
	defer m.Close()

	// Execute command
	switch *command {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration up failed: %v", err)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration down failed: %v", err)
		}
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("Get version failed: %v", err)
		}
		fmt.Printf("Version: %d, Dirty: %v\n", version, dirty)
	default:
		log.Fatalf("Unknown command: %s", *command)
	}
}
