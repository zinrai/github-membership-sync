// Command github-membership-sync makes the members of a GitHub organization,
// and of the teams named in a YAML file, match that file.
//
// diff prints what apply would change, apply changes it, and dump prints the
// current membership in the same YAML form, to start the file from.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	err := run()
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		return err
	}
	if cfg.showVersion {
		printVersion()
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runCommand(ctx, newClient(cfg.apiBase, cfg.tokenFile), cfg, os.Stdout)
}

func runCommand(ctx context.Context, gh *client, cfg *config, stdout io.Writer) error {
	switch cfg.command {
	case cmdDump:
		return dump(ctx, gh, cfg.org, stdout)
	case cmdDiff:
		_, err := planChanges(ctx, gh, cfg, stdout)
		return err
	case cmdApply:
		changes, err := planChanges(ctx, gh, cfg, stdout)
		if err != nil {
			return err
		}
		return applyChanges(ctx, gh, cfg.org, changes)
	}
	return fmt.Errorf("unknown command %q", cfg.command)
}

func dump(ctx context.Context, gh *client, org string, stdout io.Writer) error {
	slugs, err := gh.teamSlugs(ctx, org)
	if err != nil {
		return err
	}
	have, err := gh.fetch(ctx, org, slugs)
	if err != nil {
		return err
	}
	logRead(org, have)
	out, err := encodeMembership(have)
	if err != nil {
		return err
	}
	_, err = stdout.Write(out)
	return err
}

func planChanges(ctx context.Context, gh *client, cfg *config, stdout io.Writer) ([]change, error) {
	want, err := loadMembership(cfg.membershipFile)
	if err != nil {
		return nil, err
	}
	have, err := gh.fetch(ctx, cfg.org, want.teamSlugs())
	if err != nil {
		return nil, err
	}

	logPlan(cfg.org, have, want)
	changes := plan(want, have)
	for _, c := range changes {
		fmt.Fprintln(stdout, c)
	}

	// Checked after printing, so that a refused plan can still be reviewed.
	if n := countOrgRemovals(changes); n > cfg.maxRemovals {
		return nil, fmt.Errorf("plan removes %d members from %s, more than -max-removals %d", n, cfg.org, cfg.maxRemovals)
	}
	return changes, nil
}

func applyChanges(ctx context.Context, gh *client, org string, changes []change) error {
	for _, c := range changes {
		if err := gh.apply(ctx, org, c); err != nil {
			return fmt.Errorf("%s: %w", c, err)
		}
	}
	slog.Info("applied", "org", org, "changes", len(changes))
	return nil
}
