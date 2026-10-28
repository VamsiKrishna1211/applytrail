package mcpserver

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// The toolset is deliberately shaped around the steps of a search run rather
// than mirroring the REST endpoints one for one. Nine workflow tools cost far
// less context than twenty CRUD tools, and each one maps to a decision the
// agent actually makes, which is what keeps it from composing the wrong
// sequence.

// PostingInput is a candidate as the agent found it. Only enough to identify
// the job is required; everything else improves match quality if present.
type PostingInput struct {
	URL         string `json:"url,omitempty" jsonschema:"link to the posting; the single most useful field for deduplication"`
	Title       string `json:"title,omitempty" jsonschema:"job title exactly as listed"`
	Company     string `json:"company,omitempty" jsonschema:"employer name; may be omitted when the url is from a known job board"`
	Location    string `json:"location,omitempty" jsonschema:"location as listed, e.g. Remote, or Austin, TX"`
	Source      string `json:"source,omitempty" jsonschema:"where it was found: linkedin, indeed, greenhouse, lever, ashby, workday, company_site, referral, other"`
	Description string `json:"description,omitempty" jsonschema:"full posting text; enables matching the same job cross-posted under a different title"`

	RemoteType      string   `json:"remote_type,omitempty" jsonschema:"remote, hybrid, or onsite"`
	Seniority       string   `json:"seniority,omitempty" jsonschema:"intern, junior, mid, senior, staff, principal, lead, manager, director, or executive"`
	CompMin         *int64   `json:"comp_min,omitempty" jsonschema:"advertised minimum annual compensation, whole units"`
	CompMax         *int64   `json:"comp_max,omitempty" jsonschema:"advertised maximum annual compensation, whole units"`
	CompCurrency    string   `json:"comp_currency,omitempty" jsonschema:"ISO currency code for the compensation range"`
	VisaSponsorship *bool    `json:"visa_sponsorship,omitempty" jsonschema:"whether the posting states visa sponsorship is available"`
	TechTags        []string `json:"tech_tags,omitempty" jsonschema:"technologies named in the posting"`
	PostedAt        string   `json:"posted_at,omitempty" jsonschema:"when the job was posted, RFC3339"`
}

func (p PostingInput) toDomain() (domain.Candidate, error) {
	candidate := domain.Candidate{
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.Company,
		Location:    p.Location,
		Source:      domain.Source(p.Source),
		Description: p.Description,
		RemoteType:  domain.RemoteType(p.RemoteType),
		Seniority:   domain.Seniority(p.Seniority),
		Compensation: domain.Compensation{
			Min:      p.CompMin,
			Max:      p.CompMax,
			Currency: p.CompCurrency,
		},
		VisaSponsorship: p.VisaSponsorship,
		TechTags:        p.TechTags,
	}
	posted, err := parseTimeArg("posted_at", p.PostedAt)
	if err != nil {
		return domain.Candidate{}, err
	}
	candidate.PostedAt = posted
	return candidate, nil
}

// toCandidates converts a batch, reporting every malformed posted_at at once
// rather than failing on the first: an agent submitting two hundred
// candidates deserves the full list of what to fix, not one error per retry.
func toCandidates(inputs []PostingInput) ([]domain.Candidate, error) {
	candidates := make([]domain.Candidate, len(inputs))
	var v domain.ValidationError
	for i, in := range inputs {
		c, err := in.toDomain()
		if err != nil {
			v.Addf("candidates[%d]: %s", i, err.Error())
			continue
		}
		candidates[i] = c
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	return candidates, nil
}

// VerdictOutput is one candidate's answer. It reports the evidence alongside
// the recommendation, so an agent that disagrees can still act sensibly.
type VerdictOutput struct {
	Index      int     `json:"index" jsonschema:"position of this candidate in the request"`
	Verdict    string  `json:"verdict" jsonschema:"new, duplicate, eligible_again, or needs_review"`
	MatchTier  string  `json:"match_tier" jsonschema:"which rule matched: none, exact_url, ats_id, fingerprint, content_hash, or fuzzy"`
	Confidence float64 `json:"confidence" jsonschema:"1.0 for identifier matches; the measured similarity for fuzzy matches"`
	Reason     string  `json:"reason" jsonschema:"plain-language explanation of the verdict"`

	PostingID         string `json:"posting_id,omitempty" jsonschema:"the matched posting, when there was one"`
	ApplicationID     string `json:"application_id,omitempty"`
	ApplicationStatus string `json:"application_status,omitempty" jsonschema:"where the matched job stands in your pipeline"`
	LastSeenAt        string `json:"last_seen_at,omitempty"`
	DaysSince         *int   `json:"days_since,omitempty" jsonschema:"days since the matched posting was last seen"`
}

func toVerdict(r domain.FilterResult) VerdictOutput {
	out := VerdictOutput{
		Index:      r.Index,
		Verdict:    string(r.Verdict),
		MatchTier:  string(r.Tier),
		Confidence: r.Confidence,
		Reason:     r.Reason,
		DaysSince:  r.DaysSince,
	}
	if r.PostingID != nil {
		out.PostingID = r.PostingID.String()
	}
	if r.ApplicationID != nil {
		out.ApplicationID = r.ApplicationID.String()
	}
	if r.ApplicationStatus != nil {
		out.ApplicationStatus = string(*r.ApplicationStatus)
	}
	if r.LastSeenAt != nil {
		out.LastSeenAt = r.LastSeenAt.Format(time.RFC3339)
	}
	return out
}

func (s *Server) registerTools(srv *mcp.Server, p domain.Principal) {
	s.addGetSearchProfile(srv, p)
	s.addFilterNewPostings(srv, p)
	s.addRecordDiscoveries(srv, p)
	s.addRecordApplication(srv, p)
	s.addUpdateApplicationStatus(srv, p)
	s.addListApplications(srv, p)
	s.addGetFollowupsDue(srv, p)
	s.addAddNote(srv, p)
	s.addUpsertContact(srv, p)
}

// --- 1. get_search_profile --------------------------------------------------

type getProfileArgs struct {
	Name     string `json:"name,omitempty" jsonschema:"profile name; omit to list the available profiles"`
	StartRun bool   `json:"start_run,omitempty" jsonschema:"open a search run to log this session's results against"`
	Query    string `json:"query,omitempty" jsonschema:"what you are about to search for, recorded on the run"`
}

type profileOutput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Active      bool   `json:"active"`

	Keywords  []string `json:"keywords"`
	Titles    []string `json:"titles"`
	Locations []string `json:"locations"`

	RemoteOnly      bool     `json:"remote_only"`
	MinCompensation *int64   `json:"min_compensation,omitempty"`
	Seniority       []string `json:"seniority"`

	CompanyAllowlist []string `json:"company_allowlist"`
	CompanyBlocklist []string `json:"company_blocklist"`
	ExcludeKeywords  []string `json:"exclude_keywords"`

	ResurfacePolicy map[string]int `json:"resurface_policy" jsonschema:"days before a job in each status may be surfaced again; -1 means never"`

	RunsToday int    `json:"runs_today" jsonschema:"searches already logged against this profile today"`
	LastRunAt string `json:"last_run_at,omitempty"`

	SearchRunID string `json:"search_run_id,omitempty" jsonschema:"pass this to filter_new_postings and record_discoveries to log against this run"`

	AvailableProfiles []string `json:"available_profiles,omitempty" jsonschema:"set when no name was given"`
}

func (s *Server) addGetSearchProfile(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_search_profile",
		Description: "Fetch the targeting criteria for a search run: keywords, titles, locations, " +
			"compensation floor, company allow and block lists, and how long each pipeline status " +
			"suppresses a job from being surfaced again. Call this first so the search uses current " +
			"criteria rather than anything hardcoded in a prompt. Set start_run to also open a run to log against.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getProfileArgs) (*mcp.CallToolResult, profileOutput, error) {
		return call(func() (profileOutput, error) {
			if args.Name == "" {
				profiles, err := s.svc.ListProfiles(ctx, p, true)
				if err != nil {
					return profileOutput{}, err
				}
				names := make([]string, len(profiles))
				for i, profile := range profiles {
					names[i] = profile.Name
				}
				out := emptyProfileOutput()
				out.AvailableProfiles = names
				return out, nil
			}

			view, err := s.svc.GetProfile(ctx, p, args.Name)
			if err != nil {
				return profileOutput{}, err
			}

			out := profileOutput{
				Name:             view.Profile.Name,
				Description:      view.Profile.Description,
				Active:           view.Profile.Active,
				Keywords:         orEmpty(view.Profile.Keywords),
				Titles:           orEmpty(view.Profile.Titles),
				Locations:        orEmpty(view.Profile.Locations),
				RemoteOnly:       view.Profile.RemoteOnly,
				MinCompensation:  view.Profile.MinCompensation,
				Seniority:        seniorityStrings(view.Profile.Seniority),
				CompanyAllowlist: orEmpty(view.Profile.CompanyAllowlist),
				CompanyBlocklist: orEmpty(view.Profile.CompanyBlocklist),
				ExcludeKeywords:  orEmpty(view.Profile.ExcludeKeywords),
				ResurfacePolicy:  policyStrings(view.EffectiveResurfacePolicy),
				RunsToday:        view.RunsToday,
			}
			if view.LastRunAt != nil {
				out.LastRunAt = view.LastRunAt.Format(time.RFC3339)
			}

			if args.StartRun {
				run, err := s.svc.StartRun(ctx, p, args.Name, domain.SourceUnknown, args.Query, "")
				if err != nil {
					return profileOutput{}, err
				}
				out.SearchRunID = run.ID.String()
			}
			return out, nil
		})
	})
}

// emptyProfileOutput returns an output whose collections are empty rather than
// nil.
//
// A nil Go map or slice marshals to JSON null, and the tool's output schema is
// inferred from the struct, so null fails validation and the whole call errors
// out. Every partially populated result has to go through here.
func emptyProfileOutput() profileOutput {
	return profileOutput{
		Keywords:         []string{},
		Titles:           []string{},
		Locations:        []string{},
		Seniority:        []string{},
		CompanyAllowlist: []string{},
		CompanyBlocklist: []string{},
		ExcludeKeywords:  []string{},
		ResurfacePolicy:  map[string]int{},
	}
}

// --- 2. filter_new_postings -------------------------------------------------

type filterArgs struct {
	Candidates  []PostingInput `json:"candidates" jsonschema:"the postings you found, in the order you found them"`
	Profile     string         `json:"profile,omitempty" jsonschema:"profile whose resurfacing rules apply"`
	SearchRunID string         `json:"search_run_id,omitempty" jsonschema:"run to log these counts against"`
}

type filterOutput struct {
	Results  []VerdictOutput `json:"results" jsonschema:"one entry per candidate, correlated by index"`
	Summary  service.Summary `json:"summary"`
	Guidance string          `json:"guidance"`
}

func (s *Server) addFilterNewPostings(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "filter_new_postings",
		Description: "Check a batch of postings against everything already tracked, and get back a " +
			"verdict for each: new, duplicate, eligible_again, or needs_review. This is the main tool. " +
			"Call it before pursuing or recording anything. It changes nothing and is safe to retry. " +
			"Results correlate to the request by index.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args filterArgs) (*mcp.CallToolResult, filterOutput, error) {
		return call(func() (filterOutput, error) {
			runID, err := optionalID("search_run_id", args.SearchRunID)
			if err != nil {
				return filterOutput{}, err
			}

			candidates, err := toCandidates(args.Candidates)
			if err != nil {
				return filterOutput{}, err
			}
			resp, err := s.svc.FilterPostings(ctx, p, service.FilterRequest{
				ProfileName: args.Profile,
				SearchRunID: runID,
				Candidates:  candidates,
			})
			if err != nil {
				return filterOutput{}, err
			}

			results := make([]VerdictOutput, len(resp.Results))
			for i, r := range resp.Results {
				results[i] = toVerdict(r)
			}
			return filterOutput{
				Results:  results,
				Summary:  resp.Summary,
				Guidance: guidanceFor(resp.Summary),
			}, nil
		})
	})
}

// guidanceFor states the next step in words, because a summary alone leaves the
// needs_review case ambiguous and that is the case where a wrong move costs
// a duplicate application.
func guidanceFor(summary service.Summary) string {
	switch {
	case summary.NeedsReview > 0:
		return "Some candidates need review: read their reason and posting_id and decide yourself. " +
			"Record the ones you judge new with record_discoveries; skip the rest."
	case summary.New+summary.EligibleAgain > 0:
		return "Record the new and eligible_again candidates with record_discoveries."
	default:
		return "Nothing new in this batch."
	}
}

// --- 3. record_discoveries --------------------------------------------------

type recordDiscoveriesArgs struct {
	Postings    []PostingInput `json:"postings" jsonschema:"postings to remember"`
	Profile     string         `json:"profile,omitempty"`
	SearchRunID string         `json:"search_run_id,omitempty"`
}

type discoveryOutput struct {
	VerdictOutput
	Created bool `json:"created" jsonschema:"true when this posting was recorded for the first time"`
}

type recordDiscoveriesOutput struct {
	Results []discoveryOutput `json:"results"`
	Summary service.Summary   `json:"summary"`
}

func (s *Server) addRecordDiscoveries(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "record_discoveries",
		Description: "Remember postings so future runs recognise them. Deduplicates again internally, " +
			"so passing a job that is already tracked is harmless and safe to retry. Candidates that " +
			"come back needs_review are reported but deliberately not recorded: decide those yourself first.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args recordDiscoveriesArgs) (*mcp.CallToolResult, recordDiscoveriesOutput, error) {
		return call(func() (recordDiscoveriesOutput, error) {
			runID, err := optionalID("search_run_id", args.SearchRunID)
			if err != nil {
				return recordDiscoveriesOutput{}, err
			}

			candidates, err := toCandidates(args.Postings)
			if err != nil {
				return recordDiscoveriesOutput{}, err
			}
			resp, err := s.svc.IngestPostings(ctx, p, service.IngestRequest{
				ProfileName: args.Profile,
				SearchRunID: runID,
				Candidates:  candidates,
			})
			if err != nil {
				return recordDiscoveriesOutput{}, err
			}

			results := make([]discoveryOutput, len(resp.Results))
			for i, r := range resp.Results {
				results[i] = discoveryOutput{VerdictOutput: toVerdict(r.FilterResult), Created: r.Created}
			}
			return recordDiscoveriesOutput{Results: results, Summary: resp.Summary}, nil
		})
	})
}

// --- 4. record_application --------------------------------------------------

type recordApplicationArgs struct {
	PostingID string        `json:"posting_id,omitempty" jsonschema:"id of an already tracked posting"`
	Posting   *PostingInput `json:"posting,omitempty" jsonschema:"the job inline, if it is not tracked yet; supply this or posting_id"`

	AppliedAt          string `json:"applied_at,omitempty" jsonschema:"when you applied, RFC3339; defaults to now"`
	ResumeVariant      string `json:"resume_variant,omitempty" jsonschema:"label of the resume you sent, so response rates can be compared later"`
	ResumeURL          string `json:"resume_url,omitempty"`
	CoverLetterVariant string `json:"cover_letter_variant,omitempty"`
	Notes              string `json:"notes,omitempty"`
	FollowUpInDays     int    `json:"follow_up_in_days,omitempty" jsonschema:"days to wait before this shows up in get_followups_due; defaults to 14"`
}

type applicationOutput struct {
	ID            string `json:"id"`
	PostingID     string `json:"posting_id"`
	Status        string `json:"status"`
	Company       string `json:"company,omitempty"`
	Title         string `json:"title,omitempty"`
	URL           string `json:"url,omitempty"`
	AppliedAt     string `json:"applied_at,omitempty"`
	NextActionAt  string `json:"next_action_at,omitempty"`
	ResumeVariant string `json:"resume_variant,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

func toApplicationOutput(a domain.Application) applicationOutput {
	out := applicationOutput{
		ID:            a.ID.String(),
		PostingID:     a.PostingID.String(),
		Status:        string(a.Status),
		ResumeVariant: a.ResumeVariant,
		Notes:         a.Notes,
	}
	if a.AppliedAt != nil {
		out.AppliedAt = a.AppliedAt.Format(time.RFC3339)
	}
	if a.NextActionAt != nil {
		out.NextActionAt = a.NextActionAt.Format(time.RFC3339)
	}
	return out
}

func (s *Server) addRecordApplication(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "record_application",
		Description: "Record that you applied to a job. Accepts either a tracked posting_id or the " +
			"posting inline, so there is no need to record a discovery first. Sets a follow-up date, " +
			"and from then on this job will come back as a duplicate from filter_new_postings forever.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args recordApplicationArgs) (*mcp.CallToolResult, applicationOutput, error) {
		return call(func() (applicationOutput, error) {
			req := service.RecordApplicationRequest{
				ResumeVariant:      args.ResumeVariant,
				ResumeURL:          args.ResumeURL,
				CoverLetterVariant: args.CoverLetterVariant,
				Notes:              args.Notes,
				FollowUpInDays:     args.FollowUpInDays,
			}
			postingID, err := optionalID("posting_id", args.PostingID)
			if err != nil {
				return applicationOutput{}, err
			}
			req.PostingID = postingID
			if args.Posting != nil {
				candidate, err := args.Posting.toDomain()
				if err != nil {
					return applicationOutput{}, err
				}
				req.Candidate = &candidate
			}
			appliedAt, err := parseTimeArg("applied_at", args.AppliedAt)
			if err != nil {
				return applicationOutput{}, err
			}
			req.AppliedAt = appliedAt

			application, err := s.svc.RecordApplication(ctx, p, req)
			if err != nil {
				return applicationOutput{}, err
			}
			return toApplicationOutput(application), nil
		})
	})
}

// --- 5. update_application_status -------------------------------------------

type updateStatusArgs struct {
	ApplicationID   string `json:"application_id"`
	Status          string `json:"status" jsonschema:"discovered, screened_out, queued, applied, screening, interview, offer, accepted, rejected, ghosted, or withdrawn"`
	OccurredAt      string `json:"occurred_at,omitempty" jsonschema:"when this happened, RFC3339; defaults to now"`
	Note            string `json:"note,omitempty" jsonschema:"what happened, kept on the timeline"`
	NextActionAt    string `json:"next_action_at,omitempty" jsonschema:"when to chase next, RFC3339"`
	ClearNextAction bool   `json:"clear_next_action,omitempty" jsonschema:"remove any scheduled follow-up without setting a new one; terminal statuses do this automatically"`
}

func (s *Server) addUpdateApplicationStatus(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "update_application_status",
		Description: "Move an application through the pipeline and record the change on its timeline. " +
			"Only valid transitions are accepted; an invalid one comes back as an error naming what is " +
			"allowed from the current status. Terminal statuses clear the follow-up automatically.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args updateStatusArgs) (*mcp.CallToolResult, applicationOutput, error) {
		return call(func() (applicationOutput, error) {
			id, err := domain.ParseID("application_id", args.ApplicationID)
			if err != nil {
				return applicationOutput{}, err
			}

			req := service.UpdateStatusRequest{
				ApplicationID:   id,
				Status:          domain.Status(args.Status),
				Note:            args.Note,
				ClearNextAction: args.ClearNextAction,
			}
			occurredAt, err := parseTimeArg("occurred_at", args.OccurredAt)
			if err != nil {
				return applicationOutput{}, err
			}
			req.OccurredAt = occurredAt
			nextAction, err := parseTimeArg("next_action_at", args.NextActionAt)
			if err != nil {
				return applicationOutput{}, err
			}
			req.NextActionAt = nextAction

			application, err := s.svc.UpdateStatus(ctx, p, req)
			if err != nil {
				return applicationOutput{}, err
			}
			return toApplicationOutput(application), nil
		})
	})
}

// --- 6. list_applications ---------------------------------------------------

type listApplicationsArgs struct {
	Statuses []string `json:"statuses,omitempty" jsonschema:"filter to these pipeline statuses"`
	Company  string   `json:"company,omitempty" jsonschema:"filter to one employer by name"`
	Since    string   `json:"since,omitempty" jsonschema:"only applications tracked since this time, RFC3339"`
	Limit    int      `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 50, capped at 500"`
}

type listApplicationsOutput struct {
	Applications []applicationOutput `json:"applications"`
	Count        int                 `json:"count"`
}

func (s *Server) addListApplications(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_applications",
		Description: "List tracked jobs and where each one stands. Use it to answer questions about the " +
			"pipeline, or to find an application_id before updating one.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listApplicationsArgs) (*mcp.CallToolResult, listApplicationsOutput, error) {
		return call(func() (listApplicationsOutput, error) {
			filter := store.ApplicationListFilter{
				CompanyNormalizedName: args.Company,
				Limit:                 args.Limit,
			}
			for _, status := range args.Statuses {
				filter.Statuses = append(filter.Statuses, domain.Status(status))
			}
			since, err := parseTimeArg("since", args.Since)
			if err != nil {
				return listApplicationsOutput{}, err
			}
			filter.Since = since

			rows, err := s.svc.ListApplications(ctx, p, filter)
			if err != nil {
				return listApplicationsOutput{}, err
			}

			out := listApplicationsOutput{
				Applications: make([]applicationOutput, len(rows)),
				Count:        len(rows),
			}
			for i, row := range rows {
				app := toApplicationOutput(row.Application)
				app.Company = row.CompanyName
				app.Title = row.PostingTitle
				app.URL = row.PostingURL
				out.Applications[i] = app
			}
			return out, nil
		})
	})
}

// --- 7. get_followups_due ---------------------------------------------------

type followupsArgs struct {
	AsOf  string `json:"as_of,omitempty" jsonschema:"treat this time as now, RFC3339"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 50, capped at 500"`
}

type followupOutput struct {
	applicationOutput
	DaysSinceApplied int `json:"days_since_applied"`
	DaysOverdue      int `json:"days_overdue"`
}

type followupsOutput struct {
	Followups []followupOutput `json:"followups"`
	Count     int              `json:"count"`
}

func (s *Server) addGetFollowupsDue(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_followups_due",
		Description: "List applications that have gone quiet long enough to be worth chasing, with how " +
			"many days have passed since applying. Terminal applications never appear here.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args followupsArgs) (*mcp.CallToolResult, followupsOutput, error) {
		return call(func() (followupsOutput, error) {
			asOf, err := parseTimeArg("as_of", args.AsOf)
			if err != nil {
				return followupsOutput{}, err
			}

			due, err := s.svc.GetFollowupsDue(ctx, p, asOf, args.Limit)
			if err != nil {
				return followupsOutput{}, err
			}

			out := followupsOutput{Followups: make([]followupOutput, len(due)), Count: len(due)}
			for i, f := range due {
				app := toApplicationOutput(f.Application)
				app.Company = f.CompanyName
				app.Title = f.PostingTitle
				app.URL = f.PostingURL
				out.Followups[i] = followupOutput{
					applicationOutput: app,
					DaysSinceApplied:  f.DaysSinceApplied,
					DaysOverdue:       f.DaysOverdue,
				}
			}
			return out, nil
		})
	})
}

// --- 8. add_note ------------------------------------------------------------

type addNoteArgs struct {
	ApplicationID string `json:"application_id"`
	Note          string `json:"note" jsonschema:"what to remember about this application"`
}

func (s *Server) addAddNote(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "add_note",
		Description: "Append a dated note to an application. Notes accumulate rather than overwrite, " +
			"so context builds up across runs.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args addNoteArgs) (*mcp.CallToolResult, applicationOutput, error) {
		return call(func() (applicationOutput, error) {
			id, err := domain.ParseID("application_id", args.ApplicationID)
			if err != nil {
				return applicationOutput{}, err
			}
			application, err := s.svc.AddNote(ctx, p, id, args.Note)
			if err != nil {
				return applicationOutput{}, err
			}
			return toApplicationOutput(application), nil
		})
	})
}

// --- 9. upsert_contact ------------------------------------------------------

type upsertContactArgs struct {
	Name          string `json:"name,omitempty"`
	Role          string `json:"role,omitempty" jsonschema:"their role, e.g. recruiter or hiring manager"`
	Email         string `json:"email,omitempty" jsonschema:"used as the identity key when present"`
	LinkedInURL   string `json:"linkedin_url,omitempty"`
	Notes         string `json:"notes,omitempty"`
	Company       string `json:"company,omitempty" jsonschema:"employer they work for"`
	ApplicationID string `json:"application_id,omitempty" jsonschema:"link them to this application and log it on its timeline"`
	LastContactAt string `json:"last_contact_at,omitempty" jsonschema:"when you last spoke, RFC3339"`
}

type contactOutput struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Role          string `json:"role,omitempty"`
	Email         string `json:"email,omitempty"`
	LastContactAt string `json:"last_contact_at,omitempty"`
}

func (s *Server) addUpsertContact(srv *mcp.Server, p domain.Principal) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "upsert_contact",
		Description: "Record a recruiter or hiring manager, optionally linked to an application. " +
			"Matching is by email: give one and a repeated call updates that one record. Without an " +
			"email there is no reliable way to recognise the same person again, so each call creates a " +
			"new contact — pass one whenever you might call this again for the same person.",
		// Not IdempotentHint: true is not always accurate here. It only holds
		// when email is given; a name-only contact inserts a new row every
		// call, and the annotation has no way to say "sometimes".
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args upsertContactArgs) (*mcp.CallToolResult, contactOutput, error) {
		return call(func() (contactOutput, error) {
			req := service.UpsertContactRequest{
				Contact: domain.Contact{
					Name:        args.Name,
					Role:        args.Role,
					Email:       args.Email,
					LinkedInURL: args.LinkedInURL,
					Notes:       args.Notes,
				},
				CompanyName: args.Company,
			}
			applicationID, err := optionalID("application_id", args.ApplicationID)
			if err != nil {
				return contactOutput{}, err
			}
			req.LinkToApplicationID = applicationID
			lastContact, err := parseTimeArg("last_contact_at", args.LastContactAt)
			if err != nil {
				return contactOutput{}, err
			}
			req.Contact.LastContactAt = lastContact

			contact, err := s.svc.UpsertContact(ctx, p, req)
			if err != nil {
				return contactOutput{}, err
			}

			out := contactOutput{
				ID:    contact.ID.String(),
				Name:  contact.Name,
				Role:  contact.Role,
				Email: contact.Email,
			}
			if contact.LastContactAt != nil {
				out.LastContactAt = contact.LastContactAt.Format(time.RFC3339)
			}
			return out, nil
		})
	})
}

// --- helpers ----------------------------------------------------------------

// parseTimeArg accepts RFC3339 and a bare date, because models write both.
// An empty value means "not given" and returns (nil, nil). A value that is
// present but unparseable is a caller mistake and returns an error — the
// REST transport already 400s on a malformed since/due_before, and MCP used
// to silently treat the same input as absent, which the two surfaces
// disagreeing about is worse than either behaviour on its own.
func parseTimeArg(field, raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed, nil
	}
	if parsed, err := time.Parse(time.DateOnly, raw); err == nil {
		return &parsed, nil
	}
	return nil, domain.NewValidationError(field + " must be RFC3339 or YYYY-MM-DD")
}

// optionalID parses an argument that is an id when present. field names which
// argument failed, so "search_run_id must be a UUID" does not get reported as
// "id must be a UUID" regardless of which field was actually wrong.
func optionalID(field, raw string) (*domain.ID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := domain.ParseID(field, raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func seniorityStrings(levels []domain.Seniority) []string {
	out := make([]string, len(levels))
	for i, level := range levels {
		out[i] = string(level)
	}
	return out
}

func policyStrings(policy map[domain.Status]int) map[string]int {
	out := make(map[string]int, len(policy))
	for status, days := range policy {
		out[string(status)] = days
	}
	return out
}
