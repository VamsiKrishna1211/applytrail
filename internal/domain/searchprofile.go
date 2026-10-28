package domain

import "time"

// SearchProfile is the tenant's targeting configuration, stored server-side so
// it can be tuned by editing data instead of editing an agent's prompt.
//
// A caller may override any field for a single run; overrides are not
// persisted.
type SearchProfile struct {
	ID       ID
	TenantID ID

	// Name is the stable handle a caller passes, unique per tenant.
	Name        string
	Description string
	Active      bool

	Keywords  []string
	Titles    []string
	Locations []string

	RemoteOnly      bool
	MinCompensation *int64
	Currency        string
	Seniority       []Seniority

	CompanyAllowlist []string
	CompanyBlocklist []string
	ExcludeKeywords  []string

	ResurfacePolicy ResurfacePolicy

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate reports every problem with the profile at once.
func (p SearchProfile) Validate() error {
	var v ValidationError
	if p.Name == "" {
		v.Add("name must not be empty")
	}
	for _, s := range p.Seniority {
		if !s.Valid() {
			v.Add("seniority %q is not a recognised level", s)
		}
	}
	if p.MinCompensation != nil && *p.MinCompensation < 0 {
		v.Add("min_compensation must not be negative, got %d", *p.MinCompensation)
	}
	if err := p.ResurfacePolicy.Validate(); err != nil {
		var pv *ValidationError
		if ok := asValidation(err, &pv); ok {
			for _, problem := range pv.Details() {
				v.Add("%s", problem)
			}
		}
	}
	return v.OrNil()
}

// SearchRun records one execution of a profile: what was searched, how many
// candidates came back, and how many survived dedupe. It is the audit trail
// that answers "did the routine actually run today, and did it find anything?"
type SearchRun struct {
	ID        ID
	TenantID  ID
	ProfileID *ID
	// ProfileName is the profile's name, filled in by the service layer from
	// ProfileID for display; "" when the run used the built-in defaults or
	// its profile has since been deleted. The store itself never sets this.
	ProfileName string

	Source             Source
	Query              string
	CandidatesSeen     int
	NewCount           int
	EligibleAgainCount int
	DuplicateCount     int
	NeedsReviewCount   int

	StartedAt  time.Time
	FinishedAt *time.Time
	Notes      string
}
