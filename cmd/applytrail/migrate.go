package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/VamsiKrishna1211/applytrail/internal/migrate"
)

func runMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("applytrail migrate", flag.ExitOnError)
	databaseURL := dbFlags(fs)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: applytrail migrate [up|down|status|version] [flags]\n\n"+
			"  up       apply every pending migration\n"+
			"  down     roll back the most recent migration\n"+
			"  status   show which migrations have been applied\n"+
			"  version  print the current schema version\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	action := "up"
	if fs.NArg() > 0 {
		action = fs.Arg(0)
	}

	switch action {
	case "up":
		if err := migrate.Up(ctx, *databaseURL); err != nil {
			return err
		}
		fmt.Println("schema is up to date")
		return nil
	case "down":
		if err := migrate.Down(ctx, *databaseURL); err != nil {
			return err
		}
		fmt.Println("rolled back one migration")
		return nil
	case "status":
		return migrate.Status(ctx, *databaseURL)
	case "version":
		v, err := migrate.Version(ctx, *databaseURL)
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	default:
		fs.Usage()
		return fmt.Errorf("unknown migrate action %q", action)
	}
}
