package httpapi

import (
	"net/http"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
)

// handleFilterPostings is the hot path: given a page of search results, which
// of them are worth pursuing?
//
// It is a POST because the candidate list does not fit in a query string, but
// it is read-only and safe to retry.
func (h *handlers) handleFilterPostings(w http.ResponseWriter, r *http.Request) {
	var body filterRequestDTO
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	policy, err := policyFromMap(body.ResurfacePolicy)
	if err != nil {
		writeError(w, r, err)
		return
	}
	runID, err := optionalID("search_run_id", body.SearchRunID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp, err := h.svc.FilterPostings(r.Context(), principalOf(r), service.FilterRequest{
		ProfileName:       body.Profile,
		ResurfaceOverride: policy,
		SearchRunID:       runID,
		Candidates:        candidatesToDomain(body.Candidates),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	results := make([]resultDTO, len(resp.Results))
	for i, result := range resp.Results {
		results[i] = resultToDTO(result)
	}
	writeJSON(w, r, http.StatusOK, filterResponseDTO{
		Results:        results,
		Summary:        resp.Summary,
		ProfileApplied: resp.ProfileApplied,
	})
}

// handleIngestPostings records discoveries.
func (h *handlers) handleIngestPostings(w http.ResponseWriter, r *http.Request) {
	var body ingestRequestDTO
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	policy, err := policyFromMap(body.ResurfacePolicy)
	if err != nil {
		writeError(w, r, err)
		return
	}
	runID, err := optionalID("search_run_id", body.SearchRunID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp, err := h.svc.IngestPostings(r.Context(), principalOf(r), service.IngestRequest{
		ProfileName:       body.Profile,
		ResurfaceOverride: policy,
		SearchRunID:       runID,
		Candidates:        candidatesToDomain(body.Candidates),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	results := make([]ingestResultDTO, len(resp.Results))
	for i, result := range resp.Results {
		results[i] = ingestResultDTO{resultDTO: resultToDTO(result.FilterResult), Created: result.Created}
	}
	writeJSON(w, r, http.StatusOK, ingestResponseDTO{
		Results:        results,
		Summary:        resp.Summary,
		ProfileApplied: resp.ProfileApplied,
	})
}

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
