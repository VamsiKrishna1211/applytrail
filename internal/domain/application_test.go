package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.Status
		to      domain.Status
		wantErr bool
		reason  string
	}{
		{name: "discovered to applied", from: domain.StatusDiscovered, to: domain.StatusApplied},
		{name: "applied to screening", from: domain.StatusApplied, to: domain.StatusScreening},
		{name: "screening to interview", from: domain.StatusScreening, to: domain.StatusInterview},
		{name: "offer to accepted", from: domain.StatusOffer, to: domain.StatusAccepted},
		{
			name: "interview to interview", from: domain.StatusInterview, to: domain.StatusInterview,
			reason: "multiple rounds are normal and each deserves its own event",
		},
		{
			name: "ghosted back to interview", from: domain.StatusGhosted, to: domain.StatusInterview,
			reason: "employers do resurface after months of silence",
		},
		{
			name: "rejected to applied", from: domain.StatusRejected, to: domain.StatusApplied,
			wantErr: true, reason: "rejected is terminal",
		},
		{
			name: "accepted to interview", from: domain.StatusAccepted, to: domain.StatusInterview,
			wantErr: true, reason: "accepted is terminal",
		},
		{
			name: "discovered to offer", from: domain.StatusDiscovered, to: domain.StatusOffer,
			wantErr: true, reason: "an offer without applying is not a real path",
		},
		{
			name: "applied to applied", from: domain.StatusApplied, to: domain.StatusApplied,
			wantErr: true, reason: "re-asserting a non-interview status is a caller mistake",
		},
		{
			name: "unknown target", from: domain.StatusApplied, to: domain.Status("hired-maybe"),
			wantErr: true, reason: "unrecognised statuses are rejected, not stored",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.ValidateTransition(tc.from, tc.to)
			if tc.wantErr {
				require.Error(t, err, tc.reason)
				assert.ErrorIs(t, err, domain.ErrInvalidArgument)
				return
			}
			require.NoError(t, err, tc.reason)
		})
	}
}

func TestStatusClassification(t *testing.T) {
	assert.True(t, domain.StatusRejected.IsTerminal())
	assert.False(t, domain.StatusScreening.IsTerminal())

	assert.True(t, domain.StatusApplied.IsEngaged(), "engaged statuses are the ones follow-ups act on")
	assert.False(t, domain.StatusDiscovered.IsEngaged())
	assert.False(t, domain.StatusRejected.IsEngaged())
}

func TestResurfacePolicy(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	policy := domain.DefaultResurfacePolicy()

	t.Run("applied never resurfaces", func(t *testing.T) {
		eligible, reason := policy.Eligible(domain.StatusApplied, now.AddDate(-5, 0, 0), now)
		assert.False(t, eligible, "re-applying to a job you already applied to is the failure this service exists to prevent")
		assert.Contains(t, reason, "never")
	})

	t.Run("screened out becomes eligible after the cooldown", func(t *testing.T) {
		eligible, _ := policy.Eligible(domain.StatusScreenedOut, now.AddDate(0, 0, -89), now)
		assert.False(t, eligible, "89 days is inside the 90 day cooldown")

		eligible, reason := policy.Eligible(domain.StatusScreenedOut, now.AddDate(0, 0, -91), now)
		assert.True(t, eligible)
		assert.Contains(t, reason, "past the 90d cooldown")
	})

	t.Run("never judged resurfaces soonest", func(t *testing.T) {
		eligible, _ := policy.Eligible(domain.StatusDiscovered, now.AddDate(0, 0, -31), now)
		assert.True(t, eligible, "a role that was never judged is cheap to reconsider")
	})

	t.Run("a profile overrides only what it names", func(t *testing.T) {
		custom := domain.ResurfacePolicy{CooldownDays: map[domain.Status]int{
			domain.StatusScreenedOut: 7,
		}}
		eligible, _ := custom.Eligible(domain.StatusScreenedOut, now.AddDate(0, 0, -8), now)
		assert.True(t, eligible, "the override applies")

		eligible, _ = custom.Eligible(domain.StatusApplied, now.AddDate(-5, 0, 0), now)
		assert.False(t, eligible, "unnamed statuses fall back to the defaults")
	})
}

func TestPrincipalScopes(t *testing.T) {
	reader := domain.Principal{Scopes: []domain.Scope{domain.ScopeRead}, KeyLabel: "search-agent"}
	assert.True(t, reader.HasScope(domain.ScopeRead))
	assert.False(t, reader.HasScope(domain.ScopeWrite))
	require.ErrorIs(t, reader.RequireScope(domain.ScopeWrite), domain.ErrPermissionDenied)

	admin := domain.Principal{Scopes: []domain.Scope{domain.ScopeAdmin}}
	assert.True(t, admin.HasScope(domain.ScopeWrite), "admin implies every other scope")
	require.NoError(t, admin.RequireScope(domain.ScopeRead))
}

func TestValidationErrorCollectsEveryProblem(t *testing.T) {
	candidate := domain.Candidate{
		RemoteType: domain.RemoteType("hybrid-ish"),
		Seniority:  domain.Seniority("very senior"),
	}
	err := candidate.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	var v *domain.ValidationError
	require.ErrorAs(t, err, &v)
	assert.Len(t, v.Details(), 3, "a caller should learn every problem in one round trip")
}
