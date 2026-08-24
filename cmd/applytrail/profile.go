package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

func runProfile(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: applytrail profile [create|list|show] [flags]")
	}
	switch args[0] {
	case "create":
		return runProfileCreate(ctx, args[1:])
	case "list":
		return runProfileList(ctx, args[1:])
	case "show":
		return runProfileShow(ctx, args[1:])
	default:
		return fmt.Errorf("unknown profile action %q; expected create, list, or show", args[0])
	}
}

func runProfileCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail profile create", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	tenantSlug := fs.String("tenant", "", "tenant slug; optional while only one tenant exists")

	name := fs.String("name", "", "unique profile name, e.g. backend-remote")
	description := fs.String("description", "", "what this profile targets")
	keywords := fs.String("keywords", "", "comma-separated search keywords")
	titles := fs.String("titles", "", "comma-separated target job titles")
	locations := fs.String("locations", "", "comma-separated target locations")
	seniority := fs.String("seniority", "", "comma-separated levels: junior, mid, senior, staff, principal, lead, manager")
	remoteOnly := fs.Bool("remote-only", false, "only consider fully remote roles")
	minComp := fs.Int64("min-comp", 0, "minimum acceptable annual compensation")
	currency := fs.String("currency", "", "ISO currency code for --min-comp")
	allowlist := fs.String("allow-companies", "", "comma-separated employers to consider exclusively")
	blocklist := fs.String("block-companies", "", "comma-separated employers to skip")
	exclude := fs.String("exclude-keywords", "", "comma-separated keywords that disqualify a posting")

	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: applytrail profile create --name <name> [flags]\n\n"+
			"Search criteria live server-side so they can be tuned by editing data rather than\n"+
			"by editing an agent's prompt.\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	profile := domain.SearchProfile{
		Name:             *name,
		Description:      *description,
		Active:           true,
		Keywords:         splitCSV(*keywords),
		Titles:           splitCSV(*titles),
		Locations:        splitCSV(*locations),
		RemoteOnly:       *remoteOnly,
		Currency:         *currency,
		CompanyAllowlist: splitCSV(*allowlist),
		CompanyBlocklist: splitCSV(*blocklist),
		ExcludeKeywords:  splitCSV(*exclude),
	}
	if *minComp > 0 {
		profile.MinCompensation = minComp
	}
	for _, level := range splitCSV(*seniority) {
		profile.Seniority = append(profile.Seniority, domain.Seniority(level))
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		tenant, err := resolveTenant(ctx, st, *tenantSlug)
		if err != nil {
			return err
		}

		created, err := service.New(st, service.Options{}).
			CreateProfile(ctx, adminPrincipal(tenant.ID), profile)
		if err != nil {
			return err
		}
		fmt.Printf("profile %q created (%s)\n", created.Name, created.ID)
		fmt.Printf("\nAn agent fetches it with the get_search_profile tool, or:\n")
		fmt.Printf("  GET /v1/search-profiles/%s\n", created.Name)
		return nil
	})
}

func runProfileList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail profile list", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	tenantSlug := fs.String("tenant", "", "tenant slug; optional while only one tenant exists")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		tenant, err := resolveTenant(ctx, st, *tenantSlug)
		if err != nil {
			return err
		}

		profiles, err := service.New(st, service.Options{}).
			ListProfiles(ctx, adminPrincipal(tenant.ID), false)
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			fmt.Println("no profiles yet; create one with `applytrail profile create --name <name>`")
			return nil
		}

		w := newTabWriter()
		fmt.Fprintln(w, "NAME\tACTIVE\tKEYWORDS\tLOCATIONS")
		for _, p := range profiles {
			fmt.Fprintf(w, "%s\t%t\t%s\t%s\n",
				p.Name, p.Active, orDash(strings.Join(p.Keywords, ",")), orDash(strings.Join(p.Locations, ",")))
		}
		return w.Flush()
	})
}

func runProfileShow(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail profile show", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	tenantSlug := fs.String("tenant", "", "tenant slug; optional while only one tenant exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: applytrail profile show <name>")
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		tenant, err := resolveTenant(ctx, st, *tenantSlug)
		if err != nil {
			return err
		}

		view, err := service.New(st, service.Options{}).
			GetProfile(ctx, adminPrincipal(tenant.ID), fs.Arg(0))
		if err != nil {
			return err
		}

		w := newTabWriter()
		fmt.Fprintf(w, "name\t%s\n", view.Profile.Name)
		fmt.Fprintf(w, "active\t%t\n", view.Profile.Active)
		fmt.Fprintf(w, "keywords\t%s\n", orDash(strings.Join(view.Profile.Keywords, ", ")))
		fmt.Fprintf(w, "titles\t%s\n", orDash(strings.Join(view.Profile.Titles, ", ")))
		fmt.Fprintf(w, "locations\t%s\n", orDash(strings.Join(view.Profile.Locations, ", ")))
		fmt.Fprintf(w, "remote only\t%t\n", view.Profile.RemoteOnly)
		fmt.Fprintf(w, "runs today\t%d\n", view.RunsToday)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "RESURFACE AFTER\t")

		// Sorted, because a map iteration order that changes between runs makes
		// the output impossible to diff.
		statuses := make([]string, 0, len(view.EffectiveResurfacePolicy))
		for status := range view.EffectiveResurfacePolicy {
			statuses = append(statuses, string(status))
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			days := view.EffectiveResurfacePolicy[domain.Status(status)]
			if days == domain.ResurfaceNever {
				fmt.Fprintf(w, "  %s\tnever\n", status)
				continue
			}
			fmt.Fprintf(w, "  %s\t%d days\n", status, days)
		}
		return w.Flush()
	})
}
