package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireCreatesLockAndReleaseRemovesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planesync.lock")
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	release, err := acquire(path, 10*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatalf("acquire(): %v", err)
	}
	lock, err := readRecord(path)
	if err != nil {
		t.Fatalf("readRecord(): %v", err)
	}
	if lock.Timestamp != "2026-09-17T12:00:00Z" || lock.PID != os.Getpid() || lock.Hostname == "" {
		t.Errorf("lock record = %#v", lock)
	}
	if err := release(); err != nil {
		t.Fatalf("release(): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock after release: %v", err)
	}
}

func TestAcquireFreshLockReturnsClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planesync.lock")
	createdAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	release, err := acquire(path, 10*time.Minute, func() time.Time { return createdAt })
	if err != nil {
		t.Fatalf("first acquire(): %v", err)
	}
	defer func() { _ = release() }()

	_, err = acquire(path, 10*time.Minute, func() time.Time { return createdAt.Add(9 * time.Minute) })
	if err == nil || !strings.Contains(err.Error(), "another planesync instance appears to be running") || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "2026-09-17T12:00:00Z") {
		t.Fatalf("second acquire() error = %v", err)
	}
}

func TestAcquireReclaimsStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "planesync.lock")
	createdAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	oldRelease, err := acquire(path, 10*time.Minute, func() time.Time { return createdAt })
	if err != nil {
		t.Fatalf("first acquire(): %v", err)
	}

	reclaimedAt := createdAt.Add(10 * time.Minute)
	release, err := acquire(path, 10*time.Minute, func() time.Time { return reclaimedAt })
	if err != nil {
		t.Fatalf("stale acquire(): %v", err)
	}
	defer func() { _ = release() }()
	lock, err := readRecord(path)
	if err != nil {
		t.Fatalf("readRecord(): %v", err)
	}
	if lock.Timestamp != "2026-09-17T12:10:00Z" {
		t.Errorf("reclaimed timestamp = %q", lock.Timestamp)
	}
	if err := oldRelease(); err != nil {
		t.Fatalf("old release(): %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("old release removed reclaimed lock: %v", err)
	}
}
