package domain

import (
	"slices"
	"time"
)

// Scope is a coarse permission attached to an API key.
type Scope string

const (
	// ScopeRead permits every read-only operation, including batch dedupe,
	// which is read-only by design so a search agent can run with a key that
	// cannot mutate anything.
	ScopeRead Scope = "read"
	// ScopeWrite permits recording postings, applications, events, and notes.
	ScopeWrite Scope = "write"
	// ScopeAdmin permits key and profile management.
	ScopeAdmin Scope = "admin"
)

// AllScopes lists every scope, for validation messages and CLI help.
var AllScopes = []Scope{ScopeRead, ScopeWrite, ScopeAdmin}

// Valid reports whether s is a recognised scope.
func (s Scope) Valid() bool { return slices.Contains(AllScopes, s) }

// ParseScopes validates and de-duplicates a list of scope strings.
func ParseScopes(raw []string) ([]Scope, error) {
	var v ValidationError
	seen := make(map[Scope]bool, len(raw))
	scopes := make([]Scope, 0, len(raw))

	for _, r := range raw {
		s := Scope(r)
		if !s.Valid() {
			v.Add("scope %q is not one of %v", r, AllScopes)
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		scopes = append(scopes, s)
	}
	if len(scopes) == 0 {
		v.Add("at least one scope is required")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	return scopes, nil
}

// APIKey is a credential record. The secret itself is never stored: only a
// lookup prefix and a password hash of the full key.
type APIKey struct {
	ID       ID
	TenantID ID

	// Prefix is the leading, non-secret portion of the key. It is indexed and
	// used to find the single candidate row to verify against, so verification
	// costs one hash rather than one per key in the table.
	Prefix string
	// Hash is a SHA-256 hash of the full key (see internal/auth/apikey).
	Hash string

	Label  string
	Scopes []Scope

	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time

	CreatedAt time.Time
}

// Active reports whether the key may authenticate as of now.
func (k APIKey) Active(now time.Time) bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt != nil && !now.Before(*k.ExpiresAt) {
		return false
	}
	return true
}

// Principal is the authenticated caller, carried on the request context. Every
// service method takes it and scopes its queries to Principal.TenantID; that
// is the whole of the tenant isolation story.
type Principal struct {
	TenantID ID
	KeyID    ID
	KeyLabel string
	Scopes   []Scope
}

// HasScope reports whether the principal holds scope. ScopeAdmin implies every
// other scope, so an admin key does not need to enumerate them.
func (p Principal) HasScope(scope Scope) bool {
	return slices.Contains(p.Scopes, scope) || slices.Contains(p.Scopes, ScopeAdmin)
}

// RequireScope returns a permission error when the principal lacks scope.
func (p Principal) RequireScope(scope Scope) error {
	if p.HasScope(scope) {
		return nil
	}
	return NewPermissionError(p.KeyLabel, p.Scopes, scope)
}
