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

// Entry records a target issue key and its last successful synchronization markers.
type Entry struct {
	Key        string
	UpdatedAt  string
	ConfigSalt string
}

// Links persists source-to-target issue links.
type Links interface {
	Load() (map[string]Entry, error)
	Save(map[string]Entry) error
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
	ContentSalt       string
	Throttle          time.Duration
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
	// OnItem receives each outcome synchronously as its item finishes.
	OnItem func(Action)
}

// ActionKind identifies a reported per-item outcome.
type ActionKind string

const (
	ActionCreated    ActionKind = "created"
	ActionUpdated    ActionKind = "updated"
	ActionUnchanged  ActionKind = "unchanged"
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
	Unchanged int
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
	wait     func(context.Context, time.Duration) error
}

// New creates a synchronization service.
func New(source Source, target Target, links Links, resolver StatusResolver, settings Settings) *Service {
	return &Service{source: source, target: target, links: links, resolver: resolver, settings: settings, wait: waitContext}
}

// Run synchronizes one configured project.
func (s *Service) Run(ctx context.Context, options Options) (Report, error) {
	var report Report
	links, err := s.links.Load()
	if err != nil {
		return report, fmt.Errorf("load links: %w", err)
	}
	if links == nil {
		links = make(map[string]Entry)
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
	}
	var missing []string
	if options.Limit == 0 && (options.Mode == Full || options.Mode == Reconcile) {
		missing = missingSourceIDs(present, links)
	}
	remaining := len(items) + len(missing)
	emit := func(action Action) error {
		report.Actions = append(report.Actions, action)
		if options.OnItem != nil {
			options.OnItem(action)
		}
		remaining--
		if options.DryRun || remaining == 0 || s.settings.Throttle <= 0 {
			return nil
		}
		wait := s.wait
		if wait == nil {
			wait = waitContext
		}
		if err := wait(ctx, s.settings.Throttle); err != nil {
			return fmt.Errorf("throttle between items: %w", err)
		}
		return nil
	}

	linksChanged := false
	for _, item := range items {
		changed, err := s.syncItem(ctx, item, links, options.DryRun, emit, &report)
		if err != nil {
			return report, err
		}
		linksChanged = linksChanged || changed
	}
	if len(missing) > 0 {
		changed, err := s.reconcileDeleted(ctx, missing, links, options.DryRun, emit, &report)
		if err != nil {
			return report, err
		}
		linksChanged = linksChanged || changed
	}
	if !options.DryRun && linksChanged {
		if err := s.links.Save(links); err != nil {
			return report, fmt.Errorf("save links: %w", err)
		}
	}
	return report, nil
}

func (s *Service) syncItem(ctx context.Context, item Item, links map[string]Entry, dryRun bool, emit func(Action) error, report *Report) (bool, error) {
	link, mapped := links[item.ID]
	key, resolution := linkmap.Decide(linkmap.Hit{Key: link.Key, OK: mapped})
	summary := mirror.Summary(s.settings.TitlePrefix, item.Title)
	action := Action{SourceID: item.ID, Reference: item.Identifier}
	status, statusResolution, statusMapped := s.resolver.Resolve(item.StateName, item.StateGroup)
	updatedAt := item.UpdatedAt.UTC().Format(time.RFC3339Nano)

	if mapped && link.UpdatedAt == updatedAt && link.ConfigSalt == s.settings.ContentSalt {
		report.Unchanged++
		action.Kind = ActionUnchanged
		action.Key = key
		if !statusMapped {
			report.Skipped++
			action.Detail = "skipped status: " + item.StateName
		}
		return false, emit(action)
	}

	if resolution == linkmap.Create {
		report.Created++
		action.Kind = ActionCreated
		if !dryRun {
			createdKey, err := s.target.Create(ctx, CreateSpec{
				Project: s.settings.TargetProject, IssueType: s.settings.TargetIssueType,
				Summary: summary, BodyHTML: item.BodyHTML, Reference: item.Identifier,
				BodyFormat: s.settings.BodyFormat,
			})
			if err != nil {
				return false, fmt.Errorf("create target for source item %q: %w", item.ID, err)
			}
			key = createdKey
			action.Key = key
			links[item.ID] = Entry{Key: key}
			if err := s.links.Save(links); err != nil {
				return false, fmt.Errorf("save created link for source item %q: %w", item.ID, err)
			}
		}
	} else {
		report.Updated++
		action.Kind = ActionUpdated
		action.Key = key
		if !dryRun {
			if err := s.target.Update(ctx, key, UpdateSpec{
				Summary: summary, BodyHTML: item.BodyHTML, Reference: item.Identifier, BodyFormat: s.settings.BodyFormat,
			}); err != nil {
				return false, fmt.Errorf("update target %q for source item %q: %w", key, item.ID, err)
			}
		}
	}

	if !statusMapped {
		report.Skipped++
		action.Detail = "skipped status: " + item.StateName
		if !dryRun {
			links[item.ID] = Entry{Key: key, UpdatedAt: updatedAt, ConfigSalt: s.settings.ContentSalt}
		}
		return !dryRun, emit(action)
	}
	report.StatusSet++
	if !dryRun {
		if err := s.target.SetStatus(ctx, key, status, statusResolution); err != nil {
			return false, fmt.Errorf("set target %q status for source item %q: %w", key, item.ID, err)
		}
		links[item.ID] = Entry{Key: key, UpdatedAt: updatedAt, ConfigSalt: s.settings.ContentSalt}
	}
	return !dryRun, emit(action)
}

func missingSourceIDs(present map[string]struct{}, links map[string]Entry) []string {
	var missing []string
	for sourceID := range links {
		if _, exists := present[sourceID]; !exists {
			missing = append(missing, sourceID)
		}
	}
	sort.Strings(missing)
	return missing
}

func (s *Service) reconcileDeleted(ctx context.Context, missing []string, links map[string]Entry, dryRun bool, emit func(Action) error, report *Report) (bool, error) {
	changed := false
	for _, sourceID := range missing {
		key := links[sourceID].Key
		if s.settings.DeletedStatus == "" {
			report.Skipped++
			if err := emit(Action{Kind: ActionSkipDelete, SourceID: sourceID, Reference: sourceID, Key: key}); err != nil {
				return changed, err
			}
			continue
		}
		report.Deleted++
		if !dryRun {
			if err := s.target.SetStatus(ctx, key, s.settings.DeletedStatus, s.settings.DeletedResolution); err != nil {
				return changed, fmt.Errorf("mark target %q deleted for missing source item %q: %w", key, sourceID, err)
			}
			delete(links, sourceID)
			changed = true
		}
		if err := emit(Action{Kind: ActionDeleted, SourceID: sourceID, Reference: sourceID, Key: key}); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
