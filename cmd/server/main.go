// cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/codr1/Pickleicious/internal/api/members"
	"github.com/codr1/Pickleicious/internal/config"
	"github.com/codr1/Pickleicious/internal/db"
)

func setupLogger(environment string) {
	// Set time format for all logging
	zerolog.TimeFieldFormat = "15:04:05"

	// Create console writer with colors and better formatting
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}

	if environment == "development" {
		// Set global log level to debug
		zerolog.SetGlobalLevel(zerolog.DebugLevel)

		// Set global logger with debug settings
		log.Logger = zerolog.New(consoleWriter).
			With().
			Timestamp().
			Caller().
			Logger()

		// Test debug logging
		log.Debug().Msg("Debug logging enabled")
	} else {
		// Production settings
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Logger = zerolog.New(consoleWriter).
			With().
			Timestamp().
			Logger()
	}

	// Override the global logger to ensure it's used everywhere
	zerolog.DefaultContextLogger = &log.Logger
}

func main() {
	config, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	setupLogger(config.App.Environment)

	// Initialize database
	database, err := db.NewFromConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer database.Close()

	// Verify database connection
	if err := database.DB.Ping(); err != nil {
		log.Fatal().Err(err).Msg("Failed to ping database")
	}
	log.Info().Msg("Database connection successful")

	// Initialize handlers with database queries
	members.InitHandlers(database.Queries)
	log.Info().Msg("Member handlers initialized with database queries")

	// Verify members table exists and has data
	var count int
	err = database.DB.QueryRow("SELECT COUNT(*) FROM members").Scan(&count)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to query members table")
	}
	log.Info().Int("member_count", count).Msg("Found members in database")

	// Create server instance
	server := newServer(config)

	// Setup graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	// Run server
	g.Go(func() error {
		log.Info().Int("port", config.App.Port).Msg("Starting server")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	})

	// Wait for interrupt signal
	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			30*time.Second,
		)
		defer cancel()

		log.Info().Msg("Shutting down server")
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Error().Err(err).Msg("Server terminated with error")
		os.Exit(1)
	}
}
