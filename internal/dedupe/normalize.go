// Package dedupe decides whether a candidate posting is one the tenant has
// already seen.
//
// The engine is layered: cheap, exact identifier comparisons run first, and
// only what survives them is measured for similarity. Every tier reports how
// it matched and how confident it is, because an ATS job ID match is a fact
// while a title similarity is an inference, and a caller deserves to know
// which one it got.
package dedupe

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// CanonicalizeURL strips the parts of a URL that vary between sightings of the
// same job: tracking parameters, casing, default ports, and fragments.
//
// It deliberately does NOT strip every query parameter. Several boards carry
// the job identity in the query string (Indeed's jk, LinkedIn's currentJobId,
// Greenhouse's gh_jid), so a blanket strip would collapse unrelated jobs onto
// one canonical URL and report them as duplicates of each other.
func CanonicalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A bare "acme.com/jobs/1" is common in hand-entered data.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		// An unparseable URL is still a usable opaque key; lowercasing it is
		// the most we can safely do.
		return strings.ToLower(raw)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme == "http" {
		// Boards serve the same job on both schemes; pick one.
		u.Scheme = "https"
	}
	u.Host = strings.ToLower(u.Host)
	u.Host = strings.TrimPrefix(u.Host, "www.")
	u.Host = strings.TrimSuffix(u.Host, ":443")
	u.Host = strings.TrimSuffix(u.Host, ":80")
	u.Fragment = ""
	u.User = nil

	if u.RawQuery != "" {
		u.RawQuery = canonicalizeQuery(u.Query())
	}

	// Trailing slashes are cosmetic everywhere except the root.
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}

	return u.String()
}

// trackingParams are query parameters that identify how a visitor arrived, not
// which job they are looking at.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true, "mc_cid": true, "mc_eid": true,
	"ref": true, "referrer": true, "referer": true, "refid": true,
	"source": true, "src": true, "trk": true, "trackingid": true, "trackingrefid": true,
	"originalsubdomain": true, "li_fat_id": true, "ebp": true,
	"position": true, "pagenum": true, "savedsearchid": true, "seniority_filter": true,
	"lever-source": true, "lever-origin": true, "gh_src": true,
	"from": true, "campaign_id": true, "sponsored": true, "utm_id": true,
}

// canonicalizeQuery drops tracking parameters and sorts what remains, so
// parameter order stops being a source of spurious differences.
func canonicalizeQuery(values url.Values) string {
	kept := make(url.Values, len(values))
	for key, vals := range values {
		lower := strings.ToLower(key)
		if trackingParams[lower] || strings.HasPrefix(lower, "utm_") {
			continue
		}
		kept[lower] = vals
	}
	if len(kept) == 0 {
		return ""
	}

	keys := make([]string, 0, len(kept))
	for key := range kept {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		vals := kept[key]
		sort.Strings(vals)
		for _, v := range vals {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(key))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

// legalSuffixes are corporate forms that carry no identifying information.
//
// Descriptive words like "technologies", "labs", or "group" are deliberately
// absent: "Acme Labs" and "Acme" are often genuinely different employers, and
// merging them would suppress real jobs.
var legalSuffixes = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "llp": true, "lp": true,
	"ltd": true, "limited": true, "corp": true, "corporation": true,
	"co": true, "company": true, "plc": true, "gmbh": true, "ag": true,
	"sa": true, "sas": true, "bv": true, "nv": true, "ab": true, "oy": true,
	"as": true, "aps": true, "pty": true, "pvt": true, "private": true,
	"srl": true, "spa": true, "kk": true, "kg": true, "ug": true,
}

var (
	nonAlphanumeric = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	multiSpace      = regexp.MustCompile(`\s+`)
	parenthetical   = regexp.MustCompile(`[\(\[\{][^\)\]\}]*[\)\]\}]`)
	reqIDPattern    = regexp.MustCompile(`\b(?:req|job|jr|r)[\-#_ ]?\d{3,}\b`)
	trailingNumber  = regexp.MustCompile(`\s+\d{3,}$`)
)

// NormalizeCompany reduces an employer name to its identifying core, so
// "Acme, Inc.", "ACME Inc" and "acme" resolve to one company.
func NormalizeCompany(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "&", " and ")
	name = nonAlphanumeric.ReplaceAllString(name, " ")
	name = multiSpace.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	// Strip legal forms from the end only. "Co" appears mid-name in real
	// companies ("Co-op Bank"), and dropping it there would be wrong.
	tokens := strings.Fields(name)
	for len(tokens) > 1 && legalSuffixes[tokens[len(tokens)-1]] {
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(tokens, " ")
}

// titleAbbreviations expand to their long form before comparison, which is
// what makes "Sr. SWE II" and "Senior Software Engineer II" the same job.
var titleAbbreviations = map[string]string{
	"sr":     "senior",
	"snr":    "senior",
	"jr":     "junior",
	"jnr":    "junior",
	"swe":    "software engineer",
	"sde":    "software development engineer",
	"sre":    "site reliability engineer",
	"eng":    "engineer",
	"engr":   "engineer",
	"dev":    "developer",
	"mgr":    "manager",
	"mgmt":   "management",
	"ops":    "operations",
	"devops": "development operations",
	"qa":     "quality assurance",
	"ux":     "user experience",
	"ui":     "user interface",
	"pm":     "product manager",
	"tpm":    "technical program manager",
	"ml":     "machine learning",
	"nlp":    "natural language processing",
	"vp":     "vice president",
	"iii":    "",
	"ii":     "",
	"iv":     "",
	"i":      "",
	"v":      "",
}

// titleNoisePhrases are multi-word arrangements removed before tokenising.
//
// They must be matched as phrases, not as individual tokens: stripping the
// bare word "site" would turn "Site Reliability Engineer" into "Reliability
// Engineer", and stripping "full" would turn "Full Stack Engineer" into "Stack
// Engineer". Both are real roles that would then collide with other jobs.
var titleNoisePhrases = regexp.MustCompile(`\b(?:on site|full time|part time|work from home|contract to hire)\b`)

// titleNoiseWords describe the arrangement or contract, not the role. Location
// and employment type are tracked as their own fields, so leaving them in the
// title would make the same job look different depending on how a board
// happened to render it.
//
// Only words that are unambiguous on their own belong here; anything that can
// be part of a role name goes in titleNoisePhrases instead.
var titleNoiseWords = map[string]bool{
	"remote": true, "hybrid": true, "onsite": true,
	"fulltime": true, "parttime": true,
	"contract": true, "contractor": true, "permanent": true, "perm": true,
	"w2": true, "c2c": true, "1099": true,
	"us": true, "usa": true, "uk": true, "emea": true, "apac": true, "eu": true,
	"m": true, "f": true, "d": true, "x": true, // the German "(m/w/d)" convention
}

// levelMarker matches seniority ladder codes: L4, E5, IC3, T2, P6.
var levelMarker = regexp.MustCompile(`^(?:l|e|ic|t|p|g)\d{1,2}$`)

// NormalizeTitle reduces a job title to comparable tokens: abbreviations
// expanded, level and requisition markers dropped, arrangement words removed.
func NormalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return ""
	}

	// Parenthetical asides are almost always location, arrangement, or a
	// requisition number: "(Remote, US)", "(m/w/d)", "(REQ-4821)".
	title = parenthetical.ReplaceAllString(title, " ")
	title = reqIDPattern.ReplaceAllString(title, " ")
	title = strings.ReplaceAll(title, "&", " and ")
	title = nonAlphanumeric.ReplaceAllString(title, " ")
	title = multiSpace.ReplaceAllString(title, " ")
	title = titleNoisePhrases.ReplaceAllString(title, " ")
	title = trailingNumber.ReplaceAllString(title, " ")

	out := make([]string, 0, 8)
	for _, token := range strings.Fields(title) {
		if expansion, ok := titleAbbreviations[token]; ok {
			if expansion == "" {
				continue // a level numeral: "II", "IV"
			}
			out = append(out, strings.Fields(expansion)...)
			continue
		}
		if titleNoiseWords[token] || levelMarker.MatchString(token) {
			continue
		}
		// A bare trailing digit is a level ("Engineer 2"), not a distinction.
		if len(token) <= 2 && isAllDigits(token) {
			continue
		}
		out = append(out, token)
	}
	return strings.Join(out, " ")
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// remoteHints mark a location string as fully remote.
var remoteHints = []string{"remote", "anywhere", "work from home", "wfh", "distributed"}

// countryAliases collapse the many spellings of the same country.
var countryAliases = map[string]string{
	"usa": "united states", "us": "united states", "u s a": "united states",
	"united states of america": "united states", "america": "united states",
	"uk": "united kingdom", "great britain": "united kingdom", "england": "united kingdom",
	"uae": "united arab emirates", "nl": "netherlands", "de": "germany",
}

// BucketLocation reduces a free-text location to a comparable bucket:
// "remote", a city name, or a country.
//
// Hybrid roles bucket by city rather than as remote, because a hybrid job in
// Berlin and a hybrid job in Austin are not interchangeable.
func BucketLocation(location string) string {
	lower := strings.ToLower(strings.TrimSpace(location))
	if lower == "" {
		return ""
	}

	isHybrid := strings.Contains(lower, "hybrid")
	if !isHybrid {
		for _, hint := range remoteHints {
			if strings.Contains(lower, hint) {
				return "remote"
			}
		}
	}

	// "Austin, TX, United States" identifies itself by its first component.
	primary := lower
	if idx := strings.IndexAny(lower, ",|/"); idx > 0 {
		primary = lower[:idx]
	}
	primary = nonAlphanumeric.ReplaceAllString(primary, " ")
	primary = strings.TrimSpace(multiSpace.ReplaceAllString(primary, " "))
	primary = strings.TrimSpace(strings.TrimSuffix(primary, " hybrid"))
	primary = strings.TrimSpace(strings.TrimPrefix(primary, "hybrid "))

	if canonical, ok := countryAliases[primary]; ok {
		return canonical
	}
	return primary
}

// Fingerprint is the tier 3 key: the same role, at the same company, in the
// same place. It is a hash rather than a concatenation so it fits a fixed
// column and cannot be accidentally parsed back apart.
func Fingerprint(normalizedCompany, normalizedTitle, locationBucket string) string {
	sum := sha256.Sum256([]byte(normalizedCompany + "|" + normalizedTitle + "|" + locationBucket))
	return hex.EncodeToString(sum[:])
}

// minDescriptionLength is the shortest description worth hashing. Below this,
// text is usually boilerplate ("Apply on our careers page") that many unrelated
// postings share, and hashing it would manufacture false duplicates.
const minDescriptionLength = 200

// DescriptionHash returns a content hash for tier 4, or "" when the
// description is too short to be identifying.
func DescriptionHash(description string) string {
	normalized := strings.ToLower(strings.TrimSpace(description))
	normalized = multiSpace.ReplaceAllString(normalized, " ")
	if len(normalized) < minDescriptionLength {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
