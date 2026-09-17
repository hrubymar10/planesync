// Package cli provides the non-interactive command-line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
	"github.com/hrubymar10/planesync/internal/infrastructure/lockfile"
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
	return runWithLock(args, build, stdout, stderr, now, lockfile.Acquire)
}

type acquireLockFunc func(string, time.Duration) (func() error, error)

func runWithLock(args []string, build Builder, stdout, stderr io.Writer, now func() time.Time, acquire acquireLockFunc) int {
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
	force := flags.Bool("force", false, "update mapped items even when synchronization markers match")
	sinceValue := flags.String("since", "1h", "incremental window (Nd, Nh, or RFC3339)")
	configPath := flags.String("config", defaultConfigPath, "configuration file path")
	limit := flags.Int("limit", 0, "maximum source items to process (0 is unlimited)")
	flags.Usage = func() { printSyncUsage(flags.Output()) }
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "sync accepts at most one Plane identifier")
		return 2
	}
	identifier := ""
	if flags.NArg() == 1 {
		identifier = flags.Arg(0)
	}
	if *full && *reconcile {
		fmt.Fprintf(stderr, "--full and --reconcile are mutually exclusive\n")
		return 2
	}
	if identifier != "" && (*full || *reconcile) {
		fmt.Fprintln(stderr, "a Plane identifier is mutually exclusive with --full and --reconcile")
		return 2
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "--limit must not be negative")
		return 2
	}

	var since time.Time
	if identifier == "" {
		var err error
		since, err = parseSince(*sinceValue, now())
		if err != nil {
			fmt.Fprintf(stderr, "invalid --since: %v\n", err)
			return 2
		}
	}
	mode := appsync.Incremental
	if *full {
		mode = appsync.Full
	} else if *reconcile {
		mode = appsync.Reconcile
	}
	options := appsync.Options{Mode: mode, Since: since, Identifier: identifier, DryRun: *dryRun, Force: *force, Limit: *limit}
	options.OnItem = func(action appsync.Action) { printAction(stdout, action) }

	projects, err := build(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return 1
	}
	if !*dryRun {
		lockPath := filepath.Join(filepath.Dir(*configPath), "planesync.lock")
		release, err := acquire(lockPath, 10*time.Minute)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer func() { _ = release() }()
	}
	for _, project := range projects {
		report, err := project.Run(context.Background(), options)
		if err != nil {
			fmt.Fprintf(stderr, "%s: sync failed: %v\n", project.Name, err)
			return 1
		}
		printReport(stdout, report)
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

func printReport(output io.Writer, report appsync.Report) {
	fmt.Fprintf(output, "created=%d updated=%d unchanged=%d status-set=%d deleted=%d skipped=%d\n",
		report.Created, report.Updated, report.Unchanged, report.StatusSet, report.Deleted, report.Skipped)
}

func printAction(output io.Writer, action appsync.Action) {
	key := action.Key
	if key == "" {
		key = "(new)"
	}
	fmt.Fprintf(output, "%s -> %s: %s\n", action.Reference, key, action.Kind)
}

func printRootUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: planesync sync [flags]")
	fmt.Fprintln(output, "Run 'planesync sync --help' for sync flags.")
}

func printSyncUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: planesync sync [--full|--reconcile] [--dry-run] [--force] [--since Nd|Nh|RFC3339] [--config path] [identifier]")
}
