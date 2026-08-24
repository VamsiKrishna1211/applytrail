package dedupe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
)

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		why  string
	}{
		{
			name: "strips utm tracking",
			in:   "https://boards.greenhouse.io/acme/jobs/4012345?utm_source=linkedin&utm_medium=social",
			want: "https://boards.greenhouse.io/acme/jobs/4012345",
			why:  "the same job shared through two channels is one job",
		},
		{
			name: "normalises scheme, host case, and www",
			in:   "HTTP://WWW.Acme.com/careers/123/",
			want: "https://acme.com/careers/123",
		},
		{
			name: "drops the fragment",
			in:   "https://jobs.lever.co/acme/abc-123#apply",
			want: "https://jobs.lever.co/acme/abc-123",
		},
		{
			name: "keeps Indeed's job key",
			in:   "https://www.indeed.com/viewjob?jk=abc123&from=serp&vjs=3",
			want: "https://indeed.com/viewjob?jk=abc123&vjs=3",
			why:  "jk identifies the job; stripping it would merge unrelated postings",
		},
		{
			name: "keeps LinkedIn's currentJobId",
			in:   "https://www.linkedin.com/jobs/search?currentJobId=4012345&trk=public_jobs",
			want: "https://linkedin.com/jobs/search?currentjobid=4012345",
			why:  "the identity lives in the query string on search pages",
		},
		{
			name: "sorts remaining parameters",
			in:   "https://acme.com/jobs?z=1&a=2",
			want: "https://acme.com/jobs?a=2&z=1",
			why:  "parameter order must not make one job look like two",
		},
		{
			name: "accepts a bare host",
			in:   "acme.com/careers/1",
			want: "https://acme.com/careers/1",
		},
		{name: "empty stays empty", in: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dedupe.CanonicalizeURL(tc.in), tc.why)
		})
	}
}

func TestNormalizeCompany(t *testing.T) {
	tests := []struct {
		in   string
		want string
		why  string
	}{
		{in: "Acme, Inc.", want: "acme"},
		{in: "ACME Incorporated", want: "acme"},
		{in: "acme", want: "acme"},
		{in: "Acme Technologies Ltd", want: "acme technologies", why: "descriptive words are identifying and stay"},
		{in: "Ben & Jerry's", want: "ben and jerry s"},
		{in: "Co-op Group", want: "co op group", why: "'co' mid-name is part of the name, not a legal suffix"},
		{in: "Siemens AG", want: "siemens"},
		{in: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, dedupe.NormalizeCompany(tc.in), tc.why)
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		in   string
		want string
		why  string
	}{
		{in: "Senior Software Engineer II", want: "senior software engineer"},
		{in: "Sr. SWE II", want: "senior software engineer", why: "the headline case: abbreviations and level markers must collapse"},
		{in: "Software Engineer, L4", want: "software engineer"},
		{in: "Staff Engineer (Remote, US)", want: "staff engineer"},
		{in: "Backend Engineer (m/w/d)", want: "backend engineer"},
		{in: "Data Engineer - Full Time", want: "data engineer"},
		{in: "Site Reliability Engineer", want: "site reliability engineer"},
		{in: "SRE", want: "site reliability engineer"},
		{in: "Engineering Manager (REQ-4821)", want: "engineering manager"},
		{in: "Product Manager, Payments", want: "product manager payments", why: "a team qualifier is a real distinction and must survive"},
		{in: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, dedupe.NormalizeTitle(tc.in), tc.why)
		})
	}
}

func TestBucketLocation(t *testing.T) {
	tests := []struct {
		in   string
		want string
		why  string
	}{
		{in: "Remote", want: "remote"},
		{in: "Remote - US", want: "remote"},
		{in: "Work From Home", want: "remote"},
		{in: "Austin, TX, United States", want: "austin"},
		{in: "Hybrid - Berlin", want: "berlin", why: "hybrid roles are tied to a city, so they bucket by city"},
		{in: "USA", want: "united states"},
		{in: "UK", want: "united kingdom"},
		{in: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, dedupe.BucketLocation(tc.in), tc.why)
		})
	}
}

func TestFingerprintAndDescriptionHash(t *testing.T) {
	linkedIn := dedupe.Fingerprint(
		dedupe.NormalizeCompany("Acme, Inc."),
		dedupe.NormalizeTitle("Sr. SWE II"),
		dedupe.BucketLocation("Remote - US"),
	)
	careersPage := dedupe.Fingerprint(
		dedupe.NormalizeCompany("acme"),
		dedupe.NormalizeTitle("Senior Software Engineer II"),
		dedupe.BucketLocation("Remote"),
	)
	assert.Equal(t, linkedIn, careersPage,
		"the same job seen on LinkedIn and the careers page must fingerprint identically")

	different := dedupe.Fingerprint("acme", "senior software engineer", "austin")
	assert.NotEqual(t, linkedIn, different, "location is part of the identity")

	t.Run("short descriptions are not hashed", func(t *testing.T) {
		assert.Empty(t, dedupe.DescriptionHash("Apply on our careers page."),
			"boilerplate is shared by unrelated postings and would manufacture duplicates")
	})

	t.Run("whitespace differences do not change the hash", func(t *testing.T) {
		long := "We are looking for a senior engineer to join our platform team. " +
			"You will design and operate distributed systems that serve millions of requests. " +
			"Experience with Go, Postgres, and Kubernetes is strongly preferred for this role."
		spaced := "  We are looking for a senior engineer to join our platform team.   " +
			"You  will design and operate distributed systems that serve millions of requests. " +
			"Experience with Go, Postgres, and Kubernetes is strongly preferred for this role.  "
		assert.Equal(t, dedupe.DescriptionHash(long), dedupe.DescriptionHash(spaced))
		assert.NotEmpty(t, dedupe.DescriptionHash(long))
	})
}

// These titles are the reason noise removal is phrase-based rather than
// token-based. Each one contains a word that looks like noise in isolation.
func TestNormalizeTitle_DoesNotEatRealRoleNames(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "Site Reliability Engineer", want: "site reliability engineer"},
		{in: "Full Stack Engineer", want: "full stack engineer"},
		{in: "Time Series Database Engineer", want: "time series database engineer"},
		{in: "On-Site Support Technician", want: "support technician"},
		{in: "Part Time Data Analyst", want: "data analyst"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, dedupe.NormalizeTitle(tc.in))
		})
	}
}
