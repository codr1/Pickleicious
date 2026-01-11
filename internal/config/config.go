// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Filename string `yaml:"filename"`
	// For future Turso support
	URL       string `yaml:"url,omitempty"`
	AuthToken string `yaml:"-"` // Loaded from environment
}

type AWSConfig struct {
	CognitoPoolID   string `yaml:"-"` // Loaded from environment
	CognitoClientID string `yaml:"-"` // Loaded from environment
}

type Config struct {
	App struct {
		Name        string `yaml:"name"`
		Environment string `yaml:"environment"`
		Port        int    `yaml:"port"`
		BaseURL     string `yaml:"base_url"`
		BaseDomain  string `yaml:"base_domain"` // e.g., "localhost" for dev (subdomain.localhost), "pickleicious.com" for prod
		SecretKey   string `yaml:"-"`           // Loaded from environment
	} `yaml:"app"`

	Database DatabaseConfig `yaml:"database"`

	AWS AWSConfig `yaml:"aws"`

	OpenPlay struct {
		EnforcementInterval string `yaml:"enforcement_interval"`
	} `yaml:"open_play"`

	Features struct {
		EnableMetrics bool `yaml:"enable_metrics"`
		EnableTracing bool `yaml:"enable_tracing"`
		EnableDebug   bool `yaml:"enable_debug"`
	} `yaml:"features"`

	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig holds OTP rate limiting settings.
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`

	// TrustProxy: if true, extracts client IP from X-Forwarded-For (rightmost non-private IP).
	// Set to true only when running behind a trusted reverse proxy (nginx, AWS ALB, etc).
	// When false (default), ignores X-Forwarded-For to prevent IP spoofing.
	TrustProxy bool `yaml:"trust_proxy"`

	OTPSend struct {
		CooldownSeconds int `yaml:"cooldown_seconds"`    // default: 60
		MaxPerHour      int `yaml:"max_per_hour"`        // default: 5
		MaxPerIPPerHour int `yaml:"max_per_ip_per_hour"` // default: 20
	} `yaml:"otp_send"`

	OTPVerify struct {
		MaxAttempts     int `yaml:"max_attempts"`        // default: 5
		LockoutSeconds  int `yaml:"lockout_seconds"`     // default: 300 (5 min)
		MaxPerIPPerHour int `yaml:"max_per_ip_per_hour"` // default: 30
	} `yaml:"otp_verify"`
}

// Load loads both .env and yaml configuration
func Load(configPath string) (*Config, error) {
	// Load .env file if it exists
	envPath := filepath.Join(filepath.Dir(configPath), ".env")
	if err := godotenv.Load(envPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	// Read and parse YAML config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Load sensitive values from environment
	cfg.App.SecretKey = os.Getenv("APP_SECRET_KEY")
	cfg.Database.AuthToken = os.Getenv("DATABASE_AUTH_TOKEN")
	cfg.AWS.CognitoPoolID = os.Getenv("COGNITO_POOL_ID")
	cfg.AWS.CognitoClientID = os.Getenv("COGNITO_CLIENT_ID")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("app name is required")
	}
	if c.App.Port == 0 {
		return fmt.Errorf("app port is required")
	}
	if c.App.SecretKey == "" {
		return fmt.Errorf("app secret key is required")
	}
	if c.Database.Driver == "" {
		return fmt.Errorf("database driver is required")
	}
	if c.OpenPlay.EnforcementInterval == "" {
		return fmt.Errorf("open play enforcement interval is required")
	}
	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := cronParser.Parse(c.OpenPlay.EnforcementInterval); err != nil {
		return fmt.Errorf("open play enforcement interval must be a valid cron expression: %w", err)
	}

	// Validate based on database driver
	switch c.Database.Driver {
	case "sqlite":
		if c.Database.Filename == "" {
			return fmt.Errorf("database filename is required for sqlite")
		}
	case "turso":
		if c.Database.URL == "" {
			return fmt.Errorf("database URL is required for turso")
		}
		if c.Database.AuthToken == "" {
			return fmt.Errorf("database auth token is required for turso")
		}
	default:
		return fmt.Errorf("unsupported database driver: %s", c.Database.Driver)
	}

	return nil
}

// RateLimitDefaults returns the rate limit config with defaults applied.
func (c *RateLimitConfig) WithDefaults() RateLimitConfig {
	cfg := *c
	if cfg.OTPSend.CooldownSeconds == 0 {
		cfg.OTPSend.CooldownSeconds = 60
	}
	if cfg.OTPSend.MaxPerHour == 0 {
		cfg.OTPSend.MaxPerHour = 5
	}
	if cfg.OTPSend.MaxPerIPPerHour == 0 {
		cfg.OTPSend.MaxPerIPPerHour = 20
	}
	if cfg.OTPVerify.MaxAttempts == 0 {
		cfg.OTPVerify.MaxAttempts = 5
	}
	if cfg.OTPVerify.LockoutSeconds == 0 {
		cfg.OTPVerify.LockoutSeconds = 300
	}
	if cfg.OTPVerify.MaxPerIPPerHour == 0 {
		cfg.OTPVerify.MaxPerIPPerHour = 30
	}
	return cfg
}
