// Package httpapi exposes the REST surface. Handlers here decode requests,
// call internal/service, and encode results. All business rules live in the
// service layer so the MCP transport behaves identically.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/observability"
)

// maxRequestBody caps decoded request bodies. Batch dedupe requests are the
// largest realistic payload; 4 MiB comfortably holds a few hundred candidates
// with descriptions while bounding memory per request.
const maxRequestBody = 4 << 20

// errorBody is the single error envelope every failing endpoint returns.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []string `json:"details,omitempty"`
	RequestID string   `json:"request_id,omitempty"`
}

// writeJSON encodes v as the response body with the given status.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent, so this can only be logged.
		slog.ErrorContext(r.Context(), "encode response body", "error", err)
	}
}

// writeError maps a domain error onto an HTTP status and the error envelope.
// Unrecognised errors become 500 and are logged with their full detail, while
// the client receives a generic message.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	status, code := statusForError(err)

	message := err.Error()
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "request failed", "error", err, "path", r.URL.Path)
		message = "internal error"
	} else {
		slog.DebugContext(ctx, "request rejected", "error", err, "code", code, "path", r.URL.Path)
	}

	var invalid *domain.ValidationError
	var details []string
	if errors.As(err, &invalid) {
		details = invalid.Details()
	}

	writeJSON(w, r, status, errorBody{Error: errorDetail{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: observability.RequestIDFromContext(ctx),
	}})
}

func statusForError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrInvalidArgument):
		return http.StatusBadRequest, "invalid_argument"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrUnauthenticated):
		return http.StatusUnauthorized, "unauthenticated"
	case errors.Is(err, domain.ErrPermissionDenied):
		return http.StatusForbidden, "permission_denied"
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited"
	default:
		return http.StatusInternalServerError, "internal"
	}
}

// decodeJSON reads exactly one JSON value from the request body into dst,
// rejecting unknown fields so a typo in a client payload surfaces as a 400
// rather than being silently ignored.
func decodeJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, maxRequestBody)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("%w: request body exceeds %d bytes", domain.ErrInvalidArgument, maxErr.Limit)
		}
		return fmt.Errorf("%w: malformed JSON body: %s", domain.ErrInvalidArgument, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: body must contain exactly one JSON object", domain.ErrInvalidArgument)
	}
	return nil
}
