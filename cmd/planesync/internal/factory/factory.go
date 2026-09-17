package factory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	appsync "github.com/hrubymar10/planesync/internal/application/sync"
	"github.com/hrubymar10/planesync/internal/domain/configuration"
	"github.com/hrubymar10/planesync/internal/domain/mirror"
	"github.com/hrubymar10/planesync/internal/domain/statusmap"
	configloader "github.com/hrubymar10/planesync/internal/infrastructure/config"
	"github.com/hrubymar10/planesync/internal/infrastructure/jira"
	"github.com/hrubymar10/planesync/internal/infrastructure/linkstore"
	"github.com/hrubymar10/planesync/internal/infrastructure/plane"
	"github.com/hrubymar10/planesync/internal/ui/cli"
)

// Build loads configPath and composes one project runner per mapping.
func Build(configPath string) ([]cli.Project, error) {
	config, err := configloader.Load(configPath)
	if err != nil {
		return nil, err
	}
	links := linksAdapter{store: linkstore.New(filepath.Join(filepath.Dir(configPath), "planesync-links.json"))}
	projects := make([]cli.Project, 0, len(config.Projects))
	for _, project := range config.Projects {
		service, err := buildService(config, project, links)
		if err != nil {
			return nil, fmt.Errorf("build %s to %s sync: %w", project.PlaneProject, project.JiraProject, err)
		}
		projects = append(projects, cli.Project{
			Name: project.PlaneProject + " -> " + project.JiraProject,
			Run:  service.Run,
		})
	}
	return projects, nil
}

func buildService(config configuration.Config, project configuration.Project, links linksAdapter) (*appsync.Service, error) {
	planeClient, err := plane.New(config.Plane.BaseURL, config.Plane.Workspace, project.PlaneProject, project.PlaneToken)
	if err != nil {
		return nil, err
	}
	jiraClient, err := jira.New(config.Jira.BaseURL, config.Jira.CloudID, config.Jira.Email, config.Jira.AuthType, project.JiraToken)
	if err != nil {
		return nil, err
	}
	var assigneeAccountID string
	if config.Defaults.ShouldAssignAllToMe() {
		assigneeAccountID, err = jiraClient.CurrentUserAccountID(context.Background())
		if err != nil {
			return nil, fmt.Errorf("resolve Jira assignee: %w", err)
		}
	}
	settings := appsync.Settings{
		TitlePrefix:   project.TitlePrefix,
		TargetProject: project.JiraProject, TargetIssueType: project.JiraIssueType,
		BodyFormat: config.Defaults.BodyFormat, DeletedStatus: config.Defaults.DeletedStatus,
		DeletedResolution: config.Defaults.DeletedResolution,
		ContentSalt:       contentSalt(config.Defaults, project, assigneeAccountID),
		Throttle:          time.Duration(config.Defaults.ThrottleMS) * time.Millisecond,
	}
	return appsync.New(
		sourceAdapter{client: planeClient},
		targetAdapter{
			client: jiraClient, assigneeAccountID: assigneeAccountID,
			parentEpicKey: project.EpicKey, components: append([]string(nil), project.Components...),
			priority: project.Priority,
		},
		links,
		statusmap.New(project.StatusMap, project.StatusGroupMap, project.ResolutionMap),
		settings,
	), nil
}

type sourceAdapter struct {
	client *plane.Client
}

func (a sourceAdapter) ChangedSince(ctx context.Context, since time.Time) ([]appsync.Item, error) {
	items, err := a.client.ChangedSince(ctx, since)
	if err != nil {
		return nil, err
	}
	return sourceItems(items), nil
}

func (a sourceAdapter) List(ctx context.Context) ([]appsync.Item, error) {
	items, err := a.client.List(ctx)
	if err != nil {
		return nil, err
	}
	return sourceItems(items), nil
}

func sourceItems(items []plane.Item) []appsync.Item {
	result := make([]appsync.Item, 0, len(items))
	for _, item := range items {
		result = append(result, appsync.Item{
			ID: item.ID, Identifier: item.Identifier, Title: item.Title, BodyHTML: item.BodyHTML,
			StateName: item.StateName, StateGroup: item.StateGroup, UpdatedAt: item.UpdatedAt,
		})
	}
	return result
}

type targetAdapter struct {
	client            jiraTargetClient
	assigneeAccountID string
	parentEpicKey     string
	components        []string
	priority          string
}

type jiraTargetClient interface {
	Create(context.Context, jira.CreateInput) (string, error)
	Update(context.Context, string, jira.UpdateInput) error
	SetStatus(context.Context, string, string, string) error
}

func (a targetAdapter) Create(ctx context.Context, spec appsync.CreateSpec) (string, error) {
	return a.client.Create(ctx, jira.CreateInput{
		Project: spec.Project, IssueType: spec.IssueType, Summary: spec.Summary,
		Description:       renderBody(spec.BodyFormat, spec.BodyHTML, spec.Reference),
		AssigneeAccountID: a.assigneeAccountID,
		ParentEpicKey:     a.parentEpicKey,
		Components:        append([]string(nil), a.components...),
		Priority:          a.priority,
	})
}

func (a targetAdapter) Update(ctx context.Context, key string, spec appsync.UpdateSpec) error {
	return a.client.Update(ctx, key, jira.UpdateInput{
		Summary: spec.Summary, Description: renderBody(spec.BodyFormat, spec.BodyHTML, spec.Reference),
		AssigneeAccountID: a.assigneeAccountID,
		ParentEpicKey:     a.parentEpicKey,
		Components:        append([]string(nil), a.components...),
		Priority:          a.priority,
	})
}

func (a targetAdapter) SetStatus(ctx context.Context, key, status, resolution string) error {
	return a.client.SetStatus(ctx, key, status, resolution)
}

func renderBody(format, bodyHTML, reference string) []byte {
	if format == "rich" {
		return jira.HTMLToADF(bodyHTML, reference)
	}
	return jira.TextToADF(mirror.PlainTextBody(bodyHTML, reference))
}

type linksAdapter struct {
	store *linkstore.Store
}

func (a linksAdapter) Load() (map[string]appsync.Entry, error) {
	links, err := a.store.Load()
	if err != nil {
		return nil, err
	}
	result := make(map[string]appsync.Entry, len(links))
	for sourceID, entry := range links {
		result[sourceID] = appsync.Entry{Key: entry.Key, UpdatedAt: entry.UpdatedAt, ConfigSalt: entry.ConfigSalt}
	}
	return result, nil
}

func (a linksAdapter) Save(links map[string]appsync.Entry) error {
	persisted := make(linkstore.Map, len(links))
	for sourceID, entry := range links {
		persisted[sourceID] = linkstore.Entry{Key: entry.Key, UpdatedAt: entry.UpdatedAt, ConfigSalt: entry.ConfigSalt}
	}
	return a.store.Save(persisted)
}

func contentSalt(defaults configuration.Defaults, project configuration.Project, assigneeAccountID string) string {
	hash := sha256.New()
	writePart := func(name, value string) {
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	writeMap := func(name string, values map[string]string) {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			writePart(name+".key", key)
			writePart(name+".value", values[key])
		}
	}

	writePart("title_prefix", project.TitlePrefix)
	writeMap("status_map", project.StatusMap)
	writeMap("status_group_map", project.StatusGroupMap)
	writeMap("resolution_map", project.ResolutionMap)
	writePart("priority", project.Priority)
	components := append([]string(nil), project.Components...)
	sort.Strings(components)
	for _, component := range components {
		writePart("component", component)
	}
	writePart("epic_key", project.EpicKey)
	writePart("assignee_account_id", assigneeAccountID)
	writePart("body_format", defaults.BodyFormat)
	writePart("deleted_status", defaults.DeletedStatus)
	writePart("deleted_resolution", defaults.DeletedResolution)
	return hex.EncodeToString(hash.Sum(nil))
}
