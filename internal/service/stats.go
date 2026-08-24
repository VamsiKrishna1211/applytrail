package service

import (
	"context"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// Stats is the funnel report.
type Stats struct {
	// ByStatus holds every status, including those with a zero count, so a
	// caller rendering a funnel does not have to know the status list.
	ByStatus []store.StatusCount
	// ResumeVariants reports how each resume performed. This is the question
	// the resume-variant field exists to answer.
	ResumeVariants []ResumeVariantStat

	TotalTracked int
	TotalApplied int
	// ResponseRate is responses over applications sent, on [0, 1]. It is zero
	// when nothing has been applied to yet.
	ResponseRate float64
}

// ResumeVariantStat is one resume's performance.
type ResumeVariantStat struct {
	store.ResumeVariantStat
	// ResponseRate is responded over sent, on [0, 1].
	ResponseRate float64
}

// GetStats returns the funnel and per-resume response rates.
func (s *Service) GetStats(ctx context.Context, p domain.Principal) (Stats, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return Stats{}, err
	}

	counts, err := s.store.Applications().CountByStatus(ctx, p.TenantID)
	if err != nil {
		return Stats{}, err
	}
	variants, err := s.store.Applications().ResumeVariantStats(ctx, p.TenantID)
	if err != nil {
		return Stats{}, err
	}

	byStatus := make(map[domain.Status]int, len(counts))
	for _, c := range counts {
		byStatus[c.Status] = c.Count
	}

	stats := Stats{ByStatus: make([]store.StatusCount, 0, len(domain.AllStatuses))}
	for _, status := range domain.AllStatuses {
		count := byStatus[status]
		stats.ByStatus = append(stats.ByStatus, store.StatusCount{Status: status, Count: count})
		stats.TotalTracked += count
	}

	var responded int
	for _, v := range variants {
		stats.TotalApplied += v.Sent
		responded += v.Responded

		stat := ResumeVariantStat{ResumeVariantStat: v}
		if v.Sent > 0 {
			stat.ResponseRate = float64(v.Responded) / float64(v.Sent)
		}
		stats.ResumeVariants = append(stats.ResumeVariants, stat)
	}
	if stats.TotalApplied > 0 {
		stats.ResponseRate = float64(responded) / float64(stats.TotalApplied)
	}

	return stats, nil
}
