package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/hrubymar10/planesync/internal/domain/configuration"
	"github.com/hrubymar10/planesync/internal/infrastructure/httpbase"
)

const (
	defaultAuthType = "basic"
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 8 << 20
	maxErrorSize    = 2 << 10
)

// CreateInput contains the Jira fields set when creating an issue.
type CreateInput struct {
	Project           string
	IssueType         string
	Summary           string
	Description       json.RawMessage
	Labels            []string
	AssigneeAccountID string
	ParentEpicKey     string
	Components        []string
}

// UpdateInput contains the Jira fields updated on an existing issue.
type UpdateInput struct {
	Summary           string
	Description       json.RawMessage
	AssigneeAccountID string
	ParentEpicKey     string
	Components        []string
}

type assignee struct {
	AccountID string `json:"accountId"`
}

type parent struct {
	Key string `json:"key"`
}

type component struct {
	Name string `json:"name"`
}

// Client creates, updates, searches, and transitions Jira issues.
type Client struct {
	baseURL       *url.URL
	cloudID       string
	authType      string
	authorization configuration.Secret
	httpClient    *http.Client
}

// String returns a diagnostic representation with authorization redacted.
func (c Client) String() string {
	return fmt.Sprintf("Jira client(base_url=%q, cloud_id=%q, auth_type=%q, authorization=***)", c.baseURL.String(), c.cloudID, c.authType)
}

// GoString returns a diagnostic representation with authorization redacted.
func (c Client) GoString() string {
	return c.String()
}

// New creates a Jira client after validating its endpoint and authentication.
func New(baseURL, cloudID, email, authType string, token configuration.Secret) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Jira base URL: %w", err)
	}
	if err := httpbase.Validate("Jira", parsed); err != nil {
		return nil, err
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("Jira base URL must not contain user info, a query, or a fragment")
	}
	if strings.TrimSpace(cloudID) == "" {
		return nil, fmt.Errorf("Jira cloud ID is required")
	}
	if strings.TrimSpace(token.Reveal()) == "" {
		return nil, fmt.Errorf("Jira API token is required")
	}

	normalizedAuthType := strings.ToLower(strings.TrimSpace(authType))
	if normalizedAuthType == "" {
		normalizedAuthType = defaultAuthType
	}
	var authorization string
	switch normalizedAuthType {
	case "basic":
		if strings.TrimSpace(email) == "" {
			return nil, fmt.Errorf("Jira email is required for basic authentication")
		}
		credentials := base64.StdEncoding.EncodeToString([]byte(email + ":" + token.Reveal()))
		authorization = "Basic " + credentials
	case "bearer":
		if strings.TrimSpace(email) != "" {
			return nil, fmt.Errorf("Jira email must be empty for bearer authentication")
		}
		authorization = "Bearer " + token.Reveal()
	default:
		return nil, fmt.Errorf("Jira auth type must be basic or bearer")
	}

	return &Client{
		baseURL:       parsed,
		cloudID:       cloudID,
		authType:      normalizedAuthType,
		authorization: configuration.Secret(authorization),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return fmt.Errorf("redirects are not allowed")
			},
		},
	}, nil
}

// FindByLabel returns the first issue matching label.
func (c *Client) FindByLabel(ctx context.Context, label string) (string, bool, error) {
	escaped, err := escapeJQLString(label)
	if err != nil {
		return "", false, err
	}
	request := struct {
		JQL        string `json:"jql"`
		MaxResults int    `json:"maxResults"`
	}{
		JQL:        `labels = "` + escaped + `"`,
		MaxResults: 1,
	}
	var response struct {
		Issues []struct {
			Key string `json:"key"`
		} `json:"issues"`
	}
	if err := c.do(ctx, http.MethodPost, "search/jql", request, http.StatusOK, &response); err != nil {
		return "", false, fmt.Errorf("search Jira issues by label: %w", err)
	}
	if len(response.Issues) == 0 {
		return "", false, nil
	}
	if response.Issues[0].Key == "" {
		return "", false, fmt.Errorf("search Jira issues by label: response issue has no key")
	}
	return response.Issues[0].Key, true, nil
}

// CurrentUserAccountID returns the account ID of the authenticating Jira user.
func (c *Client) CurrentUserAccountID(ctx context.Context) (string, error) {
	var response struct {
		AccountID string `json:"accountId"`
	}
	if err := c.do(ctx, http.MethodGet, "myself", nil, http.StatusOK, &response); err != nil {
		return "", fmt.Errorf("get current Jira user: %w", err)
	}
	if strings.TrimSpace(response.AccountID) == "" {
		return "", fmt.Errorf("get current Jira user: response has no accountId")
	}
	return response.AccountID, nil
}

// Create creates a Jira issue and returns its key.
func (c *Client) Create(ctx context.Context, input CreateInput) (string, error) {
	request := struct {
		Fields struct {
			Project struct {
				Key string `json:"key"`
			} `json:"project"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Labels      []string        `json:"labels"`
			Assignee    *assignee       `json:"assignee,omitempty"`
			Parent      *parent         `json:"parent,omitempty"`
			Components  []component     `json:"components,omitempty"`
		} `json:"fields"`
	}{}
	request.Fields.Project.Key = input.Project
	request.Fields.IssueType.Name = input.IssueType
	request.Fields.Summary = input.Summary
	request.Fields.Description = input.Description
	request.Fields.Labels = append([]string{}, input.Labels...)
	if input.AssigneeAccountID != "" {
		request.Fields.Assignee = &assignee{AccountID: input.AssigneeAccountID}
	}
	if input.ParentEpicKey != "" {
		request.Fields.Parent = &parent{Key: input.ParentEpicKey}
	}
	for _, name := range input.Components {
		request.Fields.Components = append(request.Fields.Components, component{Name: name})
	}

	var response struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, http.MethodPost, "issue", request, http.StatusCreated, &response); err != nil {
		return "", fmt.Errorf("create Jira issue: %w", err)
	}
	if response.Key == "" {
		return "", fmt.Errorf("create Jira issue: response has no key")
	}
	return response.Key, nil
}

// Update changes the summary and description of a Jira issue.
func (c *Client) Update(ctx context.Context, key string, input UpdateInput) error {
	if err := validatePathSegment("issue key", key); err != nil {
		return err
	}
	request := struct {
		Fields struct {
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Assignee    *assignee       `json:"assignee,omitempty"`
			Parent      *parent         `json:"parent,omitempty"`
			Components  []component     `json:"components,omitempty"`
		} `json:"fields"`
	}{}
	request.Fields.Summary = input.Summary
	request.Fields.Description = input.Description
	if input.AssigneeAccountID != "" {
		request.Fields.Assignee = &assignee{AccountID: input.AssigneeAccountID}
	}
	if input.ParentEpicKey != "" {
		request.Fields.Parent = &parent{Key: input.ParentEpicKey}
	}
	for _, name := range input.Components {
		request.Fields.Components = append(request.Fields.Components, component{Name: name})
	}
	if err := c.do(ctx, http.MethodPut, "issue/"+key, request, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("update Jira issue: %w", err)
	}
	return nil
}

// SetStatus transitions a Jira issue to the named target status.
func (c *Client) SetStatus(ctx context.Context, key, statusName, resolution string) error {
	if err := validatePathSegment("issue key", key); err != nil {
		return err
	}
	if strings.TrimSpace(statusName) == "" {
		return fmt.Errorf("Jira target status name is required")
	}

	var response struct {
		Transitions []struct {
			ID string `json:"id"`
			To struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	resource := "issue/" + key + "/transitions"
	if err := c.do(ctx, http.MethodGet, resource, nil, http.StatusOK, &response); err != nil {
		return fmt.Errorf("list Jira issue transitions: %w", err)
	}

	var transitionID string
	for _, transition := range response.Transitions {
		if strings.EqualFold(strings.TrimSpace(transition.To.Name), strings.TrimSpace(statusName)) {
			transitionID = transition.ID
			break
		}
	}
	if transitionID == "" {
		return fmt.Errorf("no Jira transition targets status %q", statusName)
	}

	request := struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
		Fields *struct {
			Resolution struct {
				Name string `json:"name"`
			} `json:"resolution"`
		} `json:"fields,omitempty"`
	}{}
	request.Transition.ID = transitionID
	if resolution != "" {
		request.Fields = &struct {
			Resolution struct {
				Name string `json:"name"`
			} `json:"resolution"`
		}{}
		request.Fields.Resolution.Name = resolution
	}
	if err := c.do(ctx, http.MethodPost, resource, request, http.StatusNoContent, nil); err != nil {
		return fmt.Errorf("transition Jira issue: %w", err)
	}
	return nil
}

// TextToADF converts newline-delimited plain text to a minimal ADF document.
func TextToADF(text string) json.RawMessage {
	type textNode struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type paragraph struct {
		Type    string     `json:"type"`
		Content []textNode `json:"content,omitempty"`
	}
	document := struct {
		Version int         `json:"version"`
		Type    string      `json:"type"`
		Content []paragraph `json:"content"`
	}{Version: 1, Type: "doc"}

	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		part := paragraph{Type: "paragraph"}
		if line != "" {
			part.Content = []textNode{{Type: "text", Text: line}}
		}
		document.Content = append(document.Content, part)
	}
	encoded, _ := json.Marshal(document)
	return encoded
}

func (c *Client) do(ctx context.Context, method, resource string, input any, expectedStatus int, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/rest/api/3/" + resource
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", c.authorization.Reveal())
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		snippet, readErr := io.ReadAll(io.LimitReader(response.Body, maxErrorSize))
		if readErr != nil {
			return fmt.Errorf("Jira API returned HTTP %d (read error body: %v)", response.StatusCode, readErr)
		}
		if detail := strings.TrimSpace(string(snippet)); detail != "" {
			return fmt.Errorf("Jira API returned HTTP %d: %s", response.StatusCode, detail)
		}
		return fmt.Errorf("Jira API returned HTTP %d", response.StatusCode)
	}
	if output == nil {
		return nil
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return fmt.Errorf("Jira API response exceeds %d bytes", maxResponseSize)
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func validatePathSegment(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("Jira %s is required", name)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("Jira %s must be a single URL path segment", name)
	}
	return nil
}

func escapeJQLString(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("Jira label is required")
	}
	var result strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("Jira label must not contain control characters")
		}
		if character == '\\' || character == '"' {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
	}
	return result.String(), nil
}
