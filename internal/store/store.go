// Package store defines the persistence ports.
//
// Everything above this package speaks in domain types; only the adapters
// below it know about SQL. Swapping in a different database means writing one
// implementation of these interfaces and changing one line in main.
package store

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Store is the aggregate port. Accessors return focused sub-stores so a test
// fake can implement only the surface it exercises.
type Store interface {
	Tenants() TenantStore
	APIKeys() APIKeyStore
	Companies() CompanyStore
	Postings() PostingStore
	Applications() ApplicationStore
	Events() EventStore
	Contacts() ContactStore
	SearchProfiles() SearchProfileStore
	SearchRuns() SearchRunStore

	// Tx runs fn inside a transaction. The Store handed to fn is bound to that
	// transaction; using the outer Store inside fn escapes it, so don't.
	// Returning an error rolls back.
	Tx(ctx context.Context, fn func(ctx context.Context, tx Store) error) error
}

// TenantStore persists tenants.
type TenantStore interface {
	Create(ctx context.Context, name, slug string) (domain.Tenant, error)
	Get(ctx context.Context, id domain.ID) (domain.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (domain.Tenant, error)
	List(ctx context.Context) ([]domain.Tenant, error)
}

// APIKeyStore persists API key records. It never sees the key itself, only a
// prefix and a hash.
type APIKeyStore interface {
	Create(ctx context.Context, key domain.APIKey) (domain.APIKey, error)
	// GetByPrefix returns the single candidate for verification.
	GetByPrefix(ctx context.Context, prefix string) (domain.APIKey, error)
	List(ctx context.Context, tenantID domain.ID) ([]domain.APIKey, error)
	Touch(ctx context.Context, id domain.ID, usedAt time.Time) error
	Revoke(ctx context.Context, tenantID, id domain.ID) (domain.APIKey, error)
}

// CompanyStore persists employers, keyed by normalized name.
type CompanyStore interface {
	Upsert(ctx context.Context, company domain.Company) (domain.Company, error)
	Get(ctx context.Context, tenantID, id domain.ID) (domain.Company, error)
	GetByNormalizedName(ctx context.Context, tenantID domain.ID, normalizedName string) (domain.Company, error)
	// FindByNormalizedNames resolves many employers at once, keyed by
	// normalized name. The fuzzy match tier is scoped per company, so this
	// runs once per batch instead of once per candidate.
	FindByNormalizedNames(ctx context.Context, tenantID domain.ID, normalizedNames []string) (map[string]domain.Company, error)
	List(ctx context.Context, tenantID domain.ID) ([]domain.Company, error)
}

// PostingStore persists job postings and serves the dedupe lookups.
//
// The Find* methods are batched by design: one round trip resolves a match
// tier for an entire candidate set, so filtering 200 postings costs five
// queries rather than a thousand.
type PostingStore interface {
	FindByCanonicalURLs(ctx context.Context, tenantID domain.ID, urls []string) ([]domain.PostingRef, error)
	FindByExternalIDs(ctx context.Context, tenantID domain.ID, keys []domain.SourceKey) ([]domain.PostingRef, error)
	FindByFingerprints(ctx context.Context, tenantID domain.ID, fingerprints []string) ([]domain.PostingRef, error)
	FindByDescriptionHashes(ctx context.Context, tenantID domain.ID, companyIDs []domain.ID, hashes []string) ([]domain.PostingRef, error)

	// FindFuzzyCandidates shortlists postings at one company whose normalized
	// title is trigram-similar to the given one. Scoring and thresholds stay
	// in the match engine; this only narrows the field.
	FindFuzzyCandidates(ctx context.Context, tenantID, companyID domain.ID, normalizedTitle string, limit int) ([]domain.PostingRef, error)

	Insert(ctx context.Context, posting domain.Posting) (domain.Posting, error)
	Get(ctx context.Context, tenantID, id domain.ID) (domain.Posting, error)
	TouchSeen(ctx context.Context, tenantID, id domain.ID, seenAt time.Time) (domain.Posting, error)
	Close(ctx context.Context, tenantID, id domain.ID, closedAt time.Time) error
}

// ApplicationListFilter narrows a pipeline listing. Zero values mean "no
// filter", and Cursor implements keyset pagination.
type ApplicationListFilter struct {
	Statuses []domain.Status
	// CompanyNormalizedName must already be normalized (dedupe.NormalizeCompany)
	// by the time it reaches the store: this is compared directly against
	// companies.normalized_name. Service.ListApplications does that
	// normalization, so callers of the service may pass a raw display name.
	CompanyNormalizedName string
	Since                 *time.Time
	Cursor                *Cursor
	Limit                 int
}

// Cursor is an opaque keyset position: the (created_at, id) of the last row a
// caller saw. Keyset paging stays stable while new applications are inserted,
// which OFFSET does not.
type Cursor struct {
	CreatedAt time.Time
	ID        domain.ID
}

// ApplicationWithPosting is a pipeline row joined to enough posting and
// company detail to be useful without a second lookup.
type ApplicationWithPosting struct {
	Application    domain.Application
	PostingTitle   string
	PostingURL     string
	LocationBucket string
	CompanyName    string
}

// StatusCount is one bar of the funnel report.
type StatusCount struct {
	Status domain.Status
	Count  int
}

// ResumeVariantStat reports how a given resume performed.
type ResumeVariantStat struct {
	Variant   string
	Sent      int
	Responded int
	Rejected  int
	Ghosted   int
}

// ApplicationUpdate is a partial update. A nil field means "leave unchanged",
// which is why every field is a pointer.
type ApplicationUpdate struct {
	ResumeVariant      *string
	ResumeURL          *string
	ResumeHash         *string
	CoverLetterVariant *string
	NextActionAt       *time.Time
	// ClearNextAction removes the follow-up date. It exists because a nil
	// NextActionAt already means "unchanged", so there would otherwise be no
	// way to express "unset".
	ClearNextAction bool
	Notes           *string
}

// StatusTransition is a compare-and-swap status change: the update only
// applies while the row still reads From, so a concurrent transition loses
// loudly instead of overwriting silently.
type StatusTransition struct {
	From            domain.Status
	To              domain.Status
	AppliedAt       *time.Time
	NextActionAt    *time.Time
	ClearNextAction bool
}

// ApplicationStore persists the pipeline.
type ApplicationStore interface {
	// Insert is idempotent on (tenant, posting): a second call for the same
	// posting returns the existing row rather than failing.
	Insert(ctx context.Context, app domain.Application) (domain.Application, error)
	Get(ctx context.Context, tenantID, id domain.ID) (domain.Application, error)
	GetByPosting(ctx context.Context, tenantID, postingID domain.ID) (domain.Application, error)

	ApplyTransition(ctx context.Context, tenantID, id domain.ID, t StatusTransition) (domain.Application, error)
	Update(ctx context.Context, tenantID, id domain.ID, u ApplicationUpdate) (domain.Application, error)

	List(ctx context.Context, tenantID domain.ID, f ApplicationListFilter) ([]ApplicationWithPosting, error)
	ListFollowupsDue(ctx context.Context, tenantID domain.ID, dueBefore time.Time, limit int) ([]ApplicationWithPosting, error)

	CountByStatus(ctx context.Context, tenantID domain.ID) ([]StatusCount, error)
	ResumeVariantStats(ctx context.Context, tenantID domain.ID) ([]ResumeVariantStat, error)
}

// EventStore appends to and reads application timelines. There is deliberately
// no update or delete: the timeline is an audit trail.
type EventStore interface {
	Insert(ctx context.Context, event domain.ApplicationEvent) (domain.ApplicationEvent, error)
	List(ctx context.Context, tenantID, applicationID domain.ID, limit int) ([]domain.ApplicationEvent, error)
	LatestStatusChange(ctx context.Context, tenantID, applicationID domain.ID) (domain.ApplicationEvent, error)
}

// ContactStore persists people and their links to applications.
type ContactStore interface {
	Upsert(ctx context.Context, contact domain.Contact) (domain.Contact, error)
	Get(ctx context.Context, tenantID, id domain.ID) (domain.Contact, error)
	List(ctx context.Context, tenantID domain.ID, companyID *domain.ID, limit int) ([]domain.Contact, error)
	LinkToApplication(ctx context.Context, tenantID, applicationID, contactID domain.ID) error
	ListForApplication(ctx context.Context, tenantID, applicationID domain.ID) ([]domain.Contact, error)
}

// SearchProfileUpdate is a partial update; nil means "leave unchanged".
type SearchProfileUpdate struct {
	Description      *string
	Active           *bool
	Keywords         []string
	Titles           []string
	Locations        []string
	RemoteOnly       *bool
	MinCompensation  *int64
	Currency         *string
	Seniority        []domain.Seniority
	CompanyAllowlist []string
	CompanyBlocklist []string
	ExcludeKeywords  []string
	ResurfacePolicy  *domain.ResurfacePolicy
}

// SearchProfileStore persists targeting configuration.
type SearchProfileStore interface {
	Create(ctx context.Context, profile domain.SearchProfile) (domain.SearchProfile, error)
	Get(ctx context.Context, tenantID, id domain.ID) (domain.SearchProfile, error)
	GetByName(ctx context.Context, tenantID domain.ID, name string) (domain.SearchProfile, error)
	List(ctx context.Context, tenantID domain.ID, activeOnly bool) ([]domain.SearchProfile, error)
	Update(ctx context.Context, tenantID, id domain.ID, u SearchProfileUpdate) (domain.SearchProfile, error)
	// NamesByIDs resolves a batch of profile ids to names in one round trip,
	// for annotating a list of search runs without an N+1 lookup. A missing
	// id (deleted since the run referencing it was recorded) is simply absent
	// from the result.
	NamesByIDs(ctx context.Context, tenantID domain.ID, ids []domain.ID) (map[domain.ID]string, error)
}

// SearchRunProgress is a batch of counts to add to a run's totals. Counts
// accumulate so a long routine can report incrementally.
type SearchRunProgress struct {
	CandidatesSeen     int
	NewCount           int
	EligibleAgainCount int
	DuplicateCount     int
	NeedsReviewCount   int
}

// SearchRunFilter narrows a run listing.
type SearchRunFilter struct {
	ProfileID *domain.ID
	Since     *time.Time
	Limit     int
}

// SearchRunStore persists the audit of executed searches.
type SearchRunStore interface {
	Create(ctx context.Context, run domain.SearchRun) (domain.SearchRun, error)
	RecordProgress(ctx context.Context, tenantID, id domain.ID, p SearchRunProgress) (domain.SearchRun, error)
	Finish(ctx context.Context, tenantID, id domain.ID, finishedAt time.Time, notes string) (domain.SearchRun, error)
	List(ctx context.Context, tenantID domain.ID, f SearchRunFilter) ([]domain.SearchRun, error)
	// CountSince returns the true count of a profile's runs since the given
	// time — not capped the way a List call must be, so RunsToday reads
	// correctly for a profile run more than once a day, every day.
	CountSince(ctx context.Context, tenantID, profileID domain.ID, since time.Time) (int, error)
}
