// Package cli provides the non-interactive command-line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
)

const defaultConfigPath = "config/planesync.jsonc"

// Project is a configured project synchronization runner.
type Project struct {
	Name string
	Run  func(context.Context, appsync.Options) (appsync.Report, error)
}

// Builder loads configuration and builds project runners.
type Builder func(configPath string) ([]Project, error)

// Run executes the command with args and returns its exit code.
func Run(args []string, build Builder) int {
	return run(args, build, os.Stdout, os.Stderr, time.Now)
}

func run(args []string, build Builder, stdout, stderr io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		printRootUsage(stdout)
		return 0
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printRootUsage(stdout)
		return 0
	}
	if args[0] != "sync" {
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printRootUsage(stderr)
		return 2
	}

	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.SetOutput(stderr)
	full := flags.Bool("full", false, "sync the full source set")
	reconcile := flags.Bool("reconcile", false, "sync the full set and reconcile missing items")
	dryRun := flags.Bool("dry-run", false, "show intended writes without applying them")
	sinceValue := flags.String("since", "7d", "incremental window (Nd, Nh, or RFC3339)")
	configPath := flags.String("config", defaultConfigPath, "configuration file path")
	limit := flags.Int("limit", 0, "maximum source items to process (0 is unlimited)")
	flags.Usage = func() { printSyncUsage(flags.Output()) }
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "sync does not accept positional arguments\n")
		return 2
	}
	if *full && *reconcile {
		fmt.Fprintf(stderr, "--full and --reconcile are mutually exclusive\n")
		return 2
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "--limit must not be negative")
		return 2
	}

	since, err := parseSince(*sinceValue, now())
	if err != nil {
		fmt.Fprintf(stderr, "invalid --since: %v\n", err)
		return 2
	}
	mode := appsync.Incremental
	if *full {
		mode = appsync.Full
	} else if *reconcile {
		mode = appsync.Reconcile
	}
	options := appsync.Options{Mode: mode, Since: since, DryRun: *dryRun, Limit: *limit}

	projects, err := build(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return 1
	}
	for _, project := range projects {
		report, err := project.Run(context.Background(), options)
		if err != nil {
			fmt.Fprintf(stderr, "%s: sync failed: %v\n", project.Name, err)
			return 1
		}
		printReport(stdout, project.Name, report, options.DryRun)
	}
	return 0
}

func parseSince(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if len(value) < 2 {
		return time.Time{}, fmt.Errorf("expected Nd, Nh, or RFC3339")
	}
	amount, err := strconv.Atoi(value[:len(value)-1])
	if err != nil || amount <= 0 {
		return time.Time{}, fmt.Errorf("expected a positive Nd or Nh duration, or RFC3339")
	}
	var duration time.Duration
	switch value[len(value)-1] {
	case 'd':
		duration = time.Duration(amount) * 24 * time.Hour
	case 'h':
		duration = time.Duration(amount) * time.Hour
	default:
		return time.Time{}, fmt.Errorf("expected Nd, Nh, or RFC3339")
	}
	return now.Add(-duration), nil
}

func printReport(output io.Writer, name string, report appsync.Report, dryRun bool) {
	mode := "sync"
	if dryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(output, "%s (%s): created=%d updated=%d status-set=%d deleted=%d skipped=%d\n",
		name, mode, report.Created, report.Updated, report.StatusSet, report.Deleted, report.Skipped)
	for _, action := range report.Actions {
		fmt.Fprintf(output, "  %s source=%s", action.Kind, action.SourceID)
		if action.Key != "" {
			fmt.Fprintf(output, " target=%s", action.Key)
		}
		if action.Detail != "" {
			fmt.Fprintf(output, " detail=%q", action.Detail)
		}
		fmt.Fprintln(output)
	}
}

func printRootUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: planesync sync [flags]")
	fmt.Fprintln(output, "Run 'planesync sync --help' for sync flags.")
}

func printSyncUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: planesync sync [--full|--reconcile] [--dry-run] [--since Nd|Nh|RFC3339] [--config path]")
}
