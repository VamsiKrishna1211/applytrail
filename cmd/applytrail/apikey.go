package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

func runBootstrap(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail bootstrap", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	name := fs.String("name", "default", "human-readable tenant name")
	slug := fs.String("slug", "default", "unique tenant slug")
	label := fs.String("label", "bootstrap", "label for the admin key")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: applytrail bootstrap [flags]\n\n"+
			"Creates the first tenant and an admin key. Run this once, after `applytrail migrate up`.\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		if existing, err := st.Tenants().List(ctx); err == nil && len(existing) > 0 {
			return fmt.Errorf("this deployment is already bootstrapped (%d tenant(s)); "+
				"use `applytrail apikey create` to issue further credentials", len(existing))
		}

		tenant, err := st.Tenants().Create(ctx, *name, *slug)
		if err != nil {
			return err
		}

		svc := service.New(st, service.Options{})
		issued, err := svc.CreateAPIKey(ctx, adminPrincipal(tenant.ID), *label,
			[]domain.Scope{domain.ScopeAdmin}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("tenant %q created (%s)\n\n", tenant.Slug, tenant.ID)
		printSecret(issued)
		return nil
	})
}

func runAPIKey(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: applytrail apikey [create|list|revoke] [flags]")
	}
	switch args[0] {
	case "create":
		return runAPIKeyCreate(ctx, args[1:])
	case "list":
		return runAPIKeyList(ctx, args[1:])
	case "revoke":
		return runAPIKeyRevoke(ctx, args[1:])
	default:
		return fmt.Errorf("unknown apikey action %q; expected create, list, or revoke", args[0])
	}
}

func runAPIKeyCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail apikey create", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	tenantSlug := fs.String("tenant", "", "tenant slug; optional while only one tenant exists")
	label := fs.String("label", "", "what this key is for, e.g. search-agent")
	scopes := fs.String("scopes", "read,write", "comma-separated scopes: read, write, admin")
	expires := fs.String("expires", "", "expiry date, YYYY-MM-DD; omit for a key that does not expire")
	if err := fs.Parse(args); err != nil {
		return err
	}

	parsed, err := domain.ParseScopes(splitCSV(*scopes))
	if err != nil {
		return err
	}

	var expiresAt *time.Time
	if *expires != "" {
		at, err := time.Parse(time.DateOnly, *expires)
		if err != nil {
			return fmt.Errorf("--expires must be YYYY-MM-DD: %w", err)
		}
		expiresAt = &at
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		tenant, err := resolveTenant(ctx, st, *tenantSlug)
		if err != nil {
			return err
		}

		svc := service.New(st, service.Options{})
		issued, err := svc.CreateAPIKey(ctx, adminPrincipal(tenant.ID), *label, parsed, expiresAt)
		if err != nil {
			return err
		}
		printSecret(issued)
		return nil
	})
}

func runAPIKeyList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail apikey list", flag.ExitOnError)
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

		keys, err := service.New(st, service.Options{}).ListAPIKeys(ctx, adminPrincipal(tenant.ID))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Println("no keys yet")
			return nil
		}

		w := newTabWriter()
		fmt.Fprintln(w, "ID\tPREFIX\tLABEL\tSCOPES\tSTATUS\tLAST USED")
		for _, key := range keys {
			scopeNames := make([]string, len(key.Scopes))
			for i, scope := range key.Scopes {
				scopeNames[i] = string(scope)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				key.ID, key.Prefix, orDash(key.Label), strings.Join(scopeNames, ","),
				keyStatus(key), formatTime(key.LastUsedAt))
		}
		return w.Flush()
	})
}

func runAPIKeyRevoke(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail apikey revoke", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	tenantSlug := fs.String("tenant", "", "tenant slug; optional while only one tenant exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: applytrail apikey revoke <key-id>")
	}

	id, err := domain.ParseID("key", fs.Arg(0))
	if err != nil {
		return err
	}

	return withStore(ctx, *databaseURL, func(ctx context.Context, st store.Store) error {
		tenant, err := resolveTenant(ctx, st, *tenantSlug)
		if err != nil {
			return err
		}

		key, err := service.New(st, service.Options{}).RevokeAPIKey(ctx, adminPrincipal(tenant.ID), id)
		if err != nil {
			return err
		}
		fmt.Printf("revoked %s (%s)\n", key.Prefix, orDash(key.Label))
		return nil
	})
}

// printSecret shows a new key with the warning it needs: only the hash is
// stored, so this is the only time the secret exists anywhere but the caller's
// terminal.
func printSecret(issued service.IssuedKey) {
	fmt.Printf("API key created: %s\n\n", orDash(issued.Key.Label))
	fmt.Printf("  %s\n\n", issued.Secret)
	fmt.Println("Store it now. Only a hash is kept, so it cannot be shown again.")
	fmt.Println()
	fmt.Println("Connect an agent with:")
	fmt.Printf("  claude mcp add --transport http applytrail https://<your-host>/mcp \\\n")
	fmt.Printf("    --header \"Authorization: Bearer %s\"\n", issued.Secret)
}

func keyStatus(key domain.APIKey) string {
	switch {
	case key.RevokedAt != nil:
		return "revoked"
	case !key.Active(time.Now()):
		return "expired"
	default:
		return "active"
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Format(time.RFC3339)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
