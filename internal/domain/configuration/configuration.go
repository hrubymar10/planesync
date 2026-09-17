// Package configuration defines the tool's configuration model.
package configuration

import (
	"fmt"
	"strings"
)

// Config describes tracker endpoints, defaults, and project mappings.
type Config struct {
	Jira     Endpoint  `json:"jira"`
	Plane    Endpoint  `json:"plane"`
	Defaults Defaults  `json:"defaults"`
	Projects []Project `json:"projects"`
}

// Endpoint describes a tracker API endpoint and its default token.
type Endpoint struct {
	BaseURL    string `json:"base_url"`
	AppBaseURL string `json:"app_base_url,omitempty"`
	CloudID    string `json:"cloud_id,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	Email      string `json:"email,omitempty"`
	AuthType   string `json:"auth_type,omitempty"`
	Token      Secret `json:"token,omitempty"`
}

// Defaults contains settings shared by all project mappings.
type Defaults struct {
	Since         string `json:"since"`
	BodyFormat    string `json:"body_format"`
	DeletedStatus string `json:"deleted_status,omitempty"`
	AssignAllToMe *bool  `json:"assign_all_to_me,omitempty"`
}

// ShouldAssignAllToMe reports whether mirrored issues should be assigned to the authenticating user.
func (d Defaults) ShouldAssignAllToMe() bool {
	return d.AssignAllToMe == nil || *d.AssignAllToMe
}

// Project maps a Plane project to a Jira project.
type Project struct {
	PlaneProject   string            `json:"plane_project"`
	JiraProject    string            `json:"jira_project"`
	JiraIssueType  string            `json:"jira_issue_type"`
	TitlePrefix    string            `json:"title_prefix,omitempty"`
	JiraToken      Secret            `json:"jira_token,omitempty"`
	PlaneToken     Secret            `json:"plane_token,omitempty"`
	StatusMap      map[string]string `json:"status_map"`
	StatusGroupMap map[string]string `json:"status_group_map,omitempty"`
}

// Secret holds a sensitive configuration value.
type Secret string

// String returns a redacted representation of the secret.
func (Secret) String() string {
	return "***"
}

// GoString returns a redacted representation of the secret.
func (Secret) GoString() string {
	return "***"
}

// MarshalText returns a redacted representation of the secret.
func (Secret) MarshalText() ([]byte, error) {
	return []byte("***"), nil
}

// UnmarshalText stores a secret read from configuration.
func (s *Secret) UnmarshalText(text []byte) error {
	*s = Secret(text)
	return nil
}

// Reveal returns the sensitive value explicitly.
func (s Secret) Reveal() string {
	return string(s)
}

// Validate checks that all required configuration values are present and valid.
func (c Config) Validate() error {
	var problems []string
	require := func(path, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, path+" is required")
		}
	}

	require("jira.base_url", c.Jira.BaseURL)
	require("jira.cloud_id", c.Jira.CloudID)
	switch strings.ToLower(strings.TrimSpace(c.Jira.AuthType)) {
	case "", "basic":
		require("jira.email", c.Jira.Email)
	case "bearer":
		if strings.TrimSpace(c.Jira.Email) != "" {
			problems = append(problems, "jira.email must be empty when jira.auth_type is bearer")
		}
	default:
		problems = append(problems, `jira.auth_type must be "basic" or "bearer"`)
	}
	require("plane.base_url", c.Plane.BaseURL)
	require("plane.workspace", c.Plane.Workspace)
	require("defaults.since", c.Defaults.Since)

	switch c.Defaults.BodyFormat {
	case "rich", "text":
	case "":
		problems = append(problems, "defaults.body_format is required")
	default:
		problems = append(problems, `defaults.body_format must be "rich" or "text"`)
	}

	if len(c.Projects) == 0 {
		problems = append(problems, "projects must contain at least one project")
	}
	for i, project := range c.Projects {
		prefix := fmt.Sprintf("projects[%d].", i)
		require(prefix+"plane_project", project.PlaneProject)
		require(prefix+"jira_project", project.JiraProject)
		require(prefix+"jira_issue_type", project.JiraIssueType)
		if strings.TrimSpace(project.JiraToken.Reveal()) == "" && strings.TrimSpace(c.Jira.Token.Reveal()) == "" {
			problems = append(problems, prefix+"jira_token is required")
		}
		if strings.TrimSpace(project.PlaneToken.Reveal()) == "" && strings.TrimSpace(c.Plane.Token.Reveal()) == "" {
			problems = append(problems, prefix+"plane_token is required")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}
