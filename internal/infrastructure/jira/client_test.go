package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hrubymar10/planesync/internal/domain/configuration"
)

const testToken = "top-secret-token"

func TestFindByLabelHitAndMiss(t *testing.T) {
	for _, test := range []struct {
		name      string
		response  string
		wantKey   string
		wantFound bool
	}{
		{name: "hit", response: `{"issues":[{"key":"found-key"}]}`, wantKey: "found-key", wantFound: true},
		{name: "miss", response: `{"issues":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertRequest(t, request, http.MethodPost, "/rest/api/3/search/jql")
				var body struct {
					JQL        string `json:"jql"`
					MaxResults int    `json:"maxResults"`
				}
				decodeRequest(t, request, &body)
				if body.JQL != `labels = "source-label"` || body.MaxResults != 1 {
					t.Errorf("search body = %#v", body)
				}
				writeJSON(writer, http.StatusOK, test.response)
			}))
			defer server.Close()

			key, found, err := client.FindByLabel(context.Background(), "source-label")
			if err != nil {
				t.Fatalf("FindByLabel(): %v", err)
			}
			if key != test.wantKey || found != test.wantFound {
				t.Errorf("FindByLabel() = %q, %t; want %q, %t", key, found, test.wantKey, test.wantFound)
			}
		})
	}
}

func TestCurrentUserAccountID(t *testing.T) {
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodGet, "/rest/api/3/myself")
		writeJSON(writer, http.StatusOK, `{"accountId":"account-current"}`)
	}))
	defer server.Close()

	accountID, err := client.CurrentUserAccountID(context.Background())
	if err != nil {
		t.Fatalf("CurrentUserAccountID(): %v", err)
	}
	if accountID != "account-current" {
		t.Errorf("account ID = %q", accountID)
	}
}

func TestCreateSendsFieldsLabelsAndADF(t *testing.T) {
	description := TextToADF("First paragraph\nSecond paragraph")
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodPost, "/rest/api/3/issue")
		var body struct {
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
				Assignee    *assignee       `json:"assignee"`
				Parent      *parent         `json:"parent"`
				Components  []component     `json:"components"`
			} `json:"fields"`
		}
		decodeRequest(t, request, &body)
		if body.Fields.Project.Key != "DST" || body.Fields.IssueType.Name != "Task" || body.Fields.Summary != "Example summary" {
			t.Errorf("create fields = %#v", body.Fields)
		}
		if !reflect.DeepEqual(body.Fields.Labels, []string{"source-label", "sync-label"}) {
			t.Errorf("labels = %#v", body.Fields.Labels)
		}
		if body.Fields.Assignee == nil || body.Fields.Assignee.AccountID != "account-current" {
			t.Errorf("assignee = %#v", body.Fields.Assignee)
		}
		if body.Fields.Parent == nil || body.Fields.Parent.Key != "epic-parent" {
			t.Errorf("parent = %#v", body.Fields.Parent)
		}
		if !reflect.DeepEqual(body.Fields.Components, []component{{Name: "Backend"}, {Name: "API"}}) {
			t.Errorf("components = %#v", body.Fields.Components)
		}
		if !jsonEqual(body.Fields.Description, description) {
			t.Errorf("description = %s, want %s", body.Fields.Description, description)
		}
		writeJSON(writer, http.StatusCreated, `{"key":"created-key"}`)
	}))
	defer server.Close()

	key, err := client.Create(context.Background(), CreateInput{
		Project:           "DST",
		IssueType:         "Task",
		Summary:           "Example summary",
		Description:       description,
		Labels:            []string{"source-label", "sync-label"},
		AssigneeAccountID: "account-current",
		ParentEpicKey:     "epic-parent",
		Components:        []string{"Backend", "API"},
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if key != "created-key" {
		t.Errorf("Create() key = %q", key)
	}
}

func TestUpdate(t *testing.T) {
	description := TextToADF("Updated body")
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodPut, "/rest/api/3/issue/example-key")
		var body struct {
			Fields struct {
				Summary     string          `json:"summary"`
				Description json.RawMessage `json:"description"`
				Assignee    *assignee       `json:"assignee"`
				Parent      *parent         `json:"parent"`
				Components  []component     `json:"components"`
			} `json:"fields"`
		}
		decodeRequest(t, request, &body)
		if body.Fields.Summary != "Updated summary" || !jsonEqual(body.Fields.Description, description) {
			t.Errorf("update fields = %#v", body.Fields)
		}
		if body.Fields.Assignee == nil || body.Fields.Assignee.AccountID != "account-current" {
			t.Errorf("assignee = %#v", body.Fields.Assignee)
		}
		if body.Fields.Parent == nil || body.Fields.Parent.Key != "epic-parent" {
			t.Errorf("parent = %#v", body.Fields.Parent)
		}
		if !reflect.DeepEqual(body.Fields.Components, []component{{Name: "Backend"}}) {
			t.Errorf("components = %#v", body.Fields.Components)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := client.Update(context.Background(), "example-key", UpdateInput{Summary: "Updated summary", Description: description, AssigneeAccountID: "account-current", ParentEpicKey: "epic-parent", Components: []string{"Backend"}}); err != nil {
		t.Fatalf("Update(): %v", err)
	}
}

func TestCreateAndUpdateOmitEmptyAssignee(t *testing.T) {
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Fields map[string]json.RawMessage `json:"fields"`
		}
		decodeRequest(t, request, &body)
		if _, exists := body.Fields["assignee"]; exists {
			t.Error("empty assignee was included in fields")
		}
		if _, exists := body.Fields["parent"]; exists {
			t.Error("empty parent was included in fields")
		}
		if _, exists := body.Fields["components"]; exists {
			t.Error("empty components were included in fields")
		}
		if request.Method == http.MethodPost {
			writeJSON(writer, http.StatusCreated, `{"key":"created-key"}`)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if _, err := client.Create(context.Background(), CreateInput{Project: "DST", IssueType: "Task", Summary: "Summary", Description: TextToADF("Body")}); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if err := client.Update(context.Background(), "example-key", UpdateInput{Summary: "Summary", Description: TextToADF("Body")}); err != nil {
		t.Fatalf("Update(): %v", err)
	}
}

func TestSetStatusMatchesTargetCaseInsensitively(t *testing.T) {
	requests := 0
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		assertRequest(t, request, request.Method, "/rest/api/3/issue/example-key/transitions")
		switch request.Method {
		case http.MethodGet:
			writeJSON(writer, http.StatusOK, `{"transitions":[{"id":"transition-one","to":{"name":"In Progress"}},{"id":"transition-two","to":{"name":"Done"}}]}`)
		case http.MethodPost:
			var body struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
				Fields struct {
					Resolution struct {
						Name string `json:"name"`
					} `json:"resolution"`
				} `json:"fields"`
			}
			decodeRequest(t, request, &body)
			if body.Transition.ID != "transition-two" {
				t.Errorf("transition ID = %q", body.Transition.ID)
			}
			if body.Fields.Resolution.Name != "Declined" {
				t.Errorf("resolution = %q", body.Fields.Resolution.Name)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
	}))
	defer server.Close()

	if err := client.SetStatus(context.Background(), "example-key", "done", "Declined"); err != nil {
		t.Fatalf("SetStatus(): %v", err)
	}
	if requests != 2 {
		t.Errorf("request count = %d, want 2", requests)
	}
}

func TestSetStatusOmitsEmptyResolution(t *testing.T) {
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeJSON(writer, http.StatusOK, `{"transitions":[{"id":"transition-one","to":{"name":"Done"}}]}`)
			return
		}
		var body map[string]json.RawMessage
		decodeRequest(t, request, &body)
		if _, exists := body["fields"]; exists {
			t.Error("empty resolution emitted fields")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := client.SetStatus(context.Background(), "example-key", "Done", ""); err != nil {
		t.Fatalf("SetStatus(): %v", err)
	}
}

func TestSetStatusReturnsClearNoMatchError(t *testing.T) {
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, `{"transitions":[{"id":"transition-one","to":{"name":"Started"}}]}`)
	}))
	defer server.Close()

	err := client.SetStatus(context.Background(), "example-key", "Done", "")
	if err == nil || !strings.Contains(err.Error(), `no Jira transition targets status "Done"`) {
		t.Fatalf("SetStatus() error = %v", err)
	}
}

func TestAuthenticationHeaders(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		authType string
		want     string
	}{
		{
			name:  "empty defaults to basic",
			email: "you@example.com",
			want:  "Basic " + base64.StdEncoding.EncodeToString([]byte("you@example.com:"+testToken)),
		},
		{
			name:     "basic",
			email:    "you@example.com",
			authType: "basic",
			want:     "Basic " + base64.StdEncoding.EncodeToString([]byte("you@example.com:"+testToken)),
		},
		{name: "bearer", authType: "bearer", want: "Bearer " + testToken},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, server := newTestClient(t, test.email, test.authType, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if got := request.Header.Get("Authorization"); got != test.want {
					t.Errorf("Authorization header is missing or incorrect for %s authentication", test.authType)
				}
				writeJSON(writer, http.StatusOK, `{"issues":[]}`)
			}))
			defer server.Close()
			if _, _, err := client.FindByLabel(context.Background(), "source-label"); err != nil {
				t.Fatalf("FindByLabel(): %v", err)
			}
		})
	}
}

func TestFindByLabelEscapesJQLString(t *testing.T) {
	escaped, err := escapeJQLString(`quoted" label\value`)
	if err != nil {
		t.Fatalf("escapeJQLString(): %v", err)
	}
	if escaped != `quoted\" label\\value` {
		t.Errorf("escaped label = %q", escaped)
	}
	if _, err := escapeJQLString("line\nbreak"); err == nil {
		t.Fatal("escapeJQLString() accepted a control character")
	}
}

func TestNewRejectsHTTPAndClientRedactsAuthorization(t *testing.T) {
	client, err := New("http://jira.example.com", "example-cloud", "you@example.com", "basic", configuration.Secret(testToken))
	if err == nil || client != nil {
		t.Fatalf("New() = %#v, %v; want HTTPS rejection", client, err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatal("constructor error leaked token")
	}

	client, err = New("https://jira.example.com", "example-cloud", "you@example.com", "basic", configuration.Secret(testToken))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if got := client.httpClient.Timeout; got != defaultTimeout {
		t.Errorf("HTTP timeout = %s, want %s", got, defaultTimeout)
	}
	for _, output := range []string{fmt.Sprint(client), fmt.Sprintf("%+v", client), fmt.Sprintf("%#v", client), fmt.Sprintf("%+v", *client), fmt.Sprintf("%#v", *client)} {
		if strings.Contains(output, testToken) || strings.Contains(output, client.authorization.Reveal()) {
			t.Errorf("client formatting leaked authorization: %s", output)
		}
	}
}

func TestErrorResponseIncludesBoundedBodyWithoutAuthorization(t *testing.T) {
	const detail = `{"errors":{"components":"Component is required"}}`
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(detail + strings.Repeat("x", maxErrorSize)))
	}))
	defer server.Close()

	_, _, err := client.FindByLabel(context.Background(), "source-label")
	if err == nil {
		t.Fatal("FindByLabel() returned nil error")
	}
	if !strings.Contains(err.Error(), detail) {
		t.Fatalf("error omitted response body: %v", err)
	}
	if len(err.Error()) > maxErrorSize+200 {
		t.Fatalf("error body was not bounded: %d bytes", len(err.Error()))
	}
	if strings.Contains(err.Error(), client.authorization.Reveal()) {
		t.Fatalf("error leaked authorization: %v", err)
	}
}

func TestTextToADF(t *testing.T) {
	var document struct {
		Version int    `json:"version"`
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(TextToADF("first\n\nthird"), &document); err != nil {
		t.Fatalf("unmarshal ADF: %v", err)
	}
	if document.Version != 1 || document.Type != "doc" || len(document.Content) != 3 {
		t.Fatalf("ADF document = %#v", document)
	}
	if document.Content[0].Content[0].Text != "first" || len(document.Content[1].Content) != 0 || document.Content[2].Content[0].Text != "third" {
		t.Errorf("ADF paragraphs = %#v", document.Content)
	}
}

func TestResponseBodyIsBounded(t *testing.T) {
	client, server := newTestClient(t, "you@example.com", "basic", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat(" ", maxResponseSize+1)))
	}))
	defer server.Close()

	_, _, err := client.FindByLabel(context.Background(), "source-label")
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("FindByLabel() error = %v, want response-size error", err)
	}
}

func newTestClient(t *testing.T, email, authType string, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	client, err := New(server.URL, "example-cloud", email, authType, configuration.Secret(testToken))
	if err != nil {
		server.Close()
		t.Fatalf("New(): %v", err)
	}
	client.httpClient.Transport = server.Client().Transport
	return client, server
}

func assertRequest(t *testing.T, request *http.Request, method, path string) {
	t.Helper()
	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != path {
		t.Errorf("path = %q, want %q", request.URL.Path, path)
	}
	if request.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept header = %q", request.Header.Get("Accept"))
	}
	if request.Method != http.MethodGet && request.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type header = %q", request.Header.Get("Content-Type"))
	}
}

func decodeRequest(t *testing.T, request *http.Request, destination any) {
	t.Helper()
	defer request.Body.Close()
	if err := json.NewDecoder(request.Body).Decode(destination); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeJSON(writer http.ResponseWriter, status int, document string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(document))
}

func jsonEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}
