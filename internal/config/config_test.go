package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/config"
)

// setEnv sets vars for the duration of the test and clears every
// APPLYTRAIL_/OTEL_ var Load reads first, so a developer's actual shell
// environment cannot leak into what a test observes.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, key := range []string{
		"APPLYTRAIL_ENV", "APPLYTRAIL_ADDR", "APPLYTRAIL_LOG_LEVEL",
		"APPLYTRAIL_DATABASE_URL", "APPLYTRAIL_DB_MAX_CONNS", "APPLYTRAIL_DB_MIN_CONNS",
		"APPLYTRAIL_FUZZY_MATCH_THRESHOLD", "APPLYTRAIL_FUZZY_REVIEW_THRESHOLD",
		"APPLYTRAIL_RATE_LIMIT_RPM", "APPLYTRAIL_RATE_LIMIT_BURST",
		"OTEL_SERVICE_NAME", "OTEL_EXPORTER_OTLP_ENDPOINT", "PORT",
		"APPLYTRAIL_CLERK_SECRET_KEY", "APPLYTRAIL_CLERK_TENANTS", "APPLYTRAIL_CLERK_ISSUER",
		"APPLYTRAIL_CLERK_API_URL", "APPLYTRAIL_PUBLIC_URL",
		"APPLYTRAIL_CLERK_PUBLISHABLE_KEY", "APPLYTRAIL_CLERK_WEB_ORIGINS",
	} {
		t.Setenv(key, "")
	}
	for key, value := range kv {
		t.Setenv(key, value)
	}
}

func TestLoad_Defaults(t *testing.T) {
	setEnv(t, nil)
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, config.EnvDevelopment, cfg.Env)
	assert.Equal(t, 120, cfg.RateLimit.RequestsPerMinute)
	assert.Equal(t, 0.87, cfg.Dedupe.MatchThreshold)
}

// A malformed numeric env var must be rejected, not silently replaced by its
// default: the default is always in range, so validate() could never have
// caught a bad value that never reached it.
func TestLoad_MalformedNumericEnvVarsAreRejected(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"rate limit rpm", map[string]string{"APPLYTRAIL_RATE_LIMIT_RPM": "abc"}},
		{"rate limit burst", map[string]string{"APPLYTRAIL_RATE_LIMIT_BURST": "sixty"}},
		{"fuzzy match threshold", map[string]string{"APPLYTRAIL_FUZZY_MATCH_THRESHOLD": "high"}},
		{"fuzzy review threshold", map[string]string{"APPLYTRAIL_FUZZY_REVIEW_THRESHOLD": "n/a"}},
		{"db max conns", map[string]string{"APPLYTRAIL_DB_MAX_CONNS": "many"}},
		{"db min conns", map[string]string{"APPLYTRAIL_DB_MIN_CONNS": "-"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			_, err := config.Load()
			require.Error(t, err)
			for _, v := range tc.env {
				assert.Contains(t, err.Error(), v, "the bad value itself should be named, not just that something failed")
			}
		})
	}
}

// Two malformed vars at once must both be reported, not just the first: an
// operator fixing config from one error message at a time is worse than
// fixing it from a complete list.
func TestLoad_ReportsEveryMalformedVarAtOnce(t *testing.T) {
	setEnv(t, map[string]string{
		"APPLYTRAIL_RATE_LIMIT_RPM":        "abc",
		"APPLYTRAIL_DB_MAX_CONNS":          "xyz",
		"APPLYTRAIL_FUZZY_MATCH_THRESHOLD": "nope",
	})
	_, err := config.Load()
	require.Error(t, err)
	msg := err.Error()
	assert.True(t, strings.Contains(msg, "APPLYTRAIL_RATE_LIMIT_RPM"))
	assert.True(t, strings.Contains(msg, "APPLYTRAIL_DB_MAX_CONNS"))
	assert.True(t, strings.Contains(msg, "APPLYTRAIL_FUZZY_MATCH_THRESHOLD"))
}

func TestLoad_InvalidEnvironmentIsRejected(t *testing.T) {
	setEnv(t, map[string]string{"APPLYTRAIL_ENV": "staging"})
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APPLYTRAIL_ENV")
}

func TestLoad_ReviewThresholdAboveMatchThresholdIsRejected(t *testing.T) {
	setEnv(t, map[string]string{
		"APPLYTRAIL_FUZZY_MATCH_THRESHOLD":  "0.5",
		"APPLYTRAIL_FUZZY_REVIEW_THRESHOLD": "0.9",
	})
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APPLYTRAIL_FUZZY_REVIEW_THRESHOLD")
}

func TestLoad_ClerkOffByDefault(t *testing.T) {
	setEnv(t, nil)
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.False(t, cfg.Clerk.Enabled())
	assert.False(t, cfg.Clerk.OAuthEnabled())
}

func TestLoad_ClerkOAuth(t *testing.T) {
	setEnv(t, map[string]string{
		"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x",
		"APPLYTRAIL_CLERK_TENANTS":    "user_a=6f1c2a52-58a9-4d0c-9b1e-0f6c5d1e9a77",
		"APPLYTRAIL_CLERK_ISSUER":     "https://example.clerk.accounts.dev",
		"APPLYTRAIL_PUBLIC_URL":       "https://applytrail.example.com/",
	})
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.Clerk.OAuthEnabled())
	assert.Equal(t, "https://applytrail.example.com/mcp", cfg.Clerk.MCPResource())
}

func TestLoad_ClerkWebSignIn(t *testing.T) {
	setEnv(t, map[string]string{
		"APPLYTRAIL_CLERK_SECRET_KEY":      "sk_test_x",
		"APPLYTRAIL_CLERK_TENANTS":         "user_a=6f1c2a52-58a9-4d0c-9b1e-0f6c5d1e9a77",
		"APPLYTRAIL_CLERK_PUBLISHABLE_KEY": "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk",
		"APPLYTRAIL_PUBLIC_URL":            "https://applytrail.example.com/",
		"APPLYTRAIL_CLERK_WEB_ORIGINS":     " http://localhost:5173 ,, https://alias.example.com",
	})
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.Clerk.WebSignInEnabled())
	assert.Equal(t, []string{
		"https://applytrail.example.com", "http://localhost:5173", "https://alias.example.com",
	}, cfg.Clerk.AuthorizedParties())
}

func TestLoad_ClerkFailsClosed(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"secret without tenants": {
			env:  map[string]string{"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x"},
			want: "APPLYTRAIL_CLERK_TENANTS",
		},
		"tenants without secret": {
			env:  map[string]string{"APPLYTRAIL_CLERK_TENANTS": "user_a=x"},
			want: "APPLYTRAIL_CLERK_SECRET_KEY",
		},
		"issuer without public url": {
			env: map[string]string{
				"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x", "APPLYTRAIL_CLERK_TENANTS": "user_a=x",
				"APPLYTRAIL_CLERK_ISSUER": "https://example.clerk.accounts.dev",
			},
			want: "APPLYTRAIL_PUBLIC_URL",
		},
		"plain http issuer": {
			env: map[string]string{
				"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x", "APPLYTRAIL_CLERK_TENANTS": "user_a=x",
				"APPLYTRAIL_CLERK_ISSUER": "http://example.clerk.accounts.dev", "APPLYTRAIL_PUBLIC_URL": "https://a.example.com",
			},
			want: "APPLYTRAIL_CLERK_ISSUER",
		},
		"publishable key without secret": {
			env:  map[string]string{"APPLYTRAIL_CLERK_PUBLISHABLE_KEY": "pk_test_x"},
			want: "APPLYTRAIL_CLERK_SECRET_KEY",
		},
		"publishable key without public url": {
			env: map[string]string{
				"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x", "APPLYTRAIL_CLERK_TENANTS": "user_a=x",
				"APPLYTRAIL_CLERK_PUBLISHABLE_KEY": "pk_test_x",
			},
			want: "APPLYTRAIL_PUBLIC_URL",
		},
		"secret key in the publishable slot": {
			env: map[string]string{
				"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x", "APPLYTRAIL_CLERK_TENANTS": "user_a=x",
				"APPLYTRAIL_CLERK_PUBLISHABLE_KEY": "sk_test_x", "APPLYTRAIL_PUBLIC_URL": "https://a.example.com",
			},
			want: "APPLYTRAIL_CLERK_PUBLISHABLE_KEY",
		},
		"web origin with a path": {
			env: map[string]string{
				"APPLYTRAIL_CLERK_SECRET_KEY": "sk_test_x", "APPLYTRAIL_CLERK_TENANTS": "user_a=x",
				"APPLYTRAIL_CLERK_WEB_ORIGINS": "https://a.example.com/app",
			},
			want: "bare origin",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setEnv(t, tc.env)
			_, err := config.Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
