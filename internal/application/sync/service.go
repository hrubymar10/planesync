// Package sync coordinates one-way issue synchronization.
package sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/hrubymar10/planesync/internal/domain/linkmap"
	"github.com/hrubymar10/planesync/internal/domain/mirror"
)

// Item is the source work-item data required by synchronization.
type Item struct {
	ID         string
	Title      string
	BodyHTML   string
	StateName  string
	StateGroup string
	UpdatedAt  time.Time
}

// Source lists source work items.
type Source interface {
	ChangedSince(context.Context, time.Time) ([]Item, error)
	List(context.Context) ([]Item, error)
}

// CreateSpec contains target fields for a new issue.
type CreateSpec struct {
	Project    string
	IssueType  string
	Summary    string
	BodyHTML   string
	BackLink   string
	BodyFormat string
	Labels     []string
}

// UpdateSpec contains target fields for an existing issue.
type UpdateSpec struct {
	Summary    string
	BodyHTML   string
	BackLink   string
	BodyFormat string
}

// Target finds and mutates target issues.
type Target interface {
	FindByLabel(context.Context, string) (key string, found bool, err error)
	Create(context.Context, CreateSpec) (key string, err error)
	Update(context.Context, string, UpdateSpec) error
	SetStatus(context.Context, string, string, string) error
}

// Links persists source-to-target issue keys.
type Links interface {
	Load() (map[string]string, error)
	Save(map[string]string) error
}

// StatusResolver maps source states to target statuses.
type StatusResolver interface {
	Resolve(stateName, stateGroup string) (status, resolution string, ok bool)
}

// Settings configures one project synchronization service.
type Settings struct {
	TitlePrefix       string
	AppBaseURL        string
	Workspace         string
	ProjectID         string
	TargetProject     string
	TargetIssueType   string
	BodyFormat        string
	DeletedStatus     string
	DeletedResolution string
}

// Mode selects the source items and reconciliation behavior for a run.
type Mode int

const (
	// Incremental reads items changed at or after Options.Since.
	Incremental Mode = iota
	// Full reads the full source set and reconciles removed items.
	Full
	// Reconcile reads the full source set and reconciles removed items.
	Reconcile
)

// Options controls one synchronization run.
type Options struct {
	Mode   Mode
	Since  time.Time
	DryRun bool
	Limit  int
}

// ActionKind identifies a planned synchronization action.
type ActionKind string

const (
	ActionCreate     ActionKind = "create"
	ActionUpdate     ActionKind = "update"
	ActionSetStatus  ActionKind = "set-status"
	ActionDelete     ActionKind = "delete"
	ActionSkipStatus ActionKind = "skip-status"
	ActionSkipDelete ActionKind = "skip-delete"
)

// Action describes an intended dry-run action or a skipped-action warning.
type Action struct {
	Kind     ActionKind
	SourceID string
	Key      string
	Detail   string
}

// Report summarizes a synchronization run.
type Report struct {
	Created   int
	Updated   int
	StatusSet int
	Deleted   int
	Skipped   int
	Actions   []Action
}

// Service coordinates source, target, link-map, and status ports.
type Service struct {
	source   Source
	target   Target
	links    Links
	resolver StatusResolver
	settings Settings
}

// New creates a synchronization service.
func New(source Source, target Target, links Links, resolver StatusResolver, settings Settings) *Service {
	return &Service{source: source, target: target, links: links, resolver: resolver, settings: settings}
}

// Run synchronizes one configured project.
func (s *Service) Run(ctx context.Context, options Options) (Report, error) {
	var report Report
	links, err := s.links.Load()
	if err != nil {
		return report, fmt.Errorf("load links: %w", err)
	}
	if links == nil {
		links = make(map[string]string)
	}

	var items []Item
	switch options.Mode {
	case Incremental:
		items, err = s.source.ChangedSince(ctx, options.Since)
	case Full, Reconcile:
		items, err = s.source.List(ctx)
	default:
		return report, fmt.Errorf("unsupported sync mode %d", options.Mode)
	}
	if err != nil {
		return report, fmt.Errorf("list source items: %w", err)
	}
	if options.Limit > 0 {
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		if len(items) > options.Limit {
			items = items[:options.Limit]
		}
	}

	present := make(map[string]struct{}, len(items))
	for _, item := range items {
		present[item.ID] = struct{}{}
		if err := s.syncItem(ctx, item, links, options.DryRun, &report); err != nil {
			return report, err
		}
	}
	if options.Limit == 0 && (options.Mode == Full || options.Mode == Reconcile) {
		if err := s.reconcileDeleted(ctx, present, links, options.DryRun, &report); err != nil {
			return report, err
		}
	}
	if !options.DryRun {
		if err := s.links.Save(links); err != nil {
			return report, fmt.Errorf("save links: %w", err)
		}
	}
	return report, nil
}

func (s *Service) syncItem(ctx context.Context, item Item, links map[string]string, dryRun bool, report *Report) error {
	mappedKey, mapped := links[item.ID]
	label := mirror.Label(item.ID)
	var labelKey string
	var labeled bool
	if !mapped {
		var err error
		labelKey, labeled, err = s.target.FindByLabel(ctx, label)
		if err != nil {
			return fmt.Errorf("find target for source item %q: %w", item.ID, err)
		}
	}
	key, resolution := linkmap.Decide(item.ID, linkmap.Hit{Key: mappedKey, OK: mapped}, linkmap.Hit{Key: labelKey, OK: labeled})
	summary := mirror.Summary(s.settings.TitlePrefix, item.Title)
	backLink := mirror.BackLink(s.settings.AppBaseURL, s.settings.Workspace, s.settings.ProjectID, item.ID)

	switch resolution {
	case linkmap.Create:
		report.Created++
		report.Actions = appendDryRun(report.Actions, dryRun, Action{Kind: ActionCreate, SourceID: item.ID, Detail: summary})
		if !dryRun {
			createdKey, err := s.target.Create(ctx, CreateSpec{
				Project: s.settings.TargetProject, IssueType: s.settings.TargetIssueType,
				Summary: summary, BodyHTML: item.BodyHTML, BackLink: backLink,
				BodyFormat: s.settings.BodyFormat, Labels: []string{label},
			})
			if err != nil {
				return fmt.Errorf("create target for source item %q: %w", item.ID, err)
			}
			key = createdKey
			links[item.ID] = key
		}
	case linkmap.Mapped, linkmap.Labeled:
		report.Updated++
		report.Actions = appendDryRun(report.Actions, dryRun, Action{Kind: ActionUpdate, SourceID: item.ID, Key: key, Detail: summary})
		if !dryRun {
			if err := s.target.Update(ctx, key, UpdateSpec{
				Summary: summary, BodyHTML: item.BodyHTML, BackLink: backLink, BodyFormat: s.settings.BodyFormat,
			}); err != nil {
				return fmt.Errorf("update target %q for source item %q: %w", key, item.ID, err)
			}
			if resolution == linkmap.Labeled {
				links[item.ID] = key
			}
		}
	}

	status, statusResolution, ok := s.resolver.Resolve(item.StateName, item.StateGroup)
	if !ok {
		report.Skipped++
		report.Actions = append(report.Actions, Action{Kind: ActionSkipStatus, SourceID: item.ID, Key: key, Detail: item.StateName})
		return nil
	}
	report.StatusSet++
	report.Actions = appendDryRun(report.Actions, dryRun, Action{Kind: ActionSetStatus, SourceID: item.ID, Key: key, Detail: status})
	if !dryRun {
		if err := s.target.SetStatus(ctx, key, status, statusResolution); err != nil {
			return fmt.Errorf("set target %q status for source item %q: %w", key, item.ID, err)
		}
	}
	return nil
}

func (s *Service) reconcileDeleted(ctx context.Context, present map[string]struct{}, links map[string]string, dryRun bool, report *Report) error {
	var missing []string
	for sourceID := range links {
		if _, exists := present[sourceID]; !exists {
			missing = append(missing, sourceID)
		}
	}
	sort.Strings(missing)
	for _, sourceID := range missing {
		key := links[sourceID]
		if s.settings.DeletedStatus == "" {
			report.Skipped++
			report.Actions = append(report.Actions, Action{Kind: ActionSkipDelete, SourceID: sourceID, Key: key})
			continue
		}
		report.Deleted++
		report.Actions = appendDryRun(report.Actions, dryRun, Action{Kind: ActionDelete, SourceID: sourceID, Key: key, Detail: s.settings.DeletedStatus})
		if !dryRun {
			if err := s.target.SetStatus(ctx, key, s.settings.DeletedStatus, s.settings.DeletedResolution); err != nil {
				return fmt.Errorf("mark target %q deleted for missing source item %q: %w", key, sourceID, err)
			}
			delete(links, sourceID)
		}
	}
	return nil
}

func appendDryRun(actions []Action, dryRun bool, action Action) []Action {
	if dryRun {
		return append(actions, action)
	}
	return actions
}
