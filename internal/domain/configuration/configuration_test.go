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
	for _, field := range []string{"jira.base_url", "jira.cloud_id", "plane.base_url", "plane.workspace", "defaults.since", "defaults.body_format", "projects"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("Validate() error %q does not mention %s", err, field)
		}
	}
}
