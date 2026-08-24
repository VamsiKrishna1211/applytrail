package service

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/auth/apikey"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// IssuedKey is a newly minted credential. Secret appears here exactly once and
// is never recoverable afterwards, because only its hash is stored.
type IssuedKey struct {
	Key    domain.APIKey
	Secret string
}

// CreateAPIKey issues a credential for the caller's tenant.
func (s *Service) CreateAPIKey(ctx context.Context, p domain.Principal, label string, scopes []domain.Scope, expiresAt *time.Time) (IssuedKey, error) {
	if err := p.RequireScope(domain.ScopeAdmin); err != nil {
		return IssuedKey{}, err
	}
	if len(scopes) == 0 {
		return IssuedKey{}, domain.NewValidationError("at least one scope is required")
	}
	for _, scope := range scopes {
		if !scope.Valid() {
			return IssuedKey{}, domain.NewValidationError("scope " + string(scope) + " is not recognised")
		}
	}

	generated, err := apikey.Generate()
	if err != nil {
		return IssuedKey{}, err
	}

	created, err := s.store.APIKeys().Create(ctx, domain.APIKey{
		TenantID:  p.TenantID,
		Prefix:    generated.Prefix,
		Hash:      generated.Hash,
		Label:     label,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return IssuedKey{}, err
	}
	return IssuedKey{Key: created, Secret: generated.Secret}, nil
}

// ListAPIKeys returns the tenant's credentials. The hash is cleared before
// returning: it is not a secret worth leaking through an admin endpoint.
func (s *Service) ListAPIKeys(ctx context.Context, p domain.Principal) ([]domain.APIKey, error) {
	if err := p.RequireScope(domain.ScopeAdmin); err != nil {
		return nil, err
	}
	keys, err := s.store.APIKeys().List(ctx, p.TenantID)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i].Hash = ""
	}
	return keys, nil
}

// RevokeAPIKey disables a credential. Revocation is idempotent, so a caller
// retrying after a timeout does not get an error.
func (s *Service) RevokeAPIKey(ctx context.Context, p domain.Principal, id domain.ID) (domain.APIKey, error) {
	if err := p.RequireScope(domain.ScopeAdmin); err != nil {
		return domain.APIKey{}, err
	}
	key, err := s.store.APIKeys().Revoke(ctx, p.TenantID, id)
	if err != nil {
		return domain.APIKey{}, err
	}
	key.Hash = ""
	return key, nil
}
