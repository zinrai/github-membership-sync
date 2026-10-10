package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const (
	cmdDiff  = "diff"
	cmdApply = "apply"
	cmdDump  = "dump"
)

const usage = `Usage: github-membership-sync <command> [flags]

Commands:
  diff   print what apply would change
  apply  make GitHub match the file
  dump   print the current membership as YAML

Flags:
  -version  print version information and exit

Run github-membership-sync <command> -h for the flags of each.
`

// errUsage stands for a missing or unknown command, already printed with the
// usage. Reporting it again through slog would print it twice.
var errUsage = errors.New("usage")

type config struct {
	command     string
	org         string
	configPath  string
	tokenFile   string
	apiBase     string
	maxRemovals int
	showVersion bool
}

// parseArgs reads the configuration from the command line and nowhere else,
// so what a run was asked to do is visible in the process listing rather than
// in whatever happened to be exported around it.
func parseArgs(args []string) (*config, error) {
	var showVersion bool
	top := flag.NewFlagSet("github-membership-sync", flag.ExitOnError)
	top.BoolVar(&showVersion, "version", false, "print version information and exit")
	top.Usage = func() { fmt.Fprint(top.Output(), usage) }
	if err := top.Parse(args); err != nil {
		return nil, err
	}
	if err := refuseRepeats(top, args); err != nil {
		return nil, err
	}
	if showVersion {
		return &config{showVersion: true}, nil
	}
	if top.NArg() == 0 {
		top.Usage()
		return nil, errUsage
	}
	return parseCommand(top, top.Arg(0), top.Args()[1:])
}

func parseCommand(top *flag.FlagSet, command string, args []string) (*config, error) {
	cfg := config{command: command}
	fs := flag.NewFlagSet("github-membership-sync "+command, flag.ExitOnError)
	fs.StringVar(&cfg.org, "org", "", "organization to manage")
	fs.StringVar(&cfg.tokenFile, "token-file", "", "file holding the token, read before every request")
	fs.StringVar(&cfg.apiBase, "api-base", defaultAPIBase, "REST API root, used as written")
	switch command {
	case cmdDiff, cmdApply:
		fs.StringVar(&cfg.configPath, "config", "", "YAML file with the membership to make GitHub match")
		fs.IntVar(&cfg.maxRemovals, "max-removals", 3, "refuse a plan removing more organization members than this")
	case cmdDump:
	default:
		fmt.Fprintf(top.Output(), "unknown command %q\n", command)
		top.Usage()
		return nil, errUsage
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if err := refuseRepeats(fs, args); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// refuseRepeats parses args again only to count each flag, as flag lets a
// repeated one silently replace the first. Wrapping each flag.Value instead
// would lose the type names and defaults that -h prints.
func refuseRepeats(fs *flag.FlagSet, args []string) error {
	count := flag.NewFlagSet(fs.Name(), flag.ContinueOnError)
	count.SetOutput(io.Discard)
	seen := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			count.BoolFunc(f.Name, "", onlyOnce(seen, f.Name))
		} else {
			count.Func(f.Name, "", onlyOnce(seen, f.Name))
		}
	})
	return count.Parse(args)
}

func onlyOnce(seen map[string]bool, name string) func(string) error {
	return func(string) error {
		if seen[name] {
			return errors.New("given more than once")
		}
		seen[name] = true
		return nil
	}
}

func (c *config) validate() error {
	switch {
	case c.org == "":
		return errors.New("-org is required")
	case c.tokenFile == "":
		return errors.New("-token-file is required")
	case c.command != cmdDump && c.configPath == "":
		return errors.New("-config is required")
	}
	return nil
}
