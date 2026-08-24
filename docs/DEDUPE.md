---
title: Deduplication
---

# How deduplication works

ApplyTrail answers one question: **have I seen this job before?**

The hard part is that the same job looks different depending on where you find
it. A role on a company's Greenhouse board and the same role on LinkedIn share
no URL, no job ID, and often not even the same title — "Senior Software
Engineer II" becomes "Sr. SWE II". Meanwhile two genuinely different roles at
one company can have almost identical titles.

So the engine runs five tiers, cheapest and most certain first. Each stops the
moment it matches.

| # | Tier | Rule | Confidence |
|---|------|------|-----------|
| 1 | `exact_url` | canonical URL equality | 1.00 |
| 2 | `ats_id` | `(source, employer/job-id)` parsed from the URL | 1.00 |
| 3 | `fingerprint` | `sha256(company \| title \| location)`, all normalized | 0.95 |
| 4 | `content_hash` | identical posting description | 0.90 |
| 5 | `fuzzy` | title similarity within one company | measured |

Tiers 1–4 are facts. Tier 5 is an inference, and it is treated differently.

## Normalization

**URLs.** Tracking parameters are stripped, the scheme and host are lowercased,
`www.` and trailing slashes go, and the remaining parameters are sorted.

Not every parameter is stripped. Indeed carries the job in `jk`, LinkedIn in
`currentJobId`, embedded Greenhouse boards in `gh_jid`. A blanket strip would
collapse unrelated jobs onto one canonical URL and report them as duplicates of
each other — the worst possible failure, since it silently hides real openings.

**ATS identities.** A table of per-board regexes extracts `(source, job id)`
from the URL: Greenhouse, Lever, Ashby, Workday, SmartRecruiters, Workable,
LinkedIn, Indeed. Adding a board is one table entry plus one test.

The ID is qualified by the employer slug when the URL provides one. This is not
cosmetic: Greenhouse and Lever mint globally unique IDs, but **Workday
requisition numbers are per-tenant**, so `R-12345` exists at thousands of
companies simultaneously. Comparing bare IDs would merge unrelated jobs.

**Companies.** Lowercased, punctuation removed, legal suffixes stripped from the
end only. `Acme, Inc.` → `acme`.

Descriptive words stay. "Acme Labs" and "Acme" are often genuinely different
employers, and merging them would suppress real jobs. `Co` is only stripped as a
trailing legal form, never mid-name — "Co-op Group" keeps its `co`.

**Titles.** Abbreviations expand (`sr`→`senior`, `swe`→`software engineer`),
level markers vanish (`II`, `L4`, `E5`), parentheticals go (`(Remote, US)`,
`(m/w/d)`, `(REQ-4821)`), and arrangement words are removed.

Arrangement words are removed **as phrases**, not as tokens. Stripping the bare
word `site` turns "Site Reliability Engineer" into "Reliability Engineer";
stripping `full` breaks "Full Stack Engineer"; stripping `time` breaks "Time
Series Database Engineer". All three are regression-tested.

Team qualifiers survive on purpose: "Product Manager, Payments" keeps
`payments`, because that really is a different job from "Product Manager".

**Locations** bucket to `remote`, a city, or a country. Hybrid roles bucket by
city — a hybrid job in Berlin and one in Austin are not interchangeable.

**Descriptions** shorter than 200 characters are not hashed at all. Below that
the text is usually boilerplate ("Apply on our careers page") that many
unrelated postings share, and hashing it would manufacture duplicates.

## The fuzzy tier

Similarity is Jaccard overlap of title token sets, and it is **always scoped to
a single company**. "Senior Software Engineer" exists at every employer on
earth; comparing titles across companies would produce constant false positives.

Two thresholds, both configurable:

```
similarity >= 0.87   duplicate
0.75 - 0.87          needs_review
< 0.75               no match
```

The middle band is the point. Consider:

```
existing:  Senior Software Engineer
candidate: Senior Software Engineer, Payments     similarity 0.75
```

That could be the same job with a team appended, or a sibling role on a
different team. The engine does not know, so it says so, names the differing
word, and returns the candidate match for the caller to adjudicate.

The asymmetry justifies it. A wrong `duplicate` silently costs an
opportunity you never learn about. A wrong `new` costs a duplicate application
to an employer who already rejected you. Neither is the engine's call to make.

## Resurfacing

A match is not automatically a skip. Each pipeline status has a cooldown:

| Status | Cooldown |
|--------|----------|
| `applied`, `screening`, `interview`, `offer`, `accepted`, `rejected` | never |
| `queued` | never (still on your list) |
| `withdrawn` | 180 days |
| `ghosted` | 120 days |
| `screened_out` | 90 days |
| `discovered` (never judged) | 30 days |

The asymmetry is deliberate. A role you never judged is cheap to reconsider. One
you deliberately skipped costs more. One that rejected you should not come back.

A search profile overrides only the statuses it names; the rest fall back to
these defaults. `get_search_profile` returns the **merged** policy, so a caller
sees every rule that will actually apply rather than just the deviations.

## Batching

Filtering N candidates costs at most five queries, not 5N: each tier resolves
the whole remaining set in one round trip, and the fuzzy tier only issues a
per-candidate shortlist query for what survived the first four.

Candidates are also deduplicated **against each other** within one request, so a
single batch cannot insert the same job twice.

## Extending it

`dedupe.Matcher` is a port:

```go
type Matcher interface {
    Filter(ctx context.Context, tenantID domain.ID,
           candidates []domain.Candidate,
           policy domain.ResurfacePolicy) ([]domain.FilterResult, error)
}
```

An embedding-based matcher can replace the built-in engine by satisfying it and
passing `service.Options{Matcher: ...}`. Nothing in the service layer changes.
