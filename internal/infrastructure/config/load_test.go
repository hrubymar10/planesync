package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExample(t *testing.T) {
	t.Setenv(jiraTokenEnvironment, "jira-from-environment")
	t.Setenv(planeTokenEnvironment, "plane-from-environment")

	path := filepath.Join("..", "..", "..", "config", "planesync.example.jsonc")
	result, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(result.Projects))
	}
	if got := result.Projects[0].JiraToken.Reveal(); got != "jira-from-environment" {
		t.Errorf("Jira token = %q, want environment value", got)
	}
	if got := result.Projects[0].PlaneToken.Reveal(); got != "plane-from-environment" {
		t.Errorf("Plane token = %q, want environment value", got)
	}
	if got := result.Jira.AuthType; got != "basic" {
		t.Errorf("Jira auth type = %q, want basic", got)
	}
	if got := result.Plane.AppBaseURL; got != "https://app.plane.so" {
		t.Errorf("Plane app base URL = %q", got)
	}
	if got := result.Projects[0].StatusGroupMap["started"]; got != "In Progress" {
		t.Errorf("status group mapping = %q", got)
	}
	if got := result.Defaults.DeletedStatus; got != "Done" {
		t.Errorf("deleted status = %q", got)
	}
	if got := result.Defaults.DeletedResolution; got != "Declined" {
		t.Errorf("deleted resolution = %q", got)
	}
	if got := result.Projects[0].ResolutionMap["Cancelled"]; got != "Declined" {
		t.Errorf("resolution mapping = %q", got)
	}
	if got := result.Projects[0].EpicKey; got != "DST-100" {
		t.Errorf("epic key = %q", got)
	}
}

func TestLoadAcceptsJSONCCommentsAndTrailingCommas(t *testing.T) {
	path := writeConfiguration(t, `{
		// A URL containing comment-like characters must remain intact.
		"jira": {"base_url": "https://jira.example.com/path//segment", "cloud_id": "example-cloud", "email": "you@example.com", "token": "jira-token",},
		/* Block comments are accepted. */
		"plane": {"base_url": "https://api.plane.so", "workspace": "example-workspace", "token": "plane-token",},
		"defaults": {"since": "7d", "body_format": "rich",},
		"projects": [{
			"plane_project": "SRC",
			"jira_project": "DST",
			"jira_issue_type": "Task",
			"status_map": {"Started": "In Progress",},
		},],
	}`)

	result, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got := result.Jira.BaseURL; got != "https://jira.example.com/path//segment" {
		t.Errorf("Jira base URL = %q", got)
	}
}

func TestLoadInterpolatesEnvironmentInAnyString(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://jira.example.com")
	t.Setenv("TITLE_PREFIX", "[team]")
	t.Setenv(jiraTokenEnvironment, "jira-from-environment")
	t.Setenv(planeTokenEnvironment, "plane-from-environment")
	path := writeConfiguration(t, validConfiguration(`"${JIRA_TOKEN}"`, `"${PLANE_TOKEN}"`, "", "", `"${JIRA_BASE_URL}"`, `"${TITLE_PREFIX}"`))

	result, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got := result.Jira.BaseURL; got != "https://jira.example.com" {
		t.Errorf("Jira base URL = %q", got)
	}
	if got := result.Projects[0].TitlePrefix; got != "[team]" {
		t.Errorf("title prefix = %q", got)
	}
	if got := result.Jira.AuthType; got != "basic" {
		t.Errorf("Jira auth type = %q, want default basic", got)
	}
}

func TestTokenPrecedence(t *testing.T) {
	t.Setenv(jiraTokenEnvironment, "jira-from-environment")
	t.Setenv(planeTokenEnvironment, "plane-from-environment")

	tests := []struct {
		name          string
		jiraDefault   string
		planeDefault  string
		jiraOverride  string
		planeOverride string
		wantJira      string
		wantPlane     string
	}{
		{
			name:          "project override beats default",
			jiraDefault:   `"jira-default"`,
			planeDefault:  `"plane-default"`,
			jiraOverride:  `"jira-override"`,
			planeOverride: `"plane-override"`,
			wantJira:      "jira-override",
			wantPlane:     "plane-override",
		},
		{
			name:         "default beats environment",
			jiraDefault:  `"jira-default"`,
			planeDefault: `"plane-default"`,
			wantJira:     "jira-default",
			wantPlane:    "plane-default",
		},
		{
			name:      "environment is fallback",
			wantJira:  "jira-from-environment",
			wantPlane: "plane-from-environment",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeConfiguration(t, validConfiguration(test.jiraDefault, test.planeDefault, test.jiraOverride, test.planeOverride, `"https://jira.example.com"`, `"[team]"`))
			result, err := Load(path)
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if got := result.Projects[0].JiraToken.Reveal(); got != test.wantJira {
				t.Errorf("Jira token = %q, want %q", got, test.wantJira)
			}
			if got := result.Projects[0].PlaneToken.Reveal(); got != test.wantPlane {
				t.Errorf("Plane token = %q, want %q", got, test.wantPlane)
			}
		})
	}
}

func TestLoadRejectsMissingToken(t *testing.T) {
	t.Setenv(jiraTokenEnvironment, "")
	t.Setenv(planeTokenEnvironment, "")
	path := writeConfiguration(t, validConfiguration("", `"plane-token"`, "", "", `"https://jira.example.com"`, `"[team]"`))

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() returned nil error")
	}
	if got := err.Error(); !strings.Contains(got, "projects[0].jira_token is required") || !strings.Contains(got, jiraTokenEnvironment) {
		t.Errorf("Load() error = %q, want project field and environment hint", got)
	}
}

func writeConfiguration(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "planesync.jsonc")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
	return path
}

func validConfiguration(jiraDefault, planeDefault, jiraOverride, planeOverride, jiraBaseURL, titlePrefix string) string {
	optional := func(name, value string) string {
		if value == "" {
			return ""
		}
		return `,"` + name + `":` + value
	}
	return `{
		"jira":{"base_url":` + jiraBaseURL + `,"cloud_id":"example-cloud","email":"you@example.com"` + optional("token", jiraDefault) + `},
		"plane":{"base_url":"https://api.plane.so","workspace":"example-workspace"` + optional("token", planeDefault) + `},
		"defaults":{"since":"7d","body_format":"rich"},
		"projects":[{
			"plane_project":"SRC",
			"jira_project":"DST",
			"jira_issue_type":"Task",
			"title_prefix":` + titlePrefix + optional("jira_token", jiraOverride) + optional("plane_token", planeOverride) + `,
			"status_map":{"Started":"In Progress"}
		}]
	}`
}
