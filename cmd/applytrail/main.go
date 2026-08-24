// Command applytrail administers an ApplyTrail deployment: schema migrations,
// tenants, credentials, and search profiles.
//
// These operations are deliberately not exposed over the API. Bootstrapping a
// deployment has to work before any credential exists, and schema changes
// should be a deliberate operator action rather than something a leaked key
// can trigger.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `applytrail - administer an ApplyTrail deployment

usage: applytrail <command> [flags]

commands:
  migrate     apply, roll back, or inspect schema migrations
  bootstrap   create the first tenant and an admin key
  apikey      create, list, and revoke credentials
  profile     manage search profiles
  version     print the build version

Every command reads the database URL from APPLYTRAIL_DATABASE_URL, or from
--database-url. Run "applytrail <command> -h" for a command's flags.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "applytrail: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("a command is required")
	}

	// Signals cancel long operations, so a migration on a slow database can be
	// interrupted without leaving the shell hanging.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command, rest := args[0], args[1:]
	switch command {
	case "migrate":
		return runMigrate(ctx, rest)
	case "bootstrap":
		return runBootstrap(ctx, rest)
	case "apikey":
		return runAPIKey(ctx, rest)
	case "profile":
		return runProfile(ctx, rest)
	case "version":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

// newTabWriter returns the writer every listing uses, so output lines up.
func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}
