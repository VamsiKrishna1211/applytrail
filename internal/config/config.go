// Package config loads and validates runtime configuration from the environment.
//
// Every setting has a documented default that works for local development, so
// `go run ./cmd/applytrail-server` succeeds against a stock `make up` database
// with no .env file present.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment distinguishes local development from a deployed instance.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
)

// Config is the fully resolved configuration for the server and CLI.
type Config struct {
	Env      Environment
	Addr     string
	LogLevel string

	Database  DatabaseConfig
	Dedupe    DedupeConfig
	RateLimit RateLimitConfig
	Telemetry TelemetryConfig

	// Timeouts applied to the HTTP server. Not tunable via env on purpose:
	// they are correctness settings, not knobs.
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// DatabaseConfig describes how to reach Postgres.
type DatabaseConfig struct {
	URL      string
	MaxConns int32
	MinConns int32
}

// DedupeConfig tunes the fuzzy tier of the match engine.
//
// A similarity at or above MatchThreshold is treated as a duplicate. A
// similarity in [ReviewThreshold, MatchThreshold) is reported as needs_review
// so the caller adjudicates instead of the service guessing.
type DedupeConfig struct {
	MatchThreshold  float64
	ReviewThreshold float64
}

// RateLimitConfig bounds request rate per API key.
type RateLimitConfig struct {
	RequestsPerMinute int
	Burst             int
}

// TelemetryConfig controls OpenTelemetry export. Export is disabled entirely
// when OTLPEndpoint is empty, which is the default: no endpoint, no cost.
type TelemetryConfig struct {
	ServiceName  string
	OTLPEndpoint string
}

// Enabled reports whether traces and metrics should be exported.
func (t TelemetryConfig) Enabled() bool { return t.OTLPEndpoint != "" }

// Load reads configuration from the process environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		Env:      Environment(getString("APPLYTRAIL_ENV", string(EnvDevelopment))),
		Addr:     resolveAddr(),
		LogLevel: getString("APPLYTRAIL_LOG_LEVEL", "info"),

		Database: DatabaseConfig{
			URL:      getString("APPLYTRAIL_DATABASE_URL", "postgres://applytrail:applytrail@localhost:5432/applytrail?sslmode=disable"),
			MaxConns: int32(getInt("APPLYTRAIL_DB_MAX_CONNS", 8)),
			MinConns: int32(getInt("APPLYTRAIL_DB_MIN_CONNS", 0)),
		},
		Dedupe: DedupeConfig{
			MatchThreshold:  getFloat("APPLYTRAIL_FUZZY_MATCH_THRESHOLD", 0.87),
			ReviewThreshold: getFloat("APPLYTRAIL_FUZZY_REVIEW_THRESHOLD", 0.75),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: getInt("APPLYTRAIL_RATE_LIMIT_RPM", 120),
			Burst:             getInt("APPLYTRAIL_RATE_LIMIT_BURST", 60),
		},
		Telemetry: TelemetryConfig{
			ServiceName:  getString("OTEL_SERVICE_NAME", "applytrail"),
			OTLPEndpoint: getString("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		},

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   20 * time.Second,
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// resolveAddr honours Cloud Run's injected PORT before falling back to an
// explicit listen address.
func resolveAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return getString("APPLYTRAIL_ADDR", ":8080")
}

func (c Config) validate() error {
	var errs []error

	switch c.Env {
	case EnvDevelopment, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APPLYTRAIL_ENV: %q is not one of development, production", c.Env))
	}

	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("APPLYTRAIL_LOG_LEVEL: %q is not one of debug, info, warn, error", c.LogLevel))
	}

	if c.Database.URL == "" {
		errs = append(errs, errors.New("APPLYTRAIL_DATABASE_URL: must not be empty"))
	}
	if c.Database.MaxConns < 1 {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_DB_MAX_CONNS: must be >= 1, got %d", c.Database.MaxConns))
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_DB_MIN_CONNS: must be within [0, max_conns=%d], got %d", c.Database.MaxConns, c.Database.MinConns))
	}

	if c.Dedupe.MatchThreshold <= 0 || c.Dedupe.MatchThreshold > 1 {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_FUZZY_MATCH_THRESHOLD: must be within (0, 1], got %v", c.Dedupe.MatchThreshold))
	}
	if c.Dedupe.ReviewThreshold <= 0 || c.Dedupe.ReviewThreshold > c.Dedupe.MatchThreshold {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_FUZZY_REVIEW_THRESHOLD: must be within (0, match_threshold=%v], got %v", c.Dedupe.MatchThreshold, c.Dedupe.ReviewThreshold))
	}

	if c.RateLimit.RequestsPerMinute < 1 {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_RATE_LIMIT_RPM: must be >= 1, got %d", c.RateLimit.RequestsPerMinute))
	}
	if c.RateLimit.Burst < 1 {
		errs = append(errs, fmt.Errorf("APPLYTRAIL_RATE_LIMIT_BURST: must be >= 1, got %d", c.RateLimit.Burst))
	}

	return errors.Join(errs...)
}

func getString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		// Surfaced by validate() as an out-of-range value rather than a parse
		// error; returning the fallback here keeps Load's signature simple.
		return fallback
	}
	return n
}

func getFloat(key string, fallback float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
