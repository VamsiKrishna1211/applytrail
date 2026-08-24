# Driving ApplyTrail from a Claude Code cloud routine

## Connect

```bash
claude mcp add --transport http applytrail https://<your-host>/mcp \
  --header "Authorization: Bearer at_live_..."
```

Mint the key with `applytrail apikey create --label search-agent --scopes read,write`.

`read` alone is enough to *check* for duplicates — batch dedupe changes nothing.
Give the routine `write` only because it also records what it finds.

## An example routine prompt

```
Run my job search.

1. Call get_search_profile with name "backend-remote" and start_run true.
   Use the returned keywords, titles, locations, and compensation floor as your
   search criteria. Keep the search_run_id for later calls.

2. Search LinkedIn, Greenhouse, Lever, and Ashby for matching roles. Collect at
   least 20 candidates. For each, capture the URL, title, company, location, and
   the posting description if it is cheap to fetch — descriptions let ApplyTrail
   catch the same job cross-posted under a different title.

3. Call filter_new_postings with every candidate and the search_run_id.

4. Handle each verdict:
   - new             pursue it
   - eligible_again  pursue it; mention in your summary that it resurfaced
   - duplicate       skip silently
   - needs_review    read the reason and decide yourself. It names the words
                     that differed. If it is genuinely a different role, treat
                     it as new. If unsure, treat it as a duplicate and say so.

   Never treat needs_review as automatically new. That is how a duplicate
   application gets sent.

5. Call record_discoveries with everything you judged new or eligible_again.

6. Call get_followups_due. For anything more than 21 days overdue with no reply,
   add a note saying you flagged it, and consider marking it ghosted.

7. Summarise: how many candidates you saw, how many were new, which ones you
   would apply to first and why, and anything that needed review.
```

## After you actually apply

```
Call record_application with the posting_id (or the posting inline) and the
resume_variant you sent. That job will never surface as new again, and it enters
the follow-up queue.
```

Keeping `resume_variant` accurate is what makes `GET /v1/stats` able to compare
response rates between resume versions. It is the whole reason the field exists.

## Scheduling it

Any scheduler works — ApplyTrail is pull-only and holds no timers of its own.
With Claude Code:

```bash
claude schedule create --cron "0 9 * * 1-5" --prompt-file job-search.md
```

## Notes on behaviour

**Everything is safe to retry.** `filter_new_postings` changes nothing.
`record_discoveries` and `record_application` deduplicate internally, so a
retried call after a timeout does not create a second record.

**Results correlate by index.** `results[i]` answers `candidates[i]`. No
correlation IDs to thread through.

**Errors are actionable.** A rejected status transition says what is allowed
from the current status, so the model can correct itself rather than retrying
the same call.

**Batches cap at 500 candidates.** Split larger sweeps; the counts accumulate
onto the same `search_run_id`.
