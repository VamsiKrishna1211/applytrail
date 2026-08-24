package observability

import "context"

// requestIDKey is the context key for the per-request correlation ID.
type requestIDKey struct{}

// ContextWithRequestID returns a context carrying id.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the correlation ID, or "" when absent.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
