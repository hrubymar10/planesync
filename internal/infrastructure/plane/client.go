package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hrubymar10/planesync/internal/domain/configuration"
)

const (
	defaultTimeout   = 30 * time.Second
	maxResponseBytes = 8 << 20
	pageSize         = "100"
)

// Item is a Plane work item used by the synchronization application.
type Item struct {
	ID         string
	Title      string
	BodyHTML   string
	StateName  string
	StateGroup string
	UpdatedAt  time.Time
}

// State is a Plane workflow state.
type State struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// Client reads work items and states from the Plane API.
type Client struct {
	baseURL    *url.URL
	workspace  string
	projectID  string
	token      configuration.Secret
	httpClient *http.Client
}

// String returns a diagnostic representation with the API token redacted.
func (c *Client) String() string {
	if c == nil {
		return "Plane client(<nil>)"
	}
	return fmt.Sprintf("Plane client(base_url=%q, workspace=%q, project_id=%q, token=***)", c.baseURL.String(), c.workspace, c.projectID)
}

// GoString returns a diagnostic representation with the API token redacted.
func (c *Client) GoString() string {
	return c.String()
}

// New creates a Plane client after validating its endpoint and path parameters.
func New(baseURL, workspace, projectID string, token configuration.Secret) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Plane base URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("Plane base URL must use HTTPS and include a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("Plane base URL must not contain user info, a query, or a fragment")
	}
	if err := validatePathSegment("workspace", workspace); err != nil {
		return nil, err
	}
	if err := validatePathSegment("project ID", projectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(token.Reveal()) == "" {
		return nil, fmt.Errorf("Plane API token is required")
	}

	return &Client{
		baseURL:   parsed,
		workspace: workspace,
		projectID: projectID,
		token:     token,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return fmt.Errorf("redirects are not allowed")
			},
		},
	}, nil
}

// States returns every workflow state in the configured project.
func (c *Client) States(ctx context.Context) ([]State, error) {
	states, err := fetchPages[State](ctx, c, "states")
	if err != nil {
		return nil, fmt.Errorf("list Plane states: %w", err)
	}
	return states, nil
}

// ChangedSince returns work items updated at or after since.
func (c *Client) ChangedSince(ctx context.Context, since time.Time) ([]Item, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	changed := make([]Item, 0, len(items))
	for _, item := range items {
		if !item.UpdatedAt.Before(since) {
			changed = append(changed, item)
		}
	}
	return changed, nil
}

// List returns every work item in the configured project.
func (c *Client) List(ctx context.Context) ([]Item, error) {
	states, err := c.States(ctx)
	if err != nil {
		return nil, err
	}
	stateByID := make(map[string]State, len(states))
	for _, state := range states {
		stateByID[state.ID] = state
	}

	workItems, err := fetchPages[workItem](ctx, c, "work-items")
	if err != nil {
		return nil, fmt.Errorf("list Plane work items: %w", err)
	}

	items := make([]Item, 0, len(workItems))
	for _, workItem := range workItems {
		state, ok := stateByID[workItem.State.ID]
		if !ok {
			return nil, fmt.Errorf("work item %q references unknown state %q", workItem.ID, workItem.State.ID)
		}
		items = append(items, Item{
			ID:         workItem.ID,
			Title:      workItem.Name,
			BodyHTML:   workItem.DescriptionHTML,
			StateName:  state.Name,
			StateGroup: state.Group,
			UpdatedAt:  workItem.UpdatedAt,
		})
	}
	return items, nil
}

type workItem struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	DescriptionHTML string    `json:"description_html"`
	State           stateRef  `json:"state"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type stateRef struct {
	ID string
}

func (s *stateRef) UnmarshalJSON(data []byte) error {
	var id string
	if err := json.Unmarshal(data, &id); err == nil {
		s.ID = id
		return nil
	}

	var expanded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &expanded); err != nil {
		return fmt.Errorf("decode state reference: %w", err)
	}
	s.ID = expanded.ID
	return nil
}

type page[T any] struct {
	NextCursor     string `json:"next_cursor"`
	NextPageResult bool   `json:"next_page_results"`
	Results        []T    `json:"results"`
}

func fetchPages[T any](ctx context.Context, client *Client, resource string) ([]T, error) {
	var all []T
	cursor := ""
	seenCursors := make(map[string]struct{})

	for {
		var current page[T]
		if err := client.get(ctx, resource, cursor, &current); err != nil {
			return nil, err
		}
		all = append(all, current.Results...)
		if !current.NextPageResult {
			return all, nil
		}
		if current.NextCursor == "" {
			return nil, fmt.Errorf("%s pagination advertised another page without a cursor", resource)
		}
		if _, exists := seenCursors[current.NextCursor]; exists {
			return nil, fmt.Errorf("%s pagination repeated a cursor", resource)
		}
		seenCursors[current.NextCursor] = struct{}{}
		cursor = current.NextCursor
	}
}

func (c *Client) get(ctx context.Context, resource, cursor string, destination any) error {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") +
		"/api/v1/workspaces/" + c.workspace +
		"/projects/" + c.projectID +
		"/" + resource + "/"
	query := requestURL.Query()
	query.Set("per_page", pageSize)
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create %s request: %w", resource, err)
	}
	request.Header.Set("X-API-Key", c.token.Reveal())

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send %s request: %w", resource, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s request returned HTTP %d", resource, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read %s response: %w", resource, err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("%s response exceeds %d bytes", resource, maxResponseBytes)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode %s response: %w", resource, err)
	}
	return nil
}

func validatePathSegment(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("Plane %s is required", name)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("Plane %s must be a single URL path segment", name)
	}
	return nil
}
