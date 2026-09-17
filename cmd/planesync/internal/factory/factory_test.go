package factory

import (
	"path/filepath"
	"testing"
)

func TestBuildExampleConfiguration(t *testing.T) {
	t.Setenv("JIRA_TOKEN", "jira-token")
	t.Setenv("PLANE_TOKEN", "plane-token")
	path := filepath.Join("..", "..", "..", "..", "config", "planesync.example.jsonc")

	projects, err := Build(path)
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "SRC -> DST" || projects[0].Run == nil {
		t.Errorf("projects = %#v", projects)
	}
}
