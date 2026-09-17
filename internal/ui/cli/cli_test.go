package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{name: "days", value: "7d", want: now.Add(-7 * 24 * time.Hour)},
		{name: "hours", value: "12h", want: now.Add(-12 * time.Hour)},
		{name: "RFC3339", value: "2026-09-01T10:30:00Z", want: time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)},
		{name: "zero", value: "0d", wantErr: true},
		{name: "invalid", value: "last-week", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSince(test.value, now)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseSince() = %v, nil; want error", got)
				}
				return
			}
			if err != nil || !got.Equal(test.want) {
				t.Errorf("parseSince() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestRunParsesSyncFlagsAndPrintsReport(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	var gotPath string
	var gotOptions appsync.Options
	build := func(path string) ([]Project, error) {
		gotPath = path
		return []Project{{
			Name: "SRC -> DST",
			Run: func(_ context.Context, options appsync.Options) (appsync.Report, error) {
				gotOptions = options
				action := appsync.Action{Kind: appsync.ActionCreated, SourceID: "source-item", Reference: "SRC-16"}
				options.OnItem(action)
				return appsync.Report{Created: 1, StatusSet: 1, Actions: []appsync.Action{action}}, nil
			},
		}}, nil
	}
	var stdout, stderr bytes.Buffer
	exitCode := runWithoutLock([]string{"sync", "--reconcile", "--dry-run", "--since", "12h", "--config", "custom.jsonc"}, build, &stdout, &stderr, func() time.Time { return now })

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("run() exit=%d stderr=%q", exitCode, stderr.String())
	}
	if gotPath != "custom.jsonc" || gotOptions.Mode != appsync.Reconcile || !gotOptions.DryRun || !gotOptions.Since.Equal(now.Add(-12*time.Hour)) {
		t.Errorf("path/options = %q / %#v", gotPath, gotOptions)
	}
	if output := stdout.String(); output != "SRC-16 -> (new): created\ncreated=1 updated=0 unchanged=0 status-set=1 deleted=0 skipped=0\n" {
		t.Errorf("stdout = %q", output)
	}
}

func TestRunDefaultsToOneHourWindow(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	var got appsync.Options
	build := func(string) ([]Project, error) {
		return []Project{{Name: "project", Run: func(_ context.Context, options appsync.Options) (appsync.Report, error) {
			got = options
			return appsync.Report{}, nil
		}}}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := runWithoutLock([]string{"sync"}, build, &stdout, &stderr, func() time.Time { return now }); code != 0 {
		t.Fatalf("run() exit=%d stderr=%q", code, stderr.String())
	}
	if !got.Since.Equal(now.Add(-time.Hour)) {
		t.Errorf("default since = %v, want %v", got.Since, now.Add(-time.Hour))
	}
}

func TestRunAcceptsIdentifierAndIgnoresSince(t *testing.T) {
	var got appsync.Options
	build := func(string) ([]Project, error) {
		return []Project{{Name: "project", Run: func(_ context.Context, options appsync.Options) (appsync.Report, error) {
			got = options
			return appsync.Report{}, nil
		}}}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := runWithoutLock([]string{"sync", "--since", "not-a-window", "SRC-123"}, build, &stdout, &stderr, time.Now); code != 0 {
		t.Fatalf("run() exit=%d stderr=%q", code, stderr.String())
	}
	if got.Identifier != "SRC-123" || !got.Since.IsZero() {
		t.Errorf("options = %#v", got)
	}
}

func TestRunAcquiresAndReleasesLockForRealRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nested", "planesync.jsonc")
	build := func(string) ([]Project, error) { return nil, nil }
	var gotPath string
	var gotStaleAfter time.Duration
	released := false
	acquire := func(path string, staleAfter time.Duration) (func() error, error) {
		gotPath = path
		gotStaleAfter = staleAfter
		return func() error { released = true; return nil }, nil
	}
	var stdout, stderr bytes.Buffer
	code := runWithLock([]string{"sync", "--config", configPath}, build, &stdout, &stderr, time.Now, acquire)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("runWithLock() exit=%d stderr=%q", code, stderr.String())
	}
	wantPath := filepath.Join(filepath.Dir(configPath), "planesync.lock")
	if gotPath != wantPath || gotStaleAfter != 10*time.Minute || !released {
		t.Errorf("lock = path %q, stale %v, released %t; want %q, 10m, true", gotPath, gotStaleAfter, released, wantPath)
	}
}

func TestRunLockFailureIsRuntimeError(t *testing.T) {
	lockErr := errors.New("another planesync instance appears to be running; lock held since timestamp")
	acquire := func(string, time.Duration) (func() error, error) { return nil, lockErr }
	var stdout, stderr bytes.Buffer
	code := runWithLock([]string{"sync"}, func(string) ([]Project, error) { return nil, nil }, &stdout, &stderr, time.Now, acquire)
	if code != 1 || !strings.Contains(stderr.String(), lockErr.Error()) {
		t.Errorf("runWithLock() exit=%d stderr=%q", code, stderr.String())
	}
}

func TestRunDryRunDoesNotAcquireLock(t *testing.T) {
	acquire := func(string, time.Duration) (func() error, error) {
		t.Fatal("dry-run acquired a lock")
		return nil, nil
	}
	var stdout, stderr bytes.Buffer
	code := runWithLock([]string{"sync", "--dry-run"}, func(string) ([]Project, error) { return nil, nil }, &stdout, &stderr, time.Now, acquire)
	if code != 0 || stderr.Len() != 0 {
		t.Errorf("runWithLock() exit=%d stderr=%q", code, stderr.String())
	}
}

func TestStreamingActionsPrecedeSummary(t *testing.T) {
	report := appsync.Report{
		Updated: 1, Deleted: 1,
		Actions: []appsync.Action{
			{Kind: appsync.ActionUpdated, Reference: "SRC-16", Key: "CORE-4277"},
			{Kind: appsync.ActionDeleted, Reference: "source-id", Key: "CORE-4278"},
		},
	}
	var output bytes.Buffer
	for _, action := range report.Actions {
		printAction(&output, action)
	}
	printReport(&output, report)
	want := "SRC-16 -> CORE-4277: updated\nsource-id -> CORE-4278: deleted\ncreated=0 updated=1 unchanged=0 status-set=0 deleted=1 skipped=0\n"
	if output.String() != want {
		t.Errorf("printReport() = %q, want %q", output.String(), want)
	}
}

func TestRunKeepsStreamedOutputWhenServiceFails(t *testing.T) {
	build := func(string) ([]Project, error) {
		return []Project{{Name: "project", Run: func(_ context.Context, options appsync.Options) (appsync.Report, error) {
			options.OnItem(appsync.Action{Kind: appsync.ActionUpdated, Reference: "SRC-16", Key: "CORE-4277"})
			return appsync.Report{}, errors.New("later failure")
		}}}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := runWithoutLock([]string{"sync"}, build, &stdout, &stderr, time.Now); code != 1 {
		t.Fatalf("run() exit = %d, want 1", code)
	}
	if stdout.String() != "SRC-16 -> CORE-4277: updated\n" || !strings.Contains(stderr.String(), "later failure") {
		t.Errorf("stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		build     Builder
		wantCode  int
		wantError string
	}{
		{name: "unknown command", args: []string{"other"}, wantCode: 2, wantError: "unknown command"},
		{name: "invalid since", args: []string{"sync", "--since", "bad"}, wantCode: 2, wantError: "invalid --since"},
		{name: "conflicting modes", args: []string{"sync", "--full", "--reconcile"}, wantCode: 2, wantError: "mutually exclusive"},
		{name: "identifier with full", args: []string{"sync", "--full", "SRC-123"}, wantCode: 2, wantError: "mutually exclusive"},
		{name: "identifier with reconcile", args: []string{"sync", "--reconcile", "SRC-123"}, wantCode: 2, wantError: "mutually exclusive"},
		{name: "too many identifiers", args: []string{"sync", "SRC-123", "SRC-124"}, wantCode: 2, wantError: "at most one"},
		{
			name: "builder failure", args: []string{"sync"}, wantCode: 1, wantError: "configuration error",
			build: func(string) ([]Project, error) { return nil, errors.New("unavailable") },
		},
		{
			name: "service failure", args: []string{"sync"}, wantCode: 1, wantError: "sync failed",
			build: func(string) ([]Project, error) {
				return []Project{{Name: "project", Run: func(context.Context, appsync.Options) (appsync.Report, error) {
					return appsync.Report{}, errors.New("failed")
				}}}, nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			build := test.build
			if build == nil {
				build = func(string) ([]Project, error) { return nil, nil }
			}
			var stdout, stderr bytes.Buffer
			got := runWithoutLock(test.args, build, &stdout, &stderr, time.Now)
			if got != test.wantCode || !strings.Contains(stderr.String(), test.wantError) {
				t.Errorf("run() exit=%d stderr=%q, want %d containing %q", got, stderr.String(), test.wantCode, test.wantError)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"sync", "--help"}} {
		var stdout, stderr bytes.Buffer
		if got := runWithoutLock(args, func(string) ([]Project, error) { return nil, nil }, &stdout, &stderr, time.Now); got != 0 {
			t.Errorf("run(%v) exit = %d", args, got)
		}
		if stdout.Len() == 0 && stderr.Len() == 0 {
			t.Errorf("run(%v) printed no usage", args)
		}
	}
}

func runWithoutLock(args []string, build Builder, stdout, stderr *bytes.Buffer, now func() time.Time) int {
	return runWithLock(args, build, stdout, stderr, now, func(string, time.Duration) (func() error, error) {
		return func() error { return nil }, nil
	})
}
