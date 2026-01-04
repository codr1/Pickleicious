// internal/db/db.go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// New opens a SQLite database for the given data source name, ensures SQLite
// foreign keys are enabled in the DSN, applies embedded migrations, and
// returns a DB with generated queries bound to the connection.
// It returns an error if opening the database or running migrations fails.
func New(dataSourceName string) (*DB, error) {
	dataSourceName = ensureForeignKeysEnabledDSN(dataSourceName)
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

// NewFromConfig creates a new DB instance from cfg by opening the configured database,
// applying migrations, and returning a DB with generated queries bound to the opened connection.
// It supports "sqlite" (creates the database directory if needed and ensures foreign keys are enabled in the DSN)
// and "turso" (constructs a libsql connection string with the provided auth token).
// Returns an error if the driver is unsupported, if opening the database fails, or if migrations cannot be applied.
func NewFromConfig(cfg *config.Config) (*DB, error) {
	var db *sql.DB
	var err error

	switch cfg.Database.Driver {
	case "sqlite":
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(cfg.Database.Filename), 0755); err != nil {
			return nil, fmt.Errorf("error creating database directory: %w", err)
		}
		dataSourceName := ensureForeignKeysEnabledDSN(cfg.Database.Filename)
		db, err = sql.Open("sqlite3", dataSourceName)

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

// ensureForeignKeysEnabledDSN ensures the SQLite DSN enables foreign key enforcement by adding the `_fk=1` query parameter if missing.
// If the DSN already contains `_fk=` it is returned unchanged; otherwise `_fk=1` is appended using `?` or `&` as appropriate.
func ensureForeignKeysEnabledDSN(dataSourceName string) string {
	if strings.Contains(dataSourceName, "_fk=") {
		return dataSourceName
	}
	if strings.Contains(dataSourceName, "?") {
		return dataSourceName + "&_fk=1"
	}
	return dataSourceName + "?_fk=1"
}

// runMigrations applies the embedded SQL migrations from migrationsFS to the provided database.
// It returns an error if creating the migration driver, source, or migrate instance fails, or if applying migrations fails (a "no change" result is not treated as an error).
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