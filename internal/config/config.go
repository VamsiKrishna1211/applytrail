// Package config loads and validates runtime configuration from the environment.
//
// Every setting has a documented default that works for local development, so
// `go run ./cmd/applytrail-server` succeeds against a stock `make up` database
// with no .env file present.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// defaultDatabaseURL matches deploy/docker-compose.yml, so `go run` works
// against a freshly started local stack with no configuration at all. The
// credentials in it are that compose file's throwaway local ones.
const defaultDatabaseURL = "postgres://applytrail:applytrail@localhost:5432/applytrail?sslmode=disable" //nolint:gosec // local development default, not a secret

// maxPoolSize bounds the connection pool. Postgres itself caps connections far
// below this, and the ceiling also keeps the value inside int32 so a nonsense
// env var cannot wrap it negative.
const maxPoolSize = 1000

// Environment distinguishes local development from a deployed instance.
type Environment string

// The environments a deployment can declare.
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
	Clerk     ClerkConfig

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

// ClerkConfig lets Clerk-issued credentials in alongside ApplyTrail's own API
// keys. Everything is off while SecretKey is empty, which is the default.
type ClerkConfig struct {
	// SecretKey is the Clerk instance's secret key. Set, it accepts Clerk API
	// keys (ak_...).
	SecretKey string
	// Tenants maps Clerk subjects to tenants, "user_abc=<uuid>,org_def=<uuid>".
	// It is also the allowlist: other Clerk accounts are refused.
	Tenants string
	// Issuer is the Clerk instance's issuer URL. Set, it also accepts Clerk
	// OAuth tokens and serves OAuth discovery for MCP clients.
	Issuer string
	// PublicURL is this server's origin as clients reach it, used to build
	// the MCP resource URL that OAuth tokens must name as their audience.
	PublicURL string
	// APIURL overrides Clerk's Backend API base URL. Empty uses Clerk's.
	APIURL string
	// PublishableKey is the instance's publishable key (pk_...). It is public
	// by design. Set, the web UI offers Clerk sign-in and the server accepts
	// the session tokens it produces.
	PublishableKey string
	// WebOrigins are extra origins the web UI may be served from, beyond
	// PublicURL's. A session token minted for any other origin is refused.
	WebOrigins []string
}

// Enabled reports whether Clerk credentials are accepted at all.
func (c ClerkConfig) Enabled() bool { return c.SecretKey != "" }

// OAuthEnabled reports whether Clerk OAuth tokens are accepted.
func (c ClerkConfig) OAuthEnabled() bool { return c.Enabled() && c.Issuer != "" }

// WebSignInEnabled reports whether the web UI signs in with Clerk.
func (c ClerkConfig) WebSignInEnabled() bool { return c.Enabled() && c.PublishableKey != "" }

// AuthorizedParties are the origins the web UI is served from: PublicURL's,
// then WebOrigins.
func (c ClerkConfig) AuthorizedParties() []string {
	var parties []string
	if c.PublicURL != "" {
		parties = append(parties, strings.TrimSuffix(c.PublicURL, "/"))
	}
	for _, o := range c.WebOrigins {
		parties = append(parties, strings.TrimSuffix(o, "/"))
	}
	return parties
}

// MCPResource is the MCP endpoint's URL, exactly as registered in Clerk.
func (c ClerkConfig) MCPResource() string { return strings.TrimSuffix(c.PublicURL, "/") + "/mcp" }

func (c ClerkConfig) validate() []error {
	var errs []error
	if !c.Enabled() {
		if c.Issuer != "" || c.Tenants != "" || c.PublishableKey != "" {
			errs = append(errs, errors.New("APPLYTRAIL_CLERK_SECRET_KEY: required when any other APPLYTRAIL_CLERK_ setting is set"))
		}
		return errs
	}
	// Fail closed: with no mapping, there would be no allowlist either.
	if strings.TrimSpace(c.Tenants) == "" {
		errs = append(errs, errors.New("APPLYTRAIL_CLERK_TENANTS: required when APPLYTRAIL_CLERK_SECRET_KEY is set"))
	}
	if c.Issuer != "" {
		if !strings.HasPrefix(c.Issuer, "https://") {
			errs = append(errs, fmt.Errorf("APPLYTRAIL_CLERK_ISSUER: must be an https URL, got %q", c.Issuer))
		}
		if !strings.HasPrefix(c.PublicURL, "https://") {
			errs = append(errs, fmt.Errorf("APPLYTRAIL_PUBLIC_URL: must be this server's https origin when APPLYTRAIL_CLERK_ISSUER is set, got %q", c.PublicURL))
		}
	}
	if c.PublishableKey != "" {
		if !strings.HasPrefix(c.PublishableKey, "pk_test_") && !strings.HasPrefix(c.PublishableKey, "pk_live_") {
			errs = append(errs, errors.New("APPLYTRAIL_CLERK_PUBLISHABLE_KEY: must start with pk_test_ or pk_live_"))
		}
		if c.PublicURL == "" {
			errs = append(errs, errors.New("APPLYTRAIL_PUBLIC_URL: required when APPLYTRAIL_CLERK_PUBLISHABLE_KEY is set, as the origin browser sessions must come from"))
		}
	}
	for _, o := range c.AuthorizedParties() {
		if u, err := url.Parse(o); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			errs = append(errs, fmt.Errorf("web origin %q: must be a bare origin such as https://applytrail.example.com", o))
		}
	}
	return errs
}

// Load reads configuration from the process environment and validates it.
func Load() (Config, error) {
	// l collects a parse error the moment a numeric env var fails to parse,
	// rather than falling back to the default and letting validate() judge
	// the default's own value. Silently falling back used to mean
	// APPLYTRAIL_RATE_LIMIT_RPM=abc booted on the default of 120 with no
	// error at all — the default is always in range, so validate() could
	// never have caught it.
	l := &loader{}
	cfg := Config{
		Env:      Environment(getString("APPLYTRAIL_ENV", string(EnvDevelopment))),
		Addr:     resolveAddr(),
		LogLevel: getString("APPLYTRAIL_LOG_LEVEL", "info"),

		Database: DatabaseConfig{
			URL:      getString("APPLYTRAIL_DATABASE_URL", defaultDatabaseURL),
			MaxConns: l.getPoolSize("APPLYTRAIL_DB_MAX_CONNS", 8),
			MinConns: l.getPoolSize("APPLYTRAIL_DB_MIN_CONNS", 0),
		},
		Dedupe: DedupeConfig{
			MatchThreshold:  l.getFloat("APPLYTRAIL_FUZZY_MATCH_THRESHOLD", 0.87),
			ReviewThreshold: l.getFloat("APPLYTRAIL_FUZZY_REVIEW_THRESHOLD", 0.75),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: l.getInt("APPLYTRAIL_RATE_LIMIT_RPM", 120),
			Burst:             l.getInt("APPLYTRAIL_RATE_LIMIT_BURST", 60),
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

		Clerk: ClerkConfig{
			SecretKey: getString("APPLYTRAIL_CLERK_SECRET_KEY", ""),
			Tenants:   getString("APPLYTRAIL_CLERK_TENANTS", ""),
			Issuer:    getString("APPLYTRAIL_CLERK_ISSUER", ""),
			PublicURL: getString("APPLYTRAIL_PUBLIC_URL", ""),
			APIURL:    getString("APPLYTRAIL_CLERK_API_URL", ""),

			PublishableKey: getString("APPLYTRAIL_CLERK_PUBLISHABLE_KEY", ""),
			WebOrigins:     getList("APPLYTRAIL_CLERK_WEB_ORIGINS"),
		},
	}

	if err := errors.Join(append(l.errs, cfg.validate())...); err != nil {
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

	errs = append(errs, c.Clerk.validate()...)

	return errors.Join(errs...)
}

func getString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// getList reads a comma-separated value, dropping blanks.
func getList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// loader accumulates parse errors across the several getInt/getFloat calls
// Load makes, so one malformed env var does not stop Load from reporting
// every other one in the same pass.
type loader struct {
	errs []error
}

func (l *loader) getInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: %q is not an integer", key, v))
		return fallback
	}
	return n
}

// getPoolSize reads a connection count, clamped into a sane range. Validation
// reports anything out of bounds, but clamping first means the conversion to
// int32 can never overflow.
func (l *loader) getPoolSize(key string, fallback int) int32 {
	n := l.getInt(key, fallback)
	switch {
	case n < 0:
		return -1 // rejected by validate, with the key named
	case n > maxPoolSize:
		return maxPoolSize
	default:
		return int32(n)
	}
}

func (l *loader) getFloat(key string, fallback float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: %q is not a number", key, v))
		return fallback
	}
	return f
}
