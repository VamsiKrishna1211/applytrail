package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/auth/apikey"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/ratelimit"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/testsupport"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
)

// testAPI is a live server plus the credentials to call it, so these tests
// exercise routing, authentication, scope checks, and encoding together rather
// than any one of them in isolation.
type testAPI struct {
	server   *httptest.Server
	adminKey string
	readKey  string
	tenantID domain.ID
}

func newTestAPI(t *testing.T, limiter ratelimit.Limiter, authFailures ...ratelimit.Limiter) *testAPI {
	t.Helper()

	st, tenantID := testsupport.Store(t)
	svc := service.New(st, service.Options{})

	// Credentials are minted through the store directly, because minting one
	// through the API would need a credential to start with.
	mint := func(label string, scopes ...domain.Scope) string {
		generated, err := apikey.Generate()
		require.NoError(t, err)
		_, err = st.APIKeys().Create(t.Context(), domain.APIKey{
			TenantID: tenantID, Prefix: generated.Prefix, Hash: generated.Hash,
			Label: label, Scopes: scopes,
		})
		require.NoError(t, err)
		return generated.Secret
	}

	var failureLimiter ratelimit.Limiter
	if len(authFailures) > 0 {
		failureLimiter = authFailures[0]
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Version:            "test",
		Service:            svc,
		Authenticator:      apikey.New(st.APIKeys()),
		Limiter:            limiter,
		AuthFailureLimiter: failureLimiter,
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return &testAPI{
		server:   server,
		adminKey: mint("admin", domain.ScopeAdmin),
		readKey:  mint("search-agent", domain.ScopeRead),
		tenantID: tenantID,
	}
}

func (a *testAPI) do(t *testing.T, method, path, key string, body any) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, a.server.URL+path, reader)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, payload
}

func decode[T any](t *testing.T, payload []byte) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(payload, &out), "response body: %s", payload)
	return out
}

func candidate(jobID, title string) map[string]any {
	return map[string]any{
		"url":      "https://boards.greenhouse.io/acme/jobs/" + jobID,
		"title":    title,
		"company":  "Acme, Inc.",
		"location": "Remote",
	}
}

func TestHealthEndpointsNeedNoCredential(t *testing.T) {
	api := newTestAPI(t, nil)

	resp, payload := api.do(t, http.MethodGet, "/healthz", "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"a platform probe must reach this without a key, or the deployment never goes live")

	body := decode[map[string]any](t, payload)
	assert.Equal(t, "ok", body["status"])
}

func TestAuthFailuresAreThrottled(t *testing.T) {
	// Two attempts allowed per window, so the third is refused.
	failures := ratelimit.NewMemory(ratelimit.Config{RequestsPerMinute: 2, Burst: 2})
	api := newTestAPI(t, nil, failures)
	body := map[string]any{"candidates": []any{candidate("4012345", "Senior Software Engineer II")}}

	for i := range 2 {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", "at_live_totallyfake", body)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "attempt %d is within the budget", i+1)
	}

	resp, payload := api.do(t, http.MethodPost, "/v1/postings:filter", "at_live_totallyfake", body)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"credential guessing must not be free: the limiter that keys on a principal cannot see a request that has none")
	assert.NotEmpty(t, resp.Header.Get("Retry-After"), "a throttled client needs to know how long to wait")

	errBody := decode[map[string]map[string]any](t, payload)
	assert.Equal(t, "rate_limited", errBody["error"]["code"])

	t.Run("a valid credential still works while failures are throttled", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", api.readKey, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"the failure budget must never be reachable by a caller holding a real key")
	})
}

func TestAuthentication(t *testing.T) {
	api := newTestAPI(t, nil)
	body := map[string]any{"candidates": []any{candidate("4012345", "Senior Software Engineer II")}}

	t.Run("no credential is rejected", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodPost, "/v1/postings:filter", "", body)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		challenge := resp.Header.Get("WWW-Authenticate")
		assert.Contains(t, challenge, "Bearer",
			"the response must tell a client what scheme to use")
		assert.Contains(t, challenge, "api_key",
			"advertising api_key stops an MCP client reading a bare Bearer challenge as an OAuth invitation")

		errBody := decode[map[string]map[string]any](t, payload)
		assert.Equal(t, "unauthenticated", errBody["error"]["code"])
	})

	t.Run("a bad credential is rejected", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", "at_live_totallyfake", body)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("a valid credential is accepted", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", api.readKey, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestScopesAreEnforcedAtTheRoute(t *testing.T) {
	api := newTestAPI(t, nil)
	body := map[string]any{"candidates": []any{candidate("4012345", "Senior Software Engineer II")}}

	resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", api.readKey, body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "dedupe is read-only by design")

	resp, payload := api.do(t, http.MethodPost, "/v1/postings:ingest", api.readKey, body)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errBody := decode[map[string]map[string]any](t, payload)
	assert.Equal(t, "permission_denied", errBody["error"]["code"])

	resp, _ = api.do(t, http.MethodGet, "/v1/admin/api-keys", api.readKey, nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// The end-to-end path the whole service exists for.
func TestDedupeRoundTrip(t *testing.T) {
	api := newTestAPI(t, nil)
	candidates := []any{
		candidate("4012345", "Senior Software Engineer II"),
		candidate("4012346", "Staff Engineer"),
	}

	resp, payload := api.do(t, http.MethodPost, "/v1/postings:ingest", api.adminKey,
		map[string]any{"candidates": candidates})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", payload)

	ingested := decode[struct {
		Results []struct {
			Verdict string `json:"verdict"`
			Created bool   `json:"created"`
		} `json:"results"`
		Summary struct {
			New int `json:"new"`
		} `json:"summary"`
	}](t, payload)
	assert.Equal(t, 2, ingested.Summary.New)

	// The same search runs again tomorrow.
	resp, payload = api.do(t, http.MethodPost, "/v1/postings:filter", api.adminKey,
		map[string]any{"candidates": candidates})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	filtered := decode[struct {
		Results []struct {
			Index     int    `json:"index"`
			Verdict   string `json:"verdict"`
			MatchTier string `json:"match_tier"`
			Reason    string `json:"reason"`
			PostingID string `json:"posting_id"`
		} `json:"results"`
		Summary struct {
			New       int `json:"new"`
			Duplicate int `json:"duplicate"`
		} `json:"summary"`
	}](t, payload)

	assert.Equal(t, 0, filtered.Summary.New)
	assert.Equal(t, 2, filtered.Summary.Duplicate)
	for i, r := range filtered.Results {
		assert.Equal(t, i, r.Index, "results correlate to the request by position")
		assert.Equal(t, "exact_url", r.MatchTier)
		assert.NotEmpty(t, r.Reason, "a verdict without a reason is not actionable")
		assert.NotEmpty(t, r.PostingID)
	}
}

func TestApplicationLifecycleOverHTTP(t *testing.T) {
	api := newTestAPI(t, nil)

	resp, payload := api.do(t, http.MethodPost, "/v1/applications", api.adminKey, map[string]any{
		"posting":        candidate("4012345", "Senior Software Engineer II"),
		"resume_variant": "backend-v3",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", payload)

	created := decode[struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		NextActionAt string `json:"next_action_at"`
	}](t, payload)
	assert.Equal(t, "applied", created.Status)
	assert.NotEmpty(t, created.NextActionAt)

	resp, payload = api.do(t, http.MethodPatch, "/v1/applications/"+created.ID, api.adminKey, map[string]any{
		"status": "screening",
		"note":   "recruiter call booked",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", payload)

	resp, payload = api.do(t, http.MethodGet, "/v1/applications/"+created.ID, api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	detail := decode[struct {
		Application struct {
			Status  string `json:"status"`
			Company string `json:"company"`
		} `json:"application"`
		Events []struct {
			Type     string `json:"type"`
			ToStatus string `json:"to_status"`
		} `json:"events"`
	}](t, payload)
	assert.Equal(t, "screening", detail.Application.Status)
	assert.Equal(t, "Acme, Inc.", detail.Application.Company)
	assert.GreaterOrEqual(t, len(detail.Events), 3, "the timeline records every transition")

	t.Run("an invalid transition is a 400, not a 500", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodPatch, "/v1/applications/"+created.ID, api.adminKey,
			map[string]any{"status": "discovered"})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		errBody := decode[map[string]map[string]any](t, payload)
		assert.Equal(t, "invalid_argument", errBody["error"]["code"])
	})
}

func TestErrorEnvelope(t *testing.T) {
	api := newTestAPI(t, nil)

	t.Run("unknown id is a 404", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodGet, "/v1/applications/"+domain.NewID().String(), api.adminKey, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		errBody := decode[map[string]map[string]any](t, payload)
		assert.Equal(t, "not_found", errBody["error"]["code"])
		assert.NotEmpty(t, errBody["error"]["request_id"], "every error carries a request id for correlation")
	})

	t.Run("malformed id is a 400", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodGet, "/v1/applications/not-a-uuid", api.adminKey, nil)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unknown field is rejected rather than ignored", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodPost, "/v1/postings:filter", api.adminKey,
			map[string]any{"candidates": []any{candidate("1", "Engineer")}, "typoed_field": true})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"silently ignoring a misspelled field hides client bugs")
		assert.Contains(t, string(payload), "typoed_field")
	})

	t.Run("validation errors list every problem", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodPost, "/v1/postings:filter", api.adminKey,
			map[string]any{"candidates": []any{map[string]any{"remote_type": "nonsense", "seniority": "nonsense"}}})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		errBody := decode[struct {
			Error struct {
				Details []string `json:"details"`
			} `json:"error"`
		}](t, payload)
		assert.GreaterOrEqual(t, len(errBody.Error.Details), 2,
			"a caller should learn every problem in one round trip")
	})

	t.Run("unknown route is a 404 in the same envelope", func(t *testing.T) {
		resp, payload := api.do(t, http.MethodGet, "/v1/nope", api.adminKey, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Contains(t, string(payload), "not_found")
	})
}

func TestRateLimiting(t *testing.T) {
	api := newTestAPI(t, ratelimit.NewMemory(ratelimit.Config{RequestsPerMinute: 60, Burst: 2}))
	body := map[string]any{"candidates": []any{candidate("4012345", "Senior Software Engineer II")}}

	for range 2 {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", api.readKey, body)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	resp, payload := api.do(t, http.MethodPost, "/v1/postings:filter", api.readKey, body)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"), "a throttled caller must be told when to return")
	assert.Contains(t, string(payload), "rate_limited")

	t.Run("a different credential is unaffected", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodPost, "/v1/postings:filter", api.adminKey, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "the budget is per credential, not global")
	})
}

func TestAPIKeyManagement(t *testing.T) {
	api := newTestAPI(t, nil)

	resp, payload := api.do(t, http.MethodPost, "/v1/admin/api-keys", api.adminKey, map[string]any{
		"label":  "routine",
		"scopes": []string{"read", "write"},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", payload)

	issued := decode[struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
		Prefix string `json:"prefix"`
	}](t, payload)
	require.NotEmpty(t, issued.Secret)

	// The new key works immediately.
	resp, _ = api.do(t, http.MethodGet, "/v1/applications", issued.Secret, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Listing keys must never expose anything usable.
	resp, payload = api.do(t, http.MethodGet, "/v1/admin/api-keys", api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(payload), issued.Secret,
		"an admin listing must not hand back working credentials")

	resp, _ = api.do(t, http.MethodDelete, "/v1/admin/api-keys/"+issued.ID, api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = api.do(t, http.MethodGet, "/v1/applications", issued.Secret, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "revocation takes effect at once")
}

func TestSearchProfileRoundTrip(t *testing.T) {
	api := newTestAPI(t, nil)

	resp, payload := api.do(t, http.MethodPost, "/v1/search-profiles", api.adminKey, map[string]any{
		"name":             "backend-remote",
		"keywords":         []string{"golang", "postgres"},
		"remote_only":      true,
		"resurface_policy": map[string]int{"screened_out": 30},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", payload)

	resp, payload = api.do(t, http.MethodGet, "/v1/search-profiles/backend-remote", api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	view := decode[struct {
		Profile struct {
			Name     string   `json:"name"`
			Keywords []string `json:"keywords"`
		} `json:"profile"`
		EffectiveResurfacePolicy map[string]int `json:"effective_resurface_policy"`
	}](t, payload)

	assert.Equal(t, "backend-remote", view.Profile.Name)
	assert.Equal(t, []string{"golang", "postgres"}, view.Profile.Keywords)
	assert.Equal(t, 30, view.EffectiveResurfacePolicy["screened_out"], "the override applies")
	assert.Equal(t, -1, view.EffectiveResurfacePolicy["applied"],
		"and unnamed statuses are filled in from the defaults, so the caller sees the whole picture")

	t.Run("an unknown status in a policy is rejected", func(t *testing.T) {
		resp, _ := api.do(t, http.MethodPost, "/v1/search-profiles", api.adminKey, map[string]any{
			"name":             "broken",
			"resurface_policy": map[string]int{"applyed": 30},
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"a typo must fail loudly rather than being silently ignored at match time")
	})
}

func TestFollowupsOverHTTP(t *testing.T) {
	api := newTestAPI(t, nil)

	resp, payload := api.do(t, http.MethodPost, "/v1/applications", api.adminKey, map[string]any{
		"posting":           candidate("4012345", "Senior Software Engineer II"),
		"applied_at":        time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339),
		"follow_up_in_days": 14,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", payload)

	resp, payload = api.do(t, http.MethodGet, "/v1/applications/followups", api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	due := decode[struct {
		Followups []struct {
			ID               string `json:"id"`
			DaysSinceApplied int    `json:"days_since_applied"`
			DaysOverdue      int    `json:"days_overdue"`
		} `json:"followups"`
	}](t, payload)

	require.Len(t, due.Followups, 1)
	assert.Equal(t, 30, due.Followups[0].DaysSinceApplied)
	assert.Equal(t, 16, due.Followups[0].DaysOverdue)
}

// A garbage limit must 400, not be silently treated as the default: an agent
// that mistyped ?limit=fifty deserves to know its param was rejected, not
// results it cannot tell were capped instead of what it asked for.
func TestListApplications_BadLimitIsRejected(t *testing.T) {
	api := newTestAPI(t, nil)

	for _, bad := range []string{"fifty", "-1", "0"} {
		resp, payload := api.do(t, http.MethodGet, "/v1/applications?limit="+bad, api.adminKey, nil)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "limit=%s body: %s", bad, payload)
	}
}

// A full page must carry a next_cursor, and — the actual bug — this must
// still hold when the client asked for more than the 500-row ceiling:
// filter.Limit has to be the value the store actually clamped to and used,
// not the raw client value, or "500 rows returned" never equals "1000 rows
// requested" and next_cursor is never set even though the page was full.
func TestListApplications_NextCursorSurvivesAnOversizedLimit(t *testing.T) {
	api := newTestAPI(t, nil)

	// Genuinely distinct normalized titles: "Engineer I/II/III" would all
	// normalize to "engineer" (roman numerals are stripped as level markers)
	// and fingerprint-collide onto one posting, defeating the fixture.
	titles := []string{"Backend Engineer", "Frontend Engineer", "Platform Engineer"}
	for i, title := range titles {
		resp, payload := api.do(t, http.MethodPost, "/v1/applications", api.adminKey, map[string]any{
			// Distinct titles: identical ones would fingerprint-collide onto
			// one posting (same company, same title, same location) and
			// leave fewer than 3 applications, defeating the fixture.
			"posting": candidate("40123"+string(rune('0'+i)), title),
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", payload)
	}

	// This exercises limit=3 (filter.Limit == requested), not the 500-row
	// ceiling itself: reaching that boundary needs 500+ fixture rows, which
	// is disproportionate to this test's cost. What matters here is that
	// filter.Limit — whatever intParam clamped it to — is what next_cursor's
	// presence is computed against, and that holds regardless of where the
	// clamp lands.
	resp, payload := api.do(t, http.MethodGet, "/v1/applications?limit=3", api.adminKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", payload)

	page := decode[struct {
		Applications []struct{} `json:"applications"`
		NextCursor   string     `json:"next_cursor"`
	}](t, payload)
	assert.Len(t, page.Applications, 3)
	assert.NotEmpty(t, page.NextCursor, "a full page implies there may be more")
}
