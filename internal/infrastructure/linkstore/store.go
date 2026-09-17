package linkstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const currentVersion = 1

// Entry records the Jira key and synchronization markers for one Plane item.
type Entry struct {
	Key        string `json:"key"`
	UpdatedAt  string `json:"updated_at"`
	ConfigSalt string `json:"config_salt"`
}

// UnmarshalJSON accepts both the current object form and the legacy key string.
func (e *Entry) UnmarshalJSON(contents []byte) error {
	var legacyKey string
	if err := json.Unmarshal(contents, &legacyKey); err == nil {
		*e = Entry{Key: legacyKey}
		return nil
	}
	type entry Entry
	var decoded entry
	if err := json.Unmarshal(contents, &decoded); err != nil {
		return err
	}
	*e = Entry(decoded)
	return nil
}

// Map associates Plane work item IDs with Jira link entries.
type Map map[string]Entry

// Store persists a link map at a configured path.
type Store struct {
	path string
}

// New creates a link store at path.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the persisted link map. A missing or empty file yields an empty map.
func (s *Store) Load() (Map, error) {
	contents, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Map{}, nil
		}
		return nil, fmt.Errorf("read link store: %w", err)
	}
	if len(bytes.TrimSpace(contents)) == 0 {
		return Map{}, nil
	}

	var document struct {
		Version int `json:"version"`
		Links   Map `json:"links"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, fmt.Errorf("decode link store: %w", err)
	}
	if document.Version != currentVersion {
		return nil, fmt.Errorf("decode link store: unsupported version %d", document.Version)
	}
	if document.Links == nil {
		document.Links = Map{}
	}
	return document.Links, nil
}

// Save atomically replaces the persisted link map.
func (s *Store) Save(links Map) error {
	if s.path == "" {
		return fmt.Errorf("link store path is required")
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create link store directory: %w", err)
	}
	if links == nil {
		links = Map{}
	}
	document := struct {
		Version int `json:"version"`
		Links   Map `json:"links"`
	}{Version: currentVersion, Links: links}
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode link store: %w", err)
	}
	contents = append(contents, '\n')

	temporary, err := os.CreateTemp(directory, ".planesync-links-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary link store: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporaryPath != "" {
			_ = os.Remove(temporaryPath)
		}
	}()

	fail := func(operation string, operationErr error) error {
		_ = temporary.Close()
		return fmt.Errorf("%s link store: %w", operation, operationErr)
	}
	if err := temporary.Chmod(0o600); err != nil {
		return fail("secure temporary", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fail("write temporary", err)
	}
	if err := temporary.Sync(); err != nil {
		return fail("sync temporary", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary link store: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace link store: %w", err)
	}
	temporaryPath = ""
	return nil
}
