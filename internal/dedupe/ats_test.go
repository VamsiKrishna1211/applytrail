package dedupe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

func TestExtractIdentity(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		source  domain.Source
		id      string
		company string
	}{
		{
			name: "greenhouse board", url: "https://boards.greenhouse.io/acme/jobs/4012345",
			source: domain.SourceGreenhouse, id: "4012345", company: "acme",
		},
		{
			name: "greenhouse new domain", url: "https://job-boards.greenhouse.io/acme/jobs/4012345?gh_src=abc",
			source: domain.SourceGreenhouse, id: "4012345", company: "acme",
		},
		{
			name: "greenhouse embedded on a company site", url: "https://acme.com/careers?gh_jid=4012345",
			source: domain.SourceUnknown, id: "",
		},
		{
			name: "lever", url: "https://jobs.lever.co/acme/1a2b3c4d-5e6f-4a8b-9c0d-1e2f3a4b5c6d",
			source: domain.SourceLever, id: "1a2b3c4d-5e6f-4a8b-9c0d-1e2f3a4b5c6d", company: "acme",
		},
		{
			name: "ashby", url: "https://jobs.ashbyhq.com/acme/1a2b3c4d-5e6f-4a8b-9c0d-1e2f3a4b5c6d",
			source: domain.SourceAshby, id: "1a2b3c4d-5e6f-4a8b-9c0d-1e2f3a4b5c6d", company: "acme",
		},
		{
			name:   "workday requisition id",
			url:    "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/Austin/Senior-Engineer_R-12345",
			source: domain.SourceWorkday, id: "R-12345",
		},
		{
			name: "smartrecruiters", url: "https://jobs.smartrecruiters.com/Acme/743999912345-senior-engineer",
			source: domain.SourceSmartRecruiters, id: "743999912345", company: "Acme",
		},
		{
			name: "workable", url: "https://apply.workable.com/acme/j/A1B2C3D4E5/",
			source: domain.SourceWorkable, id: "A1B2C3D4E5", company: "acme",
		},
		{
			name: "linkedin numeric path", url: "https://www.linkedin.com/jobs/view/4012345",
			source: domain.SourceLinkedIn, id: "4012345",
		},
		{
			name: "linkedin slug path", url: "https://www.linkedin.com/jobs/view/senior-software-engineer-at-acme-4012345?trk=public_jobs",
			source: domain.SourceLinkedIn, id: "4012345",
		},
		{
			name: "linkedin search page", url: "https://www.linkedin.com/jobs/search?currentJobId=4012345",
			source: domain.SourceLinkedIn, id: "4012345",
		},
		{
			name: "indeed", url: "https://www.indeed.com/viewjob?jk=a1b2c3d4e5f6&from=serp",
			source: domain.SourceIndeed, id: "a1b2c3d4e5f6",
		},
		{
			name: "unknown company careers page", url: "https://acme.com/careers/senior-engineer",
			source: domain.SourceUnknown, id: "",
		},
		{name: "empty", url: "", source: domain.SourceUnknown, id: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupe.ExtractIdentity(tc.url)
			assert.Equal(t, tc.source, got.Source)
			assert.Equal(t, tc.id, got.ExternalID)
			if tc.company != "" {
				assert.Equal(t, tc.company, got.CompanySlug)
			}
		})
	}
}

// The whole point of tier 2 is that the same job reached by two different
// routes resolves to one identity.
func TestExtractIdentity_SameJobDifferentRoutes(t *testing.T) {
	direct := dedupe.ExtractIdentity("https://boards.greenhouse.io/acme/jobs/4012345")
	shared := dedupe.ExtractIdentity("https://boards.greenhouse.io/acme/jobs/4012345?utm_source=twitter&gh_src=xyz")

	assert.Equal(t, direct, shared)
	assert.True(t, direct.Found())
}

// Numeric IDs collide across boards, which is why the key is (source, id) and
// never the ID alone.
func TestExtractIdentity_IDsAreScopedToTheirBoard(t *testing.T) {
	greenhouse := dedupe.ExtractIdentity("https://boards.greenhouse.io/acme/jobs/4012345")
	linkedin := dedupe.ExtractIdentity("https://www.linkedin.com/jobs/view/4012345")

	assert.Equal(t, greenhouse.ExternalID, linkedin.ExternalID, "the raw ids happen to be identical")
	assert.NotEqual(t, greenhouse.Source, linkedin.Source, "so only the source keeps them apart")
}

// Requisition IDs are unique per tenant on Workday, not globally, so the
// dedupe key must carry the employer.
func TestIdentityKey_QualifiesByEmployer(t *testing.T) {
	acme := dedupe.ExtractIdentity("https://acme.wd1.myworkdayjobs.com/en-US/careers/job/Austin/Senior-Engineer_R-12345")
	globex := dedupe.ExtractIdentity("https://globex.wd5.myworkdayjobs.com/en-US/careers/job/Berlin/Staff-Engineer_R-12345")

	assert.Equal(t, acme.ExternalID, globex.ExternalID, "the bare requisition ids are identical")
	assert.NotEqual(t, acme.Key(), globex.Key(),
		"so comparing bare ids would report two unrelated jobs as the same one")
	assert.Equal(t, "acme/R-12345", acme.Key())
	assert.Equal(t, "globex/R-12345", globex.Key())
}

func TestIdentityKey_FallsBackToBareIDWhenNoEmployerInURL(t *testing.T) {
	linkedin := dedupe.ExtractIdentity("https://www.linkedin.com/jobs/view/4012345")
	assert.Equal(t, "4012345", linkedin.Key(), "LinkedIn ids are already globally unique")
}
