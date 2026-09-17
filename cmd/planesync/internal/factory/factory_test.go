package factory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
	"github.com/hrubymar10/planesync/internal/domain/configuration"
	"github.com/hrubymar10/planesync/internal/infrastructure/jira"
	"github.com/hrubymar10/planesync/internal/infrastructure/plane"
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
	adapter := targetAdapter{client: client, parentEpicKey: "epic-parent", components: []string{"Backend"}, priority: "High"}
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
	if client.created.Priority != "High" || client.updated.Priority != "High" {
		t.Errorf("create/update priorities = %q/%q", client.created.Priority, client.updated.Priority)
	}
}

func TestRenderBodyUsesPlainIdentifierFooter(t *testing.T) {
	for _, format := range []string{"rich", "text"} {
		t.Run(format, func(t *testing.T) {
			encoded := renderBody(format, "<p>Body</p>", "SRC-16")
			if !json.Valid(encoded) {
				t.Fatalf("renderBody() returned invalid JSON: %s", encoded)
			}
			body := string(encoded)
			if !strings.Contains(body, `"text":"Mirrored from Plane: SRC-16"`) {
				t.Errorf("rendered body is missing plain identifier footer: %s", body)
			}
			if strings.Contains(body, `"marks"`) || strings.Contains(body, "https://") {
				t.Errorf("rendered footer contains link data: %s", body)
			}
		})
	}
}

func TestSourceItemsForwardsIdentifier(t *testing.T) {
	items := sourceItems([]plane.Item{{ID: "source-id", Identifier: "SRC-16"}})
	if len(items) != 1 || items[0].ID != "source-id" || items[0].Identifier != "SRC-16" {
		t.Errorf("sourceItems() = %#v", items)
	}
}

func TestContentSaltIsOrderIndependentAndCoversWriteConfiguration(t *testing.T) {
	defaults := configuration.Defaults{BodyFormat: "rich", DeletedStatus: "Removed", DeletedResolution: "Declined"}
	project := configuration.Project{
		TitlePrefix: "[team]", StatusMap: map[string]string{"Started": "In Progress", "Done": "Done"},
		StatusGroupMap: map[string]string{"cancelled": "Removed"}, ResolutionMap: map[string]string{"Done": "Fixed"},
		Priority: "High", Components: []string{"API", "Backend"}, EpicKey: "EPIC-1",
	}
	want := contentSalt(defaults, project, "account-id")
	reordered := project
	reordered.StatusMap = map[string]string{"Done": "Done", "Started": "In Progress"}
	reordered.Components = []string{"Backend", "API"}
	if got := contentSalt(defaults, reordered, "account-id"); got != want {
		t.Errorf("reordered salt = %q, want %q", got, want)
	}

	changed := project
	changed.Priority = "Low"
	if got := contentSalt(defaults, changed, "account-id"); got == want {
		t.Error("priority change did not change content salt")
	}
	if got := contentSalt(defaults, project, "other-account"); got == want {
		t.Error("assignee change did not change content salt")
	}
}

type fakeJiraTarget struct {
	created jira.CreateInput
	updated jira.UpdateInput
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
