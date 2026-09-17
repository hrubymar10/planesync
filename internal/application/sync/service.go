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
	Identifier string
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
	Reference  string
	BodyFormat string
}

// UpdateSpec contains target fields for an existing issue.
type UpdateSpec struct {
	Summary    string
	BodyHTML   string
	Reference  string
	BodyFormat string
}

// Target mutates target issues.
type Target interface {
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

// ActionKind identifies a reported per-item outcome.
type ActionKind string

const (
	ActionCreated    ActionKind = "created"
	ActionUpdated    ActionKind = "updated"
	ActionSkipped    ActionKind = "skipped"
	ActionDeleted    ActionKind = "deleted"
	ActionSkipDelete ActionKind = "skip-delete"
)

// Action describes the outcome for one processed source item or missing link.
type Action struct {
	Kind      ActionKind
	SourceID  string
	Reference string
	Key       string
	Detail    string
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
	key, resolution := linkmap.Decide(linkmap.Hit{Key: mappedKey, OK: mapped})
	summary := mirror.Summary(s.settings.TitlePrefix, item.Title)
	action := Action{SourceID: item.ID, Reference: item.Identifier}

	switch resolution {
	case linkmap.Create:
		report.Created++
		action.Kind = ActionCreated
		if !dryRun {
			createdKey, err := s.target.Create(ctx, CreateSpec{
				Project: s.settings.TargetProject, IssueType: s.settings.TargetIssueType,
				Summary: summary, BodyHTML: item.BodyHTML, Reference: item.Identifier,
				BodyFormat: s.settings.BodyFormat,
			})
			if err != nil {
				return fmt.Errorf("create target for source item %q: %w", item.ID, err)
			}
			key = createdKey
			action.Key = key
			links[item.ID] = key
			if err := s.links.Save(links); err != nil {
				return fmt.Errorf("save created link for source item %q: %w", item.ID, err)
			}
		}
	case linkmap.Mapped:
		report.Updated++
		action.Kind = ActionUpdated
		action.Key = key
		if !dryRun {
			if err := s.target.Update(ctx, key, UpdateSpec{
				Summary: summary, BodyHTML: item.BodyHTML, Reference: item.Identifier, BodyFormat: s.settings.BodyFormat,
			}); err != nil {
				return fmt.Errorf("update target %q for source item %q: %w", key, item.ID, err)
			}
		}
	}

	status, statusResolution, ok := s.resolver.Resolve(item.StateName, item.StateGroup)
	if !ok {
		report.Skipped++
		action.Detail = "skipped status: " + item.StateName
		report.Actions = append(report.Actions, action)
		return nil
	}
	report.StatusSet++
	if !dryRun {
		if err := s.target.SetStatus(ctx, key, status, statusResolution); err != nil {
			return fmt.Errorf("set target %q status for source item %q: %w", key, item.ID, err)
		}
	}
	report.Actions = append(report.Actions, action)
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
			report.Actions = append(report.Actions, Action{Kind: ActionSkipDelete, SourceID: sourceID, Reference: sourceID, Key: key})
			continue
		}
		report.Deleted++
		if !dryRun {
			if err := s.target.SetStatus(ctx, key, s.settings.DeletedStatus, s.settings.DeletedResolution); err != nil {
				return fmt.Errorf("mark target %q deleted for missing source item %q: %w", key, sourceID, err)
			}
			delete(links, sourceID)
		}
		report.Actions = append(report.Actions, Action{Kind: ActionDeleted, SourceID: sourceID, Reference: sourceID, Key: key})
	}
	return nil
}
