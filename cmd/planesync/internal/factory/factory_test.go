package factory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExampleConfiguration(t *testing.T) {
	t.Setenv("JIRA_TOKEN", "jira-token")
	t.Setenv("PLANE_TOKEN", "plane-token")
	examplePath := filepath.Join("..", "..", "..", "..", "config", "planesync.example.jsonc")
	contents, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	contents = []byte(strings.Replace(string(contents), `"assign_all_to_me": true`, `"assign_all_to_me": false`, 1))
	path := filepath.Join(t.TempDir(), "planesync.jsonc")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	projects, err := Build(path)
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "SRC -> DST" || projects[0].Run == nil {
		t.Errorf("projects = %#v", projects)
	}
}
