package httpapi

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/VamsiKrishna1211/applytrail/internal/observability"
)

// requestIDHeader is both accepted from clients (for cross-service tracing)
// and echoed on every response.
const requestIDHeader = "X-Request-Id"

// maxClientRequestIDLen bounds a client-supplied request ID. It is echoed
// into every structured log line for the request, so an unbounded value is a
// log-inflation vector, not just cosmetic.
const maxClientRequestIDLen = 128

// validRequestID matches the character set a trace ID or UUID actually needs:
// no whitespace or control characters, so a value cannot inject a fake extra
// log line or corrupt the log format when concatenated in.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// requestID assigns each request a correlation ID, publishes it on the
// context for the logger, and echoes it to the client.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" || len(id) > maxClientRequestIDLen || !validRequestID.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(observability.ContextWithRequestID(r.Context(), id)))
	})
}

// requestLogger emits one structured line per request once it completes.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		level := slog.LevelInfo
		if ww.Status() >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// recoverer converts a panic into a 500 with the standard error envelope,
// logging the stack rather than killing the process.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// A client disconnect surfaces as this sentinel; it is not a bug.
			if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel is compared by identity
				panic(rec)
			}
			slog.ErrorContext(r.Context(), "panic recovered",
				"panic", rec,
				"stack", string(stack()),
				"path", r.URL.Path,
			)
			writeJSON(w, r, http.StatusInternalServerError, errorBody{Error: errorDetail{
				Code:      "internal",
				Message:   "internal error",
				RequestID: observability.RequestIDFromContext(r.Context()),
			}})
		}()
		next.ServeHTTP(w, r)
	})
}
