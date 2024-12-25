// cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	Port            string
	Environment     string
	ShutdownTimeout time.Duration
}

func loadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found")
	}

	return &Config{
		Port:            getEnv("PORT", "8080"),
		Environment:     getEnv("ENVIRONMENT", "development"),
		ShutdownTimeout: time.Duration(getEnvAsInt("SHUTDOWN_TIMEOUT_SECONDS", 30)) * time.Second,
	}, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func setupLogger(environment string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if environment == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	setupLogger(config.Environment)

	// Create server instance
	server := newServer(config)

	// Setup graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	// Run server
	g.Go(func() error {
		log.Info().Str("port", config.Port).Msg("Starting server")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	})

	// Wait for interrupt signal
	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
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
