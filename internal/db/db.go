// internal/db/db.go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/codr1/Pickleicious/internal/config"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	*sql.DB
	Queries *dbgen.Queries
}

// New creates a new DB instance with the given data source name
func New(dataSourceName string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	// Run migrations
	if err := runMigrations(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("error running migrations: %w", err)
	}

	// Create queries
	queries := dbgen.New(sqlDB)

	return &DB{
		DB:      sqlDB,
		Queries: queries,
	}, nil
}

// NewFromConfig creates a new DB instance from configuration
func NewFromConfig(cfg *config.Config) (*DB, error) {
	var db *sql.DB
	var err error

	switch cfg.Database.Driver {
	case "sqlite":
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(cfg.Database.Filename), 0755); err != nil {
			return nil, fmt.Errorf("error creating database directory: %w", err)
		}
		db, err = sql.Open("sqlite3", cfg.Database.Filename)

	case "turso":
		// Example for future Turso support
		connector := fmt.Sprintf("%s?authToken=%s", cfg.Database.URL, cfg.Database.AuthToken)
		db, err = sql.Open("libsql", connector)

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}

	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("error running migrations: %w", err)
	}

	queries := dbgen.New(db)
	return &DB{
		DB:      db,
		Queries: queries,
	}, nil
}

func runMigrations(db *sql.DB) error {
	// Create migrate instance
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("could not create migrate driver: %w", err)
	}

	// Create source instance
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("could not create source: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithInstance(
		"iofs", source,
		"sqlite3", driver,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("could not run migrations: %w", err)
	}

	return nil
}

// WithTx creates a new DB instance with the given transaction
func (db *DB) WithTx(tx *sql.Tx) *DB {
	return &DB{
		DB:      db.DB,
		Queries: dbgen.New(tx),
	}
}

// BeginTx starts a transaction
func (db *DB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %w", err)
	}
	return tx, nil
}

// RunInTx runs the given function in a transaction
func (db *DB) RunInTx(ctx context.Context, fn func(*DB) error) error {
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	txDB := db.WithTx(tx)
	if err := fn(txDB); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("error rolling back: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing: %w", err)
	}

	return nil
}
