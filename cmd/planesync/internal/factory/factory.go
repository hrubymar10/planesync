package factory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
	appBaseURL := config.Plane.AppBaseURL
	if strings.TrimSpace(appBaseURL) == "" {
		appBaseURL = config.Plane.BaseURL
	}
	settings := appsync.Settings{
		TitlePrefix: project.TitlePrefix, AppBaseURL: appBaseURL,
		Workspace: config.Plane.Workspace, ProjectID: project.PlaneProject,
		TargetProject: project.JiraProject, TargetIssueType: project.JiraIssueType,
		BodyFormat: config.Defaults.BodyFormat, DeletedStatus: config.Defaults.DeletedStatus,
	}
	return appsync.New(
		sourceAdapter{client: planeClient},
		targetAdapter{client: jiraClient, assigneeAccountID: assigneeAccountID},
		links,
		statusmap.New(project.StatusMap, project.StatusGroupMap),
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
			ID: item.ID, Title: item.Title, BodyHTML: item.BodyHTML,
			StateName: item.StateName, StateGroup: item.StateGroup, UpdatedAt: item.UpdatedAt,
		})
	}
	return result
}

type targetAdapter struct {
	client            *jira.Client
	assigneeAccountID string
}

func (a targetAdapter) FindByLabel(ctx context.Context, label string) (string, bool, error) {
	return a.client.FindByLabel(ctx, label)
}

func (a targetAdapter) Create(ctx context.Context, spec appsync.CreateSpec) (string, error) {
	return a.client.Create(ctx, jira.CreateInput{
		Project: spec.Project, IssueType: spec.IssueType, Summary: spec.Summary,
		Description: renderBody(spec.BodyFormat, spec.BodyHTML, spec.BackLink), Labels: spec.Labels,
		AssigneeAccountID: a.assigneeAccountID,
	})
}

func (a targetAdapter) Update(ctx context.Context, key string, spec appsync.UpdateSpec) error {
	return a.client.Update(ctx, key, jira.UpdateInput{
		Summary: spec.Summary, Description: renderBody(spec.BodyFormat, spec.BodyHTML, spec.BackLink),
		AssigneeAccountID: a.assigneeAccountID,
	})
}

func (a targetAdapter) SetStatus(ctx context.Context, key, status string) error {
	return a.client.SetStatus(ctx, key, status)
}

func renderBody(format, bodyHTML, backLink string) []byte {
	if format == "rich" {
		return jira.HTMLToADF(bodyHTML, backLink)
	}
	return jira.TextToADF(mirror.PlainTextBody(bodyHTML, backLink))
}

type linksAdapter struct {
	store *linkstore.Store
}

func (a linksAdapter) Load() (map[string]string, error) {
	links, err := a.store.Load()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(links))
	for sourceID, targetKey := range links {
		result[sourceID] = targetKey
	}
	return result, nil
}

func (a linksAdapter) Save(links map[string]string) error {
	persisted := make(linkstore.Map, len(links))
	for sourceID, targetKey := range links {
		persisted[sourceID] = targetKey
	}
	return a.store.Save(persisted)
}
