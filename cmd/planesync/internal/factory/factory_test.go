package factory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
	"github.com/hrubymar10/planesync/internal/infrastructure/jira"
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

func TestTargetAdapterForwardsTargetFields(t *testing.T) {
	client := &fakeJiraTarget{}
	adapter := targetAdapter{client: client, parentEpicKey: "epic-parent", components: []string{"Backend"}}
	if _, err := adapter.Create(context.Background(), appsync.CreateSpec{}); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if err := adapter.Update(context.Background(), "target-key", appsync.UpdateSpec{}); err != nil {
		t.Fatalf("Update(): %v", err)
	}
	if client.created.ParentEpicKey != "epic-parent" || client.updated.ParentEpicKey != "epic-parent" {
		t.Errorf("create/update epic keys = %q/%q", client.created.ParentEpicKey, client.updated.ParentEpicKey)
	}
	if len(client.created.Components) != 1 || client.created.Components[0] != "Backend" || len(client.updated.Components) != 1 || client.updated.Components[0] != "Backend" {
		t.Errorf("create/update components = %#v/%#v", client.created.Components, client.updated.Components)
	}
}

type fakeJiraTarget struct {
	created jira.CreateInput
	updated jira.UpdateInput
}

func (*fakeJiraTarget) FindByLabel(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeJiraTarget) Create(_ context.Context, input jira.CreateInput) (string, error) {
	f.created = input
	return "target-key", nil
}
func (f *fakeJiraTarget) Update(_ context.Context, _ string, input jira.UpdateInput) error {
	f.updated = input
	return nil
}
func (*fakeJiraTarget) SetStatus(context.Context, string, string, string) error { return nil }
