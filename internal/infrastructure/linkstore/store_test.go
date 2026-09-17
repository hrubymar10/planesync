package linkstore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.json")
	store := New(path)
	want := Map{
		"source-one": {Key: "target-one", UpdatedAt: "2026-09-17T10:00:00Z", ConfigSalt: "salt-one"},
		"source-two": {Key: "target-two", UpdatedAt: "2026-09-17T11:00:00Z", ConfigSalt: "salt-two"},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %#v, want %#v", got, want)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if !strings.Contains(string(contents), `"version": 1`) || !strings.Contains(string(contents), `"links": {`) {
		t.Errorf("persisted document does not use versioned envelope: %s", contents)
	}
	if !strings.Contains(string(contents), `"key": "target-one"`) || !strings.Contains(string(contents), `"updated_at": "2026-09-17T10:00:00Z"`) || !strings.Contains(string(contents), `"config_salt": "salt-one"`) {
		t.Errorf("persisted document does not use entry objects: %s", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat persisted file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file permissions = %o, want 600", got)
	}
}

func TestLoadMissingFileReturnsEmptyMap(t *testing.T) {
	links, err := New(filepath.Join(t.TempDir(), "missing.json")).Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("Load() = %#v, want non-nil empty map", links)
	}
}

func TestLoadEmptyFileReturnsEmptyMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(" \n\t"), 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	links, err := New(path).Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("Load() = %#v, want non-nil empty map", links)
	}
}

func TestLoadMalformedFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(path, []byte(`{"version":`), 0o600); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}
	_, err := New(path).Load()
	if err == nil || !strings.Contains(err.Error(), "decode link store") {
		t.Fatalf("Load() error = %v, want decode error", err)
	}
}

func TestSaveAtomicallyOverwritesExistingFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "links.json")
	store := New(path)
	if err := store.Save(Map{"old-source": {Key: "old-target"}}); err != nil {
		t.Fatalf("first Save(): %v", err)
	}
	want := Map{"new-source": {Key: "new-target", UpdatedAt: "2026-09-17T10:00:00Z", ConfigSalt: "new-salt"}}
	if err := store.Save(want); err != nil {
		t.Fatalf("second Save(): %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %#v, want %#v", got, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "links.json" {
		t.Errorf("directory entries after overwrite = %#v, want only target file", entries)
	}
}

func TestSaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state", "links.json")
	store := New(path)
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	links, err := store.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("Load() = %#v, want non-nil empty map", links)
	}
}

func TestLoadRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"links":{}}`), 0o600); err != nil {
		t.Fatalf("write future file: %v", err)
	}
	_, err := New(path).Load()
	if err == nil || !strings.Contains(err.Error(), "unsupported version 2") {
		t.Fatalf("Load() error = %v, want version error", err)
	}
}

func TestLoadLegacyStringEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"links":{"source-id":"target-key"}}`), 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	links, err := New(path).Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := Map{"source-id": {Key: "target-key"}}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("Load() = %#v, want %#v", links, want)
	}
}
