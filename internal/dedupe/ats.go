package dedupe

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Identity is what an ATS URL reveals about a posting.
type Identity struct {
	Source     domain.Source
	ExternalID string
	// CompanySlug is the employer handle in the URL, e.g. "acme" in
	// boards.greenhouse.io/acme/jobs/123. It is a useful fallback when the
	// caller did not supply a company name.
	CompanySlug string
}

// Found reports whether the URL yielded a usable identity.
func (i Identity) Found() bool { return i.ExternalID != "" }

// Key is the identity used for dedupe: the job ID qualified by its employer
// slug when the board provides one.
//
// The qualification is not cosmetic. Greenhouse and Lever mint globally unique
// IDs, but Workday requisition numbers are per-tenant, so "R-12345" exists at
// thousands of companies at once. Comparing the bare ID would report two
// unrelated jobs as the same one.
func (i Identity) Key() string {
	if i.CompanySlug == "" {
		return i.ExternalID
	}
	return strings.ToLower(i.CompanySlug) + "/" + i.ExternalID
}

// extractor recognises one applicant tracking system.
type extractor struct {
	source domain.Source
	// hosts are matched as suffixes, so both "boards.greenhouse.io" and a
	// customer subdomain resolve to the same ATS.
	hosts []string
	// path extracts from the URL path. Named groups "id" and "company" are
	// read when present.
	path *regexp.Regexp
	// query names a parameter that carries the job ID directly.
	query []string
	// companyFromSubdomain takes the employer slug from the host's first
	// label, for boards that give each customer their own subdomain.
	companyFromSubdomain bool
}

// extractors is the whole board-support surface. Adding a board is one entry
// here plus one test case, which is the point of keeping it table-driven.
var extractors = []extractor{
	{
		source: domain.SourceGreenhouse,
		hosts:  []string{"greenhouse.io"},
		path:   regexp.MustCompile(`^/(?:embed/job_app\?for=)?(?P<company>[^/]+)/jobs/(?P<id>\d+)`),
		// Greenhouse boards embedded on a company's own site pass the job in
		// the query string instead of the path.
		query: []string{"gh_jid"},
	},
	{
		source: domain.SourceLever,
		hosts:  []string{"lever.co"},
		path:   regexp.MustCompile(`^/(?P<company>[^/]+)/(?P<id>[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`),
	},
	{
		source: domain.SourceAshby,
		hosts:  []string{"ashbyhq.com"},
		path:   regexp.MustCompile(`^/(?P<company>[^/]+)/(?P<id>[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`),
	},
	{
		source: domain.SourceWorkday,
		hosts:  []string{"myworkdayjobs.com", "myworkdaysite.com"},
		// Workday appends the requisition ID to the slug after an underscore:
		// .../job/Austin/Senior-Engineer_R-12345
		path: regexp.MustCompile(`/job/[^/]+/[^/]*_(?P<id>[A-Za-z]{1,4}-?\d{3,})`),
		// Requisition IDs repeat across tenants, so the employer subdomain is
		// what makes the identity unique.
		companyFromSubdomain: true,
	},
	{
		source: domain.SourceSmartRecruiters,
		hosts:  []string{"smartrecruiters.com"},
		path:   regexp.MustCompile(`^/(?P<company>[^/]+)/(?P<id>\d{6,})`),
	},
	{
		source: domain.SourceWorkable,
		hosts:  []string{"workable.com"},
		path:   regexp.MustCompile(`^/(?P<company>[^/]+)/j/(?P<id>[0-9A-F]{8,})`),
	},
	{
		source: domain.SourceLinkedIn,
		hosts:  []string{"linkedin.com"},
		// Both /jobs/view/4012345 and the slug form
		// /jobs/view/software-engineer-at-acme-4012345.
		path:  regexp.MustCompile(`^/jobs/view/(?:[^/]*?-)?(?P<id>\d{6,})`),
		query: []string{"currentJobId"},
	},
	{
		source: domain.SourceIndeed,
		hosts:  []string{"indeed.com"},
		query:  []string{"jk", "vjk"},
	},
}

// ExtractIdentity parses an ATS job identity out of a URL.
//
// This runs against the raw URL rather than the canonical one: canonicalization
// lowercases the query string, and some boards use case-sensitive job keys.
func ExtractIdentity(raw string) Identity {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Identity{}
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Identity{}
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))

	for _, e := range extractors {
		if !matchesHost(host, e.hosts) {
			continue
		}

		identity := Identity{Source: e.source}
		if e.path != nil {
			if match := e.path.FindStringSubmatch(u.Path); match != nil {
				identity.ExternalID = namedGroup(e.path, match, "id")
				identity.CompanySlug = namedGroup(e.path, match, "company")
			}
		}
		if e.companyFromSubdomain {
			if label, _, ok := strings.Cut(host, "."); ok {
				identity.CompanySlug = label
			}
		}
		if identity.ExternalID == "" {
			query := u.Query()
			for _, param := range e.query {
				if v := query.Get(param); v != "" {
					identity.ExternalID = v
					break
				}
			}
		}
		if identity.Found() {
			return identity
		}
		// The host is recognised even when the job ID is not, which is still
		// worth reporting: it labels the posting's origin.
		return Identity{Source: e.source, CompanySlug: identity.CompanySlug}
	}
	return Identity{}
}

func matchesHost(host string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func namedGroup(re *regexp.Regexp, match []string, name string) string {
	for i, groupName := range re.SubexpNames() {
		if groupName == name && i < len(match) {
			return match[i]
		}
	}
	return ""
}
