package configuration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecretRedactsEveryImplicitRepresentation(t *testing.T) {
	const raw = "sensitive-value"
	secret := Secret(raw)

	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("marshal secret: %v", err)
	}

	outputs := []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%s", secret),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%#v", secret),
		string(encoded),
	}
	for _, output := range outputs {
		if strings.Contains(output, raw) {
			t.Errorf("secret leaked through representation: %q", output)
		}
		if output != `"***"` && output != "***" {
			t.Errorf("representation = %q, want redaction", output)
		}
	}

	if got := secret.Reveal(); got != raw {
		t.Errorf("Reveal() = %q, want original value", got)
	}
}

func TestConfigJSONRedactsSecrets(t *testing.T) {
	const raw = "sensitive-value"
	cfg := Config{
		Jira:  Endpoint{Token: Secret(raw)},
		Plane: Endpoint{Token: Secret(raw)},
		Projects: []Project{{
			JiraToken:  Secret(raw),
			PlaneToken: Secret(raw),
		}},
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if strings.Contains(string(encoded), raw) {
		t.Fatalf("config JSON leaked secret: %s", encoded)
	}
	if got := strings.Count(string(encoded), `"***"`); got != 4 {
		t.Errorf("redaction count = %d, want 4 in %s", got, encoded)
	}
	for _, formatted := range []string{
		fmt.Sprint(cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
	} {
		if strings.Contains(formatted, raw) {
			t.Errorf("formatted config leaked secret: %s", formatted)
		}
	}
}

func TestValidateReportsRequiredFields(t *testing.T) {
	err := (Config{}).Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for empty configuration")
	}
	for _, field := range []string{"jira.base_url", "jira.cloud_id", "jira.email", "plane.base_url", "plane.workspace", "defaults.since", "defaults.body_format", "projects"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("Validate() error %q does not mention %s", err, field)
		}
	}
}

func TestValidateRejectsNegativeThrottle(t *testing.T) {
	config := Config{
		Jira:     Endpoint{BaseURL: "https://jira.example.com", CloudID: "cloud", Email: "you@example.com", Token: Secret("token")},
		Plane:    Endpoint{BaseURL: "https://plane.example.com", Workspace: "workspace", Token: Secret("token")},
		Defaults: Defaults{Since: "7d", BodyFormat: "rich", ThrottleMS: -1},
		Projects: []Project{{PlaneProject: "source", JiraProject: "target", JiraIssueType: "Task"}},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "defaults.throttle_ms") {
		t.Fatalf("Validate() error = %v, want throttle validation", err)
	}
}

func TestValidateJiraAuthentication(t *testing.T) {
	valid := Config{
		Jira:     Endpoint{BaseURL: "https://jira.example.com", CloudID: "example-cloud", Email: "you@example.com", Token: Secret("jira-token")},
		Plane:    Endpoint{BaseURL: "https://api.plane.so", Workspace: "example-workspace", Token: Secret("plane-token")},
		Defaults: Defaults{Since: "7d", BodyFormat: "rich"},
		Projects: []Project{{PlaneProject: "SRC", JiraProject: "DST", JiraIssueType: "Task"}},
	}

	tests := []struct {
		name     string
		authType string
		email    string
		wantErr  string
	}{
		{name: "empty defaults to basic", email: "you@example.com"},
		{name: "explicit basic", authType: "basic", email: "you@example.com"},
		{name: "basic requires email", authType: "basic", wantErr: "jira.email is required"},
		{name: "bearer accepts empty email", authType: "bearer"},
		{name: "bearer rejects email", authType: "bearer", email: "you@example.com", wantErr: "jira.email must be empty"},
		{name: "rejects unsupported type", authType: "other", wantErr: "jira.auth_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Jira.AuthType = test.authType
			candidate.Jira.Email = test.email
			err := candidate.Validate()
			if test.wantErr == "" && err != nil {
				t.Fatalf("Validate(): %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestDefaultsAssignAllToMe(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
		want bool
	}{
		{name: "absent defaults true", json: `{}`, want: true},
		{name: "explicit false", json: `{"assign_all_to_me":false}`, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var defaults Defaults
			if err := json.Unmarshal([]byte(test.json), &defaults); err != nil {
				t.Fatalf("json.Unmarshal(): %v", err)
			}
			if got := defaults.ShouldAssignAllToMe(); got != test.want {
				t.Errorf("ShouldAssignAllToMe() = %t, want %t", got, test.want)
			}
		})
	}
}
