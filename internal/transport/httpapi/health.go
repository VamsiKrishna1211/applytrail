package httpapi

import (
	"context"
	"net/http"
	"time"
)

// Pinger reports whether a dependency is reachable. *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// readinessTimeout bounds the dependency check so a hung database cannot hold
// a platform health probe open.
const readinessTimeout = 3 * time.Second

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// handleLive answers the liveness probe. It deliberately checks nothing: a
// process that can serve this is alive, and failing it would make the platform
// restart a server that is merely waiting on its database.
func handleLive(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, healthResponse{Status: "ok", Version: version})
	}
}

// handleReady answers the readiness probe by checking every dependency the
// server needs before it can serve real traffic.
func handleReady(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if db != nil {
			if err := db.Ping(ctx); err != nil {
				writeJSON(w, r, http.StatusServiceUnavailable, errorBody{Error: errorDetail{
					Code:    "not_ready",
					Message: "database unreachable",
				}})
				return
			}
		}
		writeJSON(w, r, http.StatusOK, healthResponse{Status: "ready"})
	}
}
