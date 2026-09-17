package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type record struct {
	Timestamp string `json:"timestamp"`
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
}

// Acquire claims path or reclaims a lock whose timestamp is at least staleAfter old.
func Acquire(path string, staleAfter time.Duration) (release func() error, err error) {
	return acquire(path, staleAfter, time.Now)
}

func acquire(path string, staleAfter time.Duration, now func() time.Time) (func() error, error) {
	if path == "" {
		return nil, fmt.Errorf("lock path is required")
	}
	if staleAfter <= 0 {
		return nil, fmt.Errorf("lock stale duration must be positive")
	}

	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return writeLock(file, path, now())
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create lock %q: %w", path, err)
		}

		existing, err := readRecord(path)
		if err != nil {
			return nil, err
		}
		createdAt, err := time.Parse(time.RFC3339Nano, existing.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse lock %q timestamp %q: %w", path, existing.Timestamp, err)
		}
		if now().Sub(createdAt) < staleAfter {
			return nil, fmt.Errorf("another planesync instance appears to be running; lock at %s held since %s — delete it if stale", path, existing.Timestamp)
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("remove stale lock %q: %w", path, err)
		}
	}
}

func writeLock(file *os.File, path string, timestamp time.Time) (func() error, error) {
	hostname, err := os.Hostname()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("resolve hostname for lock %q: %w", path, err)
	}
	record := record{
		Timestamp: timestamp.UTC().Format(time.RFC3339Nano),
		PID:       os.Getpid(),
		Hostname:  hostname,
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write lock %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync lock %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("stat lock %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close lock %q: %w", path, err)
	}

	return func() error {
		current, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("stat lock for release %q: %w", path, err)
		}
		if !os.SameFile(info, current) {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("release lock %q: %w", path, err)
		}
		return nil
	}, nil
}

func readRecord(path string) (record, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return record{}, fmt.Errorf("read lock %q: %w", path, err)
	}
	var decoded record
	if err := json.Unmarshal(contents, &decoded); err != nil {
		return record{}, fmt.Errorf("decode lock %q: %w", path, err)
	}
	return decoded, nil
}
