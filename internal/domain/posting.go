package domain

import "time"

// Source identifies where a posting was found. It pairs with ExternalID to
// form the strongest dedupe signal after an exact URL match: two records with
// the same Greenhouse job ID are the same job regardless of how they were
// surfaced.
type Source string

// Known sources. Unrecognised values are preserved rather than rejected, so a
// new board can be recorded before its ID extractor exists.
const (
	SourceUnknown         Source = ""
	SourceGreenhouse      Source = "greenhouse"
	SourceLever           Source = "lever"
	SourceAshby           Source = "ashby"
	SourceWorkday         Source = "workday"
	SourceSmartRecruiters Source = "smartrecruiters"
	SourceWorkable        Source = "workable"
	SourceLinkedIn        Source = "linkedin"
	SourceIndeed          Source = "indeed"
	SourceCompanySite     Source = "company_site"
	SourceReferral        Source = "referral"
	SourceOther           Source = "other"
)

// RemoteType describes the work arrangement.
type RemoteType string

// The recognised work arrangements.
const (
	RemoteUnknown RemoteType = ""
	RemoteFull    RemoteType = "remote"
	RemoteHybrid  RemoteType = "hybrid"
	RemoteOnsite  RemoteType = "onsite"
)

// Valid reports whether r is a recognised arrangement.
func (r RemoteType) Valid() bool {
	switch r {
	case RemoteUnknown, RemoteFull, RemoteHybrid, RemoteOnsite:
		return true
	}
	return false
}

// Seniority is a coarse level bucket, useful for filtering out roles far from
// the target band.
type Seniority string

// The recognised seniority buckets.
const (
	SeniorityUnknown   Seniority = ""
	SeniorityIntern    Seniority = "intern"
	SeniorityJunior    Seniority = "junior"
	SeniorityMid       Seniority = "mid"
	SenioritySenior    Seniority = "senior"
	SeniorityStaff     Seniority = "staff"
	SeniorityPrincipal Seniority = "principal"
	SeniorityLead      Seniority = "lead"
	SeniorityManager   Seniority = "manager"
	SeniorityDirector  Seniority = "director"
	SeniorityExecutive Seniority = "executive"
)

// Valid reports whether s is a recognised level.
func (s Seniority) Valid() bool {
	switch s {
	case SeniorityUnknown, SeniorityIntern, SeniorityJunior, SeniorityMid,
		SenioritySenior, SeniorityStaff, SeniorityPrincipal, SeniorityLead,
		SeniorityManager, SeniorityDirector, SeniorityExecutive:
		return true
	}
	return false
}

// Compensation is an advertised pay range. Amounts are stored in whole
// currency units, annualized.
type Compensation struct {
	Min      *int64
	Max      *int64
	Currency string
}

// Posting is a job as it exists in the world: the thing being deduplicated.
// It is distinct from Application, which is the tenant's relationship to it.
type Posting struct {
	ID        ID
	TenantID  ID
	CompanyID ID

	Source     Source
	ExternalID string

	// CanonicalURL is RawURL with tracking parameters stripped and components
	// normalized. It carries a unique constraint per tenant and serves as the
	// first match tier.
	CanonicalURL string
	RawURL       string

	Title string
	// NormalizedTitle has abbreviations expanded and level markers stripped.
	// It feeds both the fingerprint and the trigram index behind fuzzy matching.
	NormalizedTitle string

	LocationRaw string
	// LocationBucket is "remote", a normalized city, or a country.
	LocationBucket string
	RemoteType     RemoteType
	Seniority      Seniority

	Compensation    Compensation
	VisaSponsorship *bool
	TechTags        []string

	// DescriptionHash catches the same job cross-posted under different titles.
	DescriptionHash string
	// Fingerprint is sha256(normalized company | normalized title | location
	// bucket) and carries a unique constraint per tenant.
	Fingerprint string

	PostedAt    *time.Time
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	ClosedAt    *time.Time

	// Raw preserves whatever the caller sent, so a future extractor can
	// backfill fields without re-fetching the posting.
	Raw []byte

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Candidate is an unsaved posting as reported by a caller: the input to both
// dedupe and ingest. Fields are intentionally loose because they come straight
// out of a search result.
type Candidate struct {
	URL         string
	Title       string
	Company     string
	Location    string
	Source      Source
	Description string
	PostedAt    *time.Time

	// Optional enrichment the caller may already have parsed.
	RemoteType      RemoteType
	Seniority       Seniority
	Compensation    Compensation
	VisaSponsorship *bool
	TechTags        []string
	Raw             []byte
}

// Validate reports every problem with the candidate at once.
func (c Candidate) Validate() error {
	var v ValidationError
	if c.URL == "" && (c.Title == "" || c.Company == "") {
		v.Add("candidate needs either a url, or both a title and a company")
	}
	if !c.RemoteType.Valid() {
		v.Add("remote_type %q is not a recognised arrangement", c.RemoteType)
	}
	if !c.Seniority.Valid() {
		v.Add("seniority %q is not a recognised level", c.Seniority)
	}
	if c.Compensation.Min != nil && c.Compensation.Max != nil && *c.Compensation.Min > *c.Compensation.Max {
		v.Add("compensation min (%d) is greater than max (%d)", *c.Compensation.Min, *c.Compensation.Max)
	}
	return v.OrNil()
}
