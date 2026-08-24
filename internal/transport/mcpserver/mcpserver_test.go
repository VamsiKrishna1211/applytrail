package mcpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/auth/apikey"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/testsupport"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/mcpserver"
)

// newMCPSession spins up the real router, mounts the MCP handler behind the
// same authentication the REST API uses, and connects a real MCP client. These
// tests therefore cover the transport, the auth middleware, and the tools
// together, which is the combination that actually has to work.
// session bundles a connected MCP client with the service behind it, so a test
// can seed data that has no tool of its own (search profiles are managed over
// REST or the CLI).
type session struct {
	*mcp.ClientSession
	svc       *service.Service
	principal domain.Principal
}

func newMCPSession(t *testing.T, scopes ...domain.Scope) *session {
	t.Helper()

	st, tenantID := testsupport.Store(t)
	svc := service.New(st, service.Options{})

	if len(scopes) == 0 {
		scopes = []domain.Scope{domain.ScopeRead, domain.ScopeWrite}
	}
	generated, err := apikey.Generate()
	require.NoError(t, err)
	_, err = st.APIKeys().Create(t.Context(), domain.APIKey{
		TenantID: tenantID, Prefix: generated.Prefix, Hash: generated.Hash,
		Label: "routine", Scopes: scopes,
	})
	require.NoError(t, err)

	router := httpapi.NewRouter(httpapi.Deps{
		Version:       "test",
		Service:       svc,
		Authenticator: apikey.New(st.APIKeys()),
		MCPHandler:    mcpserver.New(svc, "test").Handler(),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: server.Client(),
	}
	// The bearer token travels on every HTTP request the transport makes,
	// exactly as `claude mcp add --transport http ... --header` sends it.
	transport.HTTPClient.Transport = &bearerTransport{token: generated.Secret}

	connected, err := client.Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connected.Close() })

	return &session{
		ClientSession: connected,
		svc:           svc,
		principal:     testsupport.Principal(tenantID),
	}
}

type bearerTransport struct{ token string }

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func callTool[T any](t *testing.T, session *session, name string, args map[string]any) T {
	t.Helper()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool %s failed: %s", name, contentText(result))

	var out T
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &out), "structured output: %s", raw)
	return out
}

func callToolExpectingError(t *testing.T, session *session, name string, args map[string]any) string {
	t.Helper()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err, "a tool error must be a tool result, not a protocol failure")
	require.True(t, result.IsError, "expected %s to fail", name)
	return contentText(result)
}

func contentText(result *mcp.CallToolResult) string {
	var text string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}
	return text
}

func posting(jobID, title string) map[string]any {
	return map[string]any{
		"url":      "https://boards.greenhouse.io/acme/jobs/" + jobID,
		"title":    title,
		"company":  "Acme, Inc.",
		"location": "Remote",
	}
}

func TestToolsAreListed(t *testing.T) {
	session := newMCPSession(t)

	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	names := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		names[tool.Name] = tool
	}

	for _, expected := range []string{
		"get_search_profile", "filter_new_postings", "record_discoveries",
		"record_application", "update_application_status", "list_applications",
		"get_followups_due", "add_note", "upsert_contact",
	} {
		tool, ok := names[expected]
		require.True(t, ok, "tool %s must be exposed", expected)
		assert.NotEmpty(t, tool.Description, "a tool without a description is one the model will misuse")
		assert.NotNil(t, tool.InputSchema, "schemas are inferred from the argument types")
	}
	assert.Len(t, result.Tools, 9, "the toolset is deliberately small; adding one is a decision, not an accident")
}

// The workflow the instructions describe, end to end.
func TestSearchRunWorkflow(t *testing.T) {
	session := newMCPSession(t)
	candidates := []any{
		posting("4012345", "Senior Software Engineer II"),
		posting("4012346", "Staff Engineer"),
	}

	type verdict struct {
		Index     int    `json:"index"`
		Verdict   string `json:"verdict"`
		MatchTier string `json:"match_tier"`
		Reason    string `json:"reason"`
		PostingID string `json:"posting_id"`
	}
	type filterOut struct {
		Results  []verdict `json:"results"`
		Guidance string    `json:"guidance"`
		Summary  struct {
			New       int `json:"new"`
			Duplicate int `json:"duplicate"`
		} `json:"summary"`
	}

	first := callTool[filterOut](t, session, "filter_new_postings", map[string]any{"candidates": candidates})
	assert.Equal(t, 2, first.Summary.New)
	assert.Contains(t, first.Guidance, "record_discoveries", "the model is told what to do next")

	type discoveryOut struct {
		Results []struct {
			Created   bool   `json:"created"`
			PostingID string `json:"posting_id"`
		} `json:"results"`
	}
	recorded := callTool[discoveryOut](t, session, "record_discoveries", map[string]any{"postings": candidates})
	require.Len(t, recorded.Results, 2)
	for _, r := range recorded.Results {
		assert.True(t, r.Created)
		assert.NotEmpty(t, r.PostingID)
	}

	// The next day's run finds the same jobs.
	second := callTool[filterOut](t, session, "filter_new_postings", map[string]any{"candidates": candidates})
	assert.Equal(t, 0, second.Summary.New, "this is the entire point of the service")
	assert.Equal(t, 2, second.Summary.Duplicate)
	assert.Contains(t, second.Guidance, "Nothing new")
	for i, r := range second.Results {
		assert.Equal(t, i, r.Index)
		assert.NotEmpty(t, r.Reason)
	}
}

func TestRecordApplicationAndFollowup(t *testing.T) {
	session := newMCPSession(t)

	type applicationOut struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		NextActionAt string `json:"next_action_at"`
	}

	applied := callTool[applicationOut](t, session, "record_application", map[string]any{
		"posting":           posting("4012345", "Senior Software Engineer II"),
		"resume_variant":    "backend-v3",
		"applied_at":        "2026-01-15T09:00:00Z",
		"follow_up_in_days": 14,
	})
	assert.Equal(t, "applied", applied.Status)
	require.NotEmpty(t, applied.ID)

	type followupsOut struct {
		Followups []struct {
			ID               string `json:"id"`
			Company          string `json:"company"`
			DaysSinceApplied int    `json:"days_since_applied"`
		} `json:"followups"`
		Count int `json:"count"`
	}

	quiet := callTool[followupsOut](t, session, "get_followups_due", map[string]any{"as_of": "2026-01-20T00:00:00Z"})
	assert.Zero(t, quiet.Count, "five days in there is nothing to chase")

	due := callTool[followupsOut](t, session, "get_followups_due", map[string]any{"as_of": "2026-02-15T00:00:00Z"})
	require.Equal(t, 1, due.Count)
	assert.Equal(t, applied.ID, due.Followups[0].ID)
	assert.Equal(t, "Acme, Inc.", due.Followups[0].Company)
	// 2026-01-15T09:00Z to 2026-02-15T00:00Z is 30 whole days.
	assert.Equal(t, 30, due.Followups[0].DaysSinceApplied)

	// Once applied, the job is permanently a duplicate.
	type filterOut struct {
		Results []struct {
			Verdict           string `json:"verdict"`
			ApplicationStatus string `json:"application_status"`
		} `json:"results"`
	}
	filtered := callTool[filterOut](t, session, "filter_new_postings", map[string]any{
		"candidates": []any{posting("4012345", "Senior Software Engineer II")},
	})
	assert.Equal(t, "duplicate", filtered.Results[0].Verdict)
	assert.Equal(t, "applied", filtered.Results[0].ApplicationStatus)

	t.Run("status moves and the timeline records it", func(t *testing.T) {
		moved := callTool[applicationOut](t, session, "update_application_status", map[string]any{
			"application_id": applied.ID,
			"status":         "screening",
			"note":           "recruiter call booked",
		})
		assert.Equal(t, "screening", moved.Status)
	})

	t.Run("an invalid transition is a tool error naming what is allowed", func(t *testing.T) {
		message := callToolExpectingError(t, session, "update_application_status", map[string]any{
			"application_id": applied.ID,
			"status":         "discovered",
		})
		assert.Contains(t, message, "cannot move from",
			"the model can only correct itself if the error says what went wrong")
	})
}

func TestNeedsReviewIsSurfacedNotGuessed(t *testing.T) {
	session := newMCPSession(t)

	callTool[map[string]any](t, session, "record_discoveries", map[string]any{
		"postings": []any{posting("4012345", "Senior Software Engineer")},
	})

	type filterOut struct {
		Results []struct {
			Verdict   string `json:"verdict"`
			Reason    string `json:"reason"`
			PostingID string `json:"posting_id"`
		} `json:"results"`
		Guidance string `json:"guidance"`
	}

	out := callTool[filterOut](t, session, "filter_new_postings", map[string]any{
		"candidates": []any{map[string]any{
			"url": "https://acme.com/careers/payments", "title": "Senior Software Engineer, Payments",
			"company": "Acme", "location": "Remote",
		}},
	})

	require.Equal(t, "needs_review", out.Results[0].Verdict)
	assert.NotEmpty(t, out.Results[0].PostingID, "the model needs the candidate match to adjudicate")
	assert.Contains(t, out.Results[0].Reason, "payments")
	assert.Contains(t, out.Guidance, "need review")
}

func TestScopesAreEnforcedOverMCP(t *testing.T) {
	session := newMCPSession(t, domain.ScopeRead)
	candidates := []any{posting("4012345", "Senior Software Engineer II")}

	// Reading is fine on a read-only credential, which is the point: a search
	// agent needs no write access to do its job.
	callTool[map[string]any](t, session, "filter_new_postings", map[string]any{"candidates": candidates})

	message := callToolExpectingError(t, session, "record_discoveries", map[string]any{"postings": candidates})
	assert.Contains(t, message, "permission denied")
}

func TestUnauthenticatedConnectionIsRejected(t *testing.T) {
	st, _ := testsupport.Store(t)
	svc := service.New(st, service.Options{})

	router := httpapi.NewRouter(httpapi.Deps{
		Version:       "test",
		Service:       svc,
		Authenticator: apikey.New(st.APIKeys()),
		MCPHandler:    mcpserver.New(svc, "test").Handler(),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	_, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: server.Client(),
	}, nil)
	require.Error(t, err, "the MCP endpoint sits behind the same trust boundary as the REST API")
}

func TestGetSearchProfile(t *testing.T) {
	sess := newMCPSession(t)

	_, err := sess.svc.CreateProfile(t.Context(), sess.principal, domain.SearchProfile{
		Name:            "backend-remote",
		Active:          true,
		Keywords:        []string{"golang", "postgres"},
		RemoteOnly:      true,
		ResurfacePolicy: domain.ResurfacePolicy{CooldownDays: map[domain.Status]int{domain.StatusScreenedOut: 30}},
	})
	require.NoError(t, err)

	type profileOut struct {
		Name              string         `json:"name"`
		Keywords          []string       `json:"keywords"`
		RemoteOnly        bool           `json:"remote_only"`
		ResurfacePolicy   map[string]int `json:"resurface_policy"`
		SearchRunID       string         `json:"search_run_id"`
		RunsToday         int            `json:"runs_today"`
		AvailableProfiles []string       `json:"available_profiles"`
	}

	t.Run("with no name it lists what is available", func(t *testing.T) {
		listed := callTool[profileOut](t, sess, "get_search_profile", map[string]any{})
		assert.Equal(t, []string{"backend-remote"}, listed.AvailableProfiles,
			"listing rather than failing lets an agent discover its own configuration")
		assert.NotNil(t, listed.ResurfacePolicy,
			"nil collections marshal to JSON null, which fails the tool's own output schema")
	})

	t.Run("by name it returns criteria and can open a run", func(t *testing.T) {
		fetched := callTool[profileOut](t, sess, "get_search_profile", map[string]any{
			"name":      "backend-remote",
			"start_run": true,
			"query":     "golang remote",
		})

		assert.Equal(t, "backend-remote", fetched.Name)
		assert.Equal(t, []string{"golang", "postgres"}, fetched.Keywords)
		assert.True(t, fetched.RemoteOnly)
		assert.NotEmpty(t, fetched.SearchRunID, "the run id threads this session's counts together")

		assert.Equal(t, 30, fetched.ResurfacePolicy["screened_out"], "the profile's override")
		assert.Equal(t, -1, fetched.ResurfacePolicy["applied"],
			"and the defaults fill in the rest, so the agent sees every rule that will apply")
	})
}
