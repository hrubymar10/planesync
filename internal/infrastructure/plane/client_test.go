package plane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hrubymar10/planesync/internal/domain/configuration"
)

const testToken = "top-secret-token"

func TestStates(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, "/api/v1/workspaces/example-workspace/projects/example-project/states/", "")
		writeJSON(writer, `{"next_cursor":"","next_page_results":false,"results":[{"id":"state-one","name":"Started","group":"started"}]}`)
	}))
	defer server.Close()

	states, err := client.States(context.Background())
	if err != nil {
		t.Fatalf("States(): %v", err)
	}
	if len(states) != 1 || states[0] != (State{ID: "state-one", Name: "Started", Group: "started"}) {
		t.Fatalf("States() = %#v", states)
	}
}

func TestChangedSinceIncludesBoundaryAndFollowsCursor(t *testing.T) {
	boundary := time.Date(2026, time.September, 17, 10, 0, 0, 0, time.UTC)
	var stateRequests atomic.Int32
	var itemRequests atomic.Int32
	var projectRequests atomic.Int32

	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/workspaces/example-workspace/projects/example-project/":
			projectRequests.Add(1)
			assertProjectRequest(t, request)
			writeJSON(writer, `{"identifier":"SRC"}`)
		case "/api/v1/workspaces/example-workspace/projects/example-project/states/":
			stateRequests.Add(1)
			assertRequest(t, request, request.URL.Path, "")
			writeJSON(writer, `{"next_cursor":"","next_page_results":false,"results":[{"id":"state-one","name":"Started","group":"started"}]}`)
		case "/api/v1/workspaces/example-workspace/projects/example-project/work-items/":
			itemRequests.Add(1)
			if request.URL.Query().Get("cursor") == "" {
				assertRequest(t, request, request.URL.Path, "")
				writeJSON(writer, `{
					"next_cursor":"100:1:0",
					"next_page_results":true,
					"results":[
						{"id":"before","sequence_id":15,"name":"Before","description_html":"<p>before</p>","state":"state-one","updated_at":"2026-09-17T09:59:59Z"},
						{"id":"boundary","sequence_id":16,"name":"Boundary","description_html":"<p>boundary</p>","state":"state-one","updated_at":"2026-09-17T10:00:00Z"}
					]
				}`)
				return
			}
			assertRequest(t, request, request.URL.Path, "100:1:0")
			writeJSON(writer, `{
				"next_cursor":"",
				"next_page_results":false,
				"results":[{"id":"after","sequence_id":17,"name":"After","description_html":"<p>after</p>","state":{"id":"state-one"},"updated_at":"2026-09-17T10:00:01Z"}]
			}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	items, err := client.ChangedSince(context.Background(), boundary)
	if err != nil {
		t.Fatalf("ChangedSince(): %v", err)
	}
	if len(items) != 2 || items[0].ID != "boundary" || items[1].ID != "after" {
		t.Fatalf("ChangedSince() IDs = %v, want boundary and after", itemIDs(items))
	}
	for _, item := range items {
		if item.StateName != "Started" || item.StateGroup != "started" {
			t.Errorf("state join for %s = %q/%q", item.ID, item.StateName, item.StateGroup)
		}
	}
	if items[0].Identifier != "SRC-16" || items[1].Identifier != "SRC-17" {
		t.Errorf("identifiers = %q, %q", items[0].Identifier, items[1].Identifier)
	}
	if got := projectRequests.Load(); got != 1 {
		t.Errorf("project requests = %d, want 1", got)
	}
	if got := stateRequests.Load(); got != 1 {
		t.Errorf("state requests = %d, want 1", got)
	}
	if got := itemRequests.Load(); got != 2 {
		t.Errorf("work item requests = %d, want 2", got)
	}
}

func TestListReturnsFullJoinedItems(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/workspaces/example-workspace/projects/example-project/":
			assertProjectRequest(t, request)
			writeJSON(writer, `{"identifier":"SRC"}`)
		case "/api/v1/workspaces/example-workspace/projects/example-project/states/":
			writeJSON(writer, `{"next_page_results":false,"results":[{"id":"state-done","name":"Done","group":"completed"}]}`)
		case "/api/v1/workspaces/example-workspace/projects/example-project/work-items/":
			writeJSON(writer, `{"next_page_results":false,"results":[{"id":"item-one","sequence_id":16,"name":"A title","description_html":"<p>Body</p>","state":"state-done","updated_at":"2026-09-17T10:00:00Z"}]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	items, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List() count = %d, want 1", len(items))
	}
	item := items[0]
	if item.ID != "item-one" || item.Identifier != "SRC-16" || item.Title != "A title" || item.BodyHTML != "<p>Body</p>" || item.StateName != "Done" || item.StateGroup != "completed" {
		t.Errorf("List() item = %#v", item)
	}
}

func TestNewRejectsNonHTTPSBaseURL(t *testing.T) {
	client, err := New("http://plane.example.com", "workspace", "project", configuration.Secret(testToken))
	if err == nil || client != nil {
		t.Fatalf("New() = %#v, %v; want rejection", client, err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatal("New() error leaked token")
	}
}

func TestNewSetsRequestTimeout(t *testing.T) {
	client, err := New("https://plane.example.com", "workspace", "project", configuration.Secret(testToken))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if got := client.httpClient.Timeout; got != defaultTimeout {
		t.Errorf("HTTP timeout = %s, want %s", got, defaultTimeout)
	}
}

func TestErrorsAndFormattingNeverRevealToken(t *testing.T) {
	const detail = `{"detail":"permission denied"}`
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(detail + strings.Repeat("x", maxErrorBytes)))
	}))
	defer server.Close()

	_, err := client.States(context.Background())
	if err == nil {
		t.Fatal("States() returned nil error")
	}
	if !strings.Contains(err.Error(), detail) || len(err.Error()) > maxErrorBytes+200 {
		t.Errorf("error did not contain a bounded response body: %v", err)
	}
	for _, output := range []string{err.Error(), fmt.Sprint(client), fmt.Sprintf("%+v", client), fmt.Sprintf("%#v", client)} {
		if strings.Contains(output, testToken) {
			t.Errorf("token leaked in output: %s", output)
		}
	}
}

func TestPaginationRejectsRepeatedCursor(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, `{"next_cursor":"same","next_page_results":true,"results":[]}`)
	}))
	defer server.Close()

	_, err := client.States(context.Background())
	if err == nil || !strings.Contains(err.Error(), "repeated a cursor") {
		t.Fatalf("States() error = %v, want repeated cursor error", err)
	}
}

func TestResponseBodyIsBounded(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat(" ", maxResponseBytes+1)))
	}))
	defer server.Close()

	_, err := client.States(context.Background())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("States() error = %v, want response-size error", err)
	}
}

func TestStatesRetriesRateLimitWithRetryAfter(t *testing.T) {
	attempts := 0
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			writer.Header().Set("Retry-After", "2")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(writer, `{"next_page_results":false,"results":[{"id":"state-one","name":"Started","group":"started"}]}`)
	}))
	defer server.Close()

	var waits []time.Duration
	client.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	states, err := client.States(context.Background())
	if err != nil {
		t.Fatalf("States(): %v", err)
	}
	if len(states) != 1 || attempts != 2 || len(waits) != 1 || waits[0] != 2*time.Second {
		t.Errorf("states/attempts/waits = %#v / %d / %v", states, attempts, waits)
	}
}

func TestStatesStopsAfterRateLimitAttemptCap(t *testing.T) {
	attempts := 0
	client, server := newTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client.wait = func(context.Context, time.Duration) error { return nil }

	_, err := client.States(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || attempts != maxRequestAttempts {
		t.Fatalf("States() error/attempts = %v / %d", err, attempts)
	}
}

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	client, err := New(server.URL, "example-workspace", "example-project", configuration.Secret(testToken))
	if err != nil {
		server.Close()
		t.Fatalf("New(): %v", err)
	}
	client.httpClient.Transport = server.Client().Transport
	return client, server
}

func assertRequest(t *testing.T, request *http.Request, path, cursor string) {
	t.Helper()
	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if request.URL.Path != path {
		t.Errorf("path = %q, want %q", request.URL.Path, path)
	}
	if got := request.Header.Get("X-API-Key"); got != testToken {
		t.Errorf("X-API-Key header is missing or incorrect")
	}
	if got := request.URL.Query().Get("per_page"); got != pageSize {
		t.Errorf("per_page = %q, want %q", got, pageSize)
	}
	if got := request.URL.Query().Get("cursor"); got != cursor {
		t.Errorf("cursor = %q, want %q", got, cursor)
	}
}

func assertProjectRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if got := request.Header.Get("X-API-Key"); got != testToken {
		t.Errorf("X-API-Key header is missing or incorrect")
	}
	if request.URL.RawQuery != "" {
		t.Errorf("project query = %q, want empty", request.URL.RawQuery)
	}
}

func writeJSON(writer http.ResponseWriter, document string) {
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(document))
}

func itemIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
