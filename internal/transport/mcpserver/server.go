// Package mcpserver exposes ApplyTrail over the Model Context Protocol.
//
// Every tool here decodes arguments, calls one internal/service method, and
// encodes the result. That is the whole of it: an agent and a curl command go
// through identical business rules, so the two surfaces cannot drift.
package mcpserver

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
)

// Server builds MCP handlers backed by the service layer.
type Server struct {
	svc     *service.Service
	version string
	// cache keeps inferred JSON schemas across requests. A fresh mcp.Server is
	// built per request (see serverFor), and re-deriving nine tools' schemas
	// every time would be pure waste.
	cache *mcp.SchemaCache
}

// New returns an MCP server over svc.
func New(svc *service.Service, version string) *Server {
	return &Server{svc: svc, version: version, cache: mcp.NewSchemaCache()}
}

// instructions tell the model how to use the toolset as a workflow rather than
// as nine unrelated functions. Without this, the common failure is calling
// record_discoveries without filtering first, which works but wastes the point
// of the service.
const instructions = `ApplyTrail remembers which jobs you have already seen and applied to.

A typical search run:
  1. get_search_profile   - fetch targeting criteria, and open a run to log against
  2. (search the web yourself using those criteria)
  3. filter_new_postings  - ask which results are worth pursuing
  4. record_discoveries   - record the ones that came back new or eligible_again
  5. record_application   - after actually applying to one
  6. get_followups_due    - applications that have gone quiet long enough to chase

Verdicts from filter_new_postings:
  new             nothing on record; pursue it
  duplicate       already seen, and policy says do not resurface; skip it
  eligible_again  seen before, but enough time has passed to reconsider
  needs_review    a close but inconclusive match. Read the reason and posting_id,
                  decide yourself, and say which you chose in the note field.

Never treat a needs_review as automatically new: that is how a duplicate
application gets sent.`

// Handler returns the HTTP handler to mount at /mcp.
//
// The transport is stateless with JSON responses rather than SSE. That is a
// deployment decision: ApplyTrail is built to run on a scale-to-zero platform
// where consecutive requests can land on different container instances, and a
// stateful session map lives in one process only. Stateless costs
// server-initiated messages, which none of these nine tools need.
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(s.serverFor, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
}

// serverFor builds an MCP server bound to the request's authenticated caller.
//
// The principal comes from the same middleware that guards the REST API, so
// authentication, scope checks, and rate limits are enforced once, in one
// place, for both transports. Capturing it in the tool closures means a
// handler cannot forget to authorise.
func (s *Server) serverFor(r *http.Request) *mcp.Server {
	principal, _ := auth.PrincipalFromContext(r.Context())

	srv := mcp.NewServer(
		&mcp.Implementation{Name: "applytrail", Version: s.version},
		&mcp.ServerOptions{Instructions: instructions, SchemaCache: s.cache},
	)
	s.registerTools(srv, principal)
	return srv
}

// toolError converts a service error into a message the model can act on.
//
// Client mistakes come back as tool errors with the reason intact, because a
// model that reads "cannot move from applied to discovered" can correct itself.
// Server faults return a generic message: their detail belongs in the logs, not
// in a model's context window.
func toolError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound),
		errors.Is(err, domain.ErrInvalidArgument),
		errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrPermissionDenied),
		errors.Is(err, domain.ErrRateLimited),
		errors.Is(err, domain.ErrUnauthenticated):
		return err
	default:
		return fmt.Errorf("applytrail is unavailable right now; retry shortly")
	}
}

// call is the shape every tool body takes: run the use case, translate the
// error, and hand back a typed result.
func call[Out any](fn func() (Out, error)) (*mcp.CallToolResult, Out, error) {
	out, err := fn()
	if err != nil {
		var zero Out
		return nil, zero, toolError(err)
	}
	return nil, out, nil
}
