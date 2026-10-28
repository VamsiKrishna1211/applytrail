// Package service holds every use case ApplyTrail supports.
//
// This is the only layer that contains business rules. The REST handlers and
// the MCP tools are both thin translators over it, which is what guarantees an
// agent and a curl command get identical behaviour rather than two
// implementations that drift.
package service

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// MatcherFactory builds a match strategy bound to a particular set of lookups.
//
// It is a factory rather than a ready-made Matcher because ingest runs inside a
// transaction: the engine has to read through the same transaction it writes
// to, or it would fail to see postings created moments earlier in the batch.
type MatcherFactory func(postings dedupe.Lookup, companies dedupe.CompanyLookup) dedupe.Matcher

// Service is the use-case layer.
type Service struct {
	store   store.Store
	matcher MatcherFactory
	now     func() time.Time
}

// Options configure a Service. Every field has a working default.
type Options struct {
	// Dedupe tunes the built-in match engine. Ignored when Matcher is set.
	Dedupe dedupe.Config
	// Matcher replaces the built-in engine, for a deployment that wants a
	// different match strategy.
	Matcher MatcherFactory
	// Now overrides the clock, for tests.
	Now func() time.Time
}

// New returns a Service.
func New(s store.Store, opts Options) *Service {
	matcher := opts.Matcher
	if matcher == nil {
		cfg := opts.Dedupe
		matcher = func(postings dedupe.Lookup, companies dedupe.CompanyLookup) dedupe.Matcher {
			return dedupe.NewEngine(postings, companies, cfg)
		}
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: s, matcher: matcher, now: now}
}

// matcherFor builds a matcher reading through st, which may be a transaction.
func (s *Service) matcherFor(st store.Store) dedupe.Matcher {
	engine := s.matcher(st.Postings(), st.Companies())
	if e, ok := engine.(*dedupe.Engine); ok {
		// Keep the engine on the same clock as the service, so a test that
		// freezes time sees consistent cooldown arithmetic.
		e.WithClock(s.now)
	}
	return engine
}

// actorOf names who caused a change, for the event timeline. The key label is
// more useful than its ID: "search-agent" reads better in a timeline than a
// UUID, and it is what the operator chose.
func actorOf(p domain.Principal) string {
	if p.KeyLabel != "" {
		return p.KeyLabel
	}
	return "api"
}

// resolvePolicy picks the resurfacing rules for a call: an explicit per-run
// override, else the named profile's, else the defaults.
//
// Overrides are not persisted. A caller experimenting with a shorter cooldown
// for one run should not silently change the profile for every later run.
func (s *Service) resolvePolicy(ctx context.Context, st store.Store, tenantID domain.ID, profileName string, override *domain.ResurfacePolicy) (domain.ResurfacePolicy, string, error) {
	if override != nil {
		if err := override.Validate(); err != nil {
			return domain.ResurfacePolicy{}, "", err
		}
		if profileName == "" {
			return *override, "", nil
		}
		// ProfileApplied must name a profile that actually exists: a caller
		// combining a typo'd profile name with an override would otherwise
		// see that name echoed back as if it had been consulted, when in
		// fact only the override's rules ran.
		profile, err := st.SearchProfiles().GetByName(ctx, tenantID, profileName)
		if err != nil {
			return domain.ResurfacePolicy{}, "", err
		}
		return *override, profile.Name, nil
	}
	if profileName == "" {
		return domain.DefaultResurfacePolicy(), "", nil
	}

	profile, err := st.SearchProfiles().GetByName(ctx, tenantID, profileName)
	if err != nil {
		return domain.ResurfacePolicy{}, "", err
	}
	return profile.ResurfacePolicy, profile.Name, nil
}
