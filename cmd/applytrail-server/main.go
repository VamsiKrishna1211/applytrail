// Command applytrail-server runs the ApplyTrail HTTP service: the REST API and
// the MCP endpoint, both backed by the same service layer.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/VamsiKrishna1211/applytrail/internal/config"
	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/observability"
	"github.com/VamsiKrishna1211/applytrail/internal/ratelimit"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/mcpserver"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/webui"
)

// version is set at build time via -ldflags.
var version = "dev"

// The budget for failed authentication attempts, deliberately not tunable by
// environment variable. Like the HTTP timeouts, it is a correctness setting
// rather than a knob: a caller holding a valid credential never consumes it, so
// the only reason to raise it would be to make credential guessing cheaper.
const (
	authFailuresPerMinute = 60
	authFailureBurst      = 30
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.LogLevel, cfg.Env == config.EnvDevelopment)
	slog.SetDefault(logger)

	// Signals cancel this context, which unwinds shutdown in reverse order.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := observability.Setup(ctx, observability.TelemetryOptions{
		ServiceName:    cfg.Telemetry.ServiceName,
		ServiceVersion: version,
		Environment:    string(cfg.Env),
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("setup telemetry: %w", err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := shutdownTelemetry(flushCtx); err != nil {
			slog.Error("flush telemetry", "error", err)
		}
	}()
	slog.Info("telemetry configured", "export_enabled", cfg.Telemetry.Enabled())

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		URL:      cfg.Database.URL,
		MaxConns: cfg.Database.MaxConns,
		MinConns: cfg.Database.MinConns,
	})
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	st := postgres.New(pool)
	svc := service.New(st, service.Options{
		Dedupe: dedupe.Config{
			MatchThreshold:  cfg.Dedupe.MatchThreshold,
			ReviewThreshold: cfg.Dedupe.ReviewThreshold,
		},
	})

	authn, err := authentication(cfg.Clerk, st.APIKeys())
	if err != nil {
		return err
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Version:             version,
		DB:                  pool,
		Service:             svc,
		Authenticator:       authn.Authenticator,
		ProtectedResource:   authn.ProtectedResource,
		ClerkPublishableKey: authn.ClerkPublishableKey,
		Limiter: ratelimit.NewMemory(ratelimit.Config{
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			Burst:             cfg.RateLimit.Burst,
		}),
		AuthFailureLimiter: ratelimit.NewMemory(ratelimit.Config{
			RequestsPerMinute: authFailuresPerMinute,
			Burst:             authFailureBurst,
		}),
		MCPHandler: mcpserver.New(svc, version).Handler(),
		// Nil unless the binary was built with `-tags webui`, in which case
		// every unmatched path stays a JSON 404.
		WebUI: webui.Handler(webui.Options{ClerkFrontendAPI: authn.ClerkFrontendAPI}),
	})
	slog.Info("web ui", "embedded", webui.Available())

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		// Rooted in Background, not ctx: ctx is cancelled by the signal that
		// starts shutdown, and every in-flight request's context derives from
		// whatever BaseContext returns. Using ctx here cancelled every request
		// already in flight the instant SIGTERM arrived — before
		// server.Shutdown even got a chance to drain them — so the drain
		// window below waited on handlers whose database calls were already
		// failing with context canceled. A signal cannot be un-delivered, so
		// the only fix is for requests not to observe it in the first place.
		BaseContext: func(net.Listener) context.Context { return context.Background() },
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "version", version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received", "timeout", cfg.ShutdownTimeout)
	}

	// Stop honouring the signal so a second Ctrl-C can still kill a hung
	// shutdown, then drain in-flight requests.
	stop()
	drainCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(drainCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	slog.Info("server stopped cleanly")
	return nil
}
