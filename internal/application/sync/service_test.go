package sync

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunCreatesItemAndPersistsLink(t *testing.T) {
	item := testItem()
	source := &fakeSource{changed: [][]Item{{item}}}
	target := &fakeTarget{createKey: "created-key"}
	links := &fakeLinks{links: map[string]Entry{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	var streamed []Action
	report, err := service.Run(context.Background(), Options{
		Mode: Incremental, Since: item.UpdatedAt.Add(-time.Hour),
		OnItem: func(action Action) { streamed = append(streamed, action) },
	})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Created != 1 || report.StatusSet != 1 || report.Updated != 0 {
		t.Errorf("report = %#v", report)
	}
	if len(target.creates) != 1 {
		t.Fatalf("create calls = %d", len(target.creates))
	}
	created := target.creates[0]
	if created.Project != "DST" || created.IssueType != "Task" || created.Summary != "[team] Example title" {
		t.Errorf("create spec = %#v", created)
	}
	if created.Reference != "SRC-16" {
		t.Errorf("reference = %q", created.Reference)
	}
	if links.saved["source-item"].Key != "created-key" || links.saved["source-item"].UpdatedAt != item.UpdatedAt.Format(time.RFC3339) || links.saved["source-item"].ConfigSalt != "test-salt" || links.saveCalls != 2 {
		t.Errorf("saved links = %#v, calls = %d", links.saved, links.saveCalls)
	}
	wantAction := Action{Kind: ActionCreated, SourceID: "source-item", Reference: "SRC-16", Key: "created-key"}
	if len(report.Actions) != 1 || report.Actions[0] != wantAction {
		t.Errorf("actions = %#v, want %#v", report.Actions, wantAction)
	}
	if !reflect.DeepEqual(streamed, report.Actions) {
		t.Errorf("streamed actions = %#v, report actions = %#v", streamed, report.Actions)
	}
}

func TestRunUpdatesMappedItem(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{"source-item": {Key: "mapped-key"}}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Updated != 1 || len(target.updates) != 1 || target.updates[0].key != "mapped-key" || target.updates[0].spec.Reference != "SRC-16" {
		t.Errorf("report/updates = %#v / %#v", report, target.updates)
	}
	if len(target.creates) != 0 {
		t.Errorf("unexpected create calls: %d", len(target.creates))
	}
	if len(target.statuses) != 1 || target.statuses[0].resolution != "Fixed" {
		t.Errorf("status calls = %#v", target.statuses)
	}
	wantAction := Action{Kind: ActionUpdated, SourceID: "source-item", Reference: "SRC-16", Key: "mapped-key"}
	if len(report.Actions) != 1 || report.Actions[0] != wantAction {
		t.Errorf("actions = %#v, want %#v", report.Actions, wantAction)
	}
}

func TestRunLegacyEntryResynchronizesOnce(t *testing.T) {
	item := testItem()
	source := &fakeSource{changed: [][]Item{{item}, {item}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{item.ID: {Key: "mapped-key"}}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	first, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("first Run(): %v", err)
	}
	second, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("second Run(): %v", err)
	}
	if first.Updated != 1 || second.Unchanged != 1 {
		t.Errorf("reports = first %#v, second %#v", first, second)
	}
	if len(target.updates) != 1 || len(target.statuses) != 1 || links.saveCalls != 1 {
		t.Errorf("writes = updates=%d statuses=%d link saves=%d", len(target.updates), len(target.statuses), links.saveCalls)
	}
}

func TestRunSkipsUnchangedMappedItem(t *testing.T) {
	item := testItem()
	source := &fakeSource{changed: [][]Item{{item}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{item.ID: {
		Key: "mapped-key", UpdatedAt: item.UpdatedAt.Format(time.RFC3339), ConfigSalt: "test-salt",
	}}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	wantAction := Action{Kind: ActionUnchanged, SourceID: item.ID, Reference: item.Identifier, Key: "mapped-key"}
	if report.Unchanged != 1 || len(report.Actions) != 1 || report.Actions[0] != wantAction {
		t.Errorf("report = %#v, want unchanged action %#v", report, wantAction)
	}
	if len(target.updates) != 0 || len(target.statuses) != 0 || links.saveCalls != 0 {
		t.Errorf("no-op writes: updates=%d statuses=%d link saves=%d", len(target.updates), len(target.statuses), links.saveCalls)
	}
}

func TestRunChangedUpdatedAtOrSaltUpdatesMarkers(t *testing.T) {
	item := testItem()
	for _, test := range []struct {
		name  string
		entry Entry
	}{
		{name: "updated_at", entry: Entry{Key: "mapped-key", UpdatedAt: item.UpdatedAt.Add(-time.Minute).Format(time.RFC3339), ConfigSalt: "test-salt"}},
		{name: "config_salt", entry: Entry{Key: "mapped-key", UpdatedAt: item.UpdatedAt.Format(time.RFC3339), ConfigSalt: "old-salt"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &fakeSource{changed: [][]Item{{item}}}
			target := &fakeTarget{}
			links := &fakeLinks{links: map[string]Entry{item.ID: test.entry}}
			service := testService(source, target, links, mapResolver{"Started": "In Progress"})

			report, err := service.Run(context.Background(), Options{Mode: Incremental})
			if err != nil {
				t.Fatalf("Run(): %v", err)
			}
			if report.Updated != 1 || len(target.updates) != 1 || len(target.statuses) != 1 {
				t.Errorf("report/writes = %#v / %#v / %#v", report, target.updates, target.statuses)
			}
			got := links.saved[item.ID]
			if got.UpdatedAt != item.UpdatedAt.Format(time.RFC3339) || got.ConfigSalt != "test-salt" {
				t.Errorf("saved markers = %#v", got)
			}
		})
	}
}

func TestRunSecondIncrementalWithNoChangesIsNoOp(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}, {}}}
	target := &fakeTarget{createKey: "created-key"}
	links := &fakeLinks{links: map[string]Entry{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	if _, err := service.Run(context.Background(), Options{Mode: Incremental}); err != nil {
		t.Fatalf("first Run(): %v", err)
	}
	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("second Run(): %v", err)
	}
	if !reflect.DeepEqual(report, Report{}) {
		t.Errorf("second report = %#v, want zero report", report)
	}
	if len(target.creates) != 1 || len(target.updates) != 0 || len(target.statuses) != 1 {
		t.Errorf("target writes after no-op run: creates=%d updates=%d statuses=%d", len(target.creates), len(target.updates), len(target.statuses))
	}
}

func TestRunPersistsCreatedLinkBeforeStatusFailure(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{createKey: "created-key", statusErr: errors.New("status unavailable")}
	links := &fakeLinks{links: map[string]Entry{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	_, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err == nil || !strings.Contains(err.Error(), "status unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
	if links.saveCalls != 1 || links.saved["source-item"].Key != "created-key" || links.saved["source-item"].UpdatedAt != "" || links.saved["source-item"].ConfigSalt != "" {
		t.Fatalf("durable links after status failure = %#v, calls=%d", links.saved, links.saveCalls)
	}
}

func TestRunSkipsUnmappedStatus(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{"source-item": {Key: "mapped-key"}}}
	service := testService(source, target, links, mapResolver{})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Skipped != 1 || report.StatusSet != 0 || len(report.Actions) != 1 || report.Actions[0].Kind != ActionUpdated || report.Actions[0].Detail != "skipped status: Started" || len(target.statuses) != 0 {
		t.Errorf("report/statuses = %#v / %#v", report, target.statuses)
	}
}

func TestRunFullReconcilesDeletedItem(t *testing.T) {
	source := &fakeSource{listed: []Item{}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{"missing-source": {Key: "target-key"}}}
	service := testService(source, target, links, mapResolver{})

	report, err := service.Run(context.Background(), Options{Mode: Full})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if source.listCalls != 1 || source.changedCalls != 0 || report.Deleted != 1 {
		t.Errorf("source/report = %#v / %#v", source, report)
	}
	if len(target.statuses) != 1 || target.statuses[0] != (statusCall{key: "target-key", status: "Removed", resolution: "Declined"}) {
		t.Errorf("status calls = %#v", target.statuses)
	}
	if len(links.saved) != 0 {
		t.Errorf("saved links = %#v, want removed mapping", links.saved)
	}
	wantAction := Action{Kind: ActionDeleted, SourceID: "missing-source", Reference: "missing-source", Key: "target-key"}
	if len(report.Actions) != 1 || report.Actions[0] != wantAction {
		t.Errorf("actions = %#v, want %#v", report.Actions, wantAction)
	}
}

func TestRunFullReportsSkippedDelete(t *testing.T) {
	source := &fakeSource{listed: []Item{}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{"missing-source": {Key: "target-key"}}}
	service := testService(source, target, links, mapResolver{})
	service.settings.DeletedStatus = ""

	report, err := service.Run(context.Background(), Options{Mode: Full})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	wantAction := Action{Kind: ActionSkipDelete, SourceID: "missing-source", Reference: "missing-source", Key: "target-key"}
	if report.Skipped != 1 || report.Deleted != 0 || len(report.Actions) != 1 || report.Actions[0] != wantAction {
		t.Errorf("report = %#v, want action %#v", report, wantAction)
	}
	if len(target.statuses) != 0 {
		t.Errorf("status calls = %#v", target.statuses)
	}
}

func TestRunDryRunPerformsNoWrites(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{createKey: "must-not-be-used"}
	links := &fakeLinks{links: map[string]Entry{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental, DryRun: true})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if len(target.creates) != 0 || len(target.updates) != 0 || len(target.statuses) != 0 || links.saveCalls != 0 {
		t.Errorf("dry-run calls: target=%#v save=%d", target, links.saveCalls)
	}
	if report.Created != 1 || report.StatusSet != 1 || len(report.Actions) != 1 || report.Actions[0] != (Action{Kind: ActionCreated, SourceID: "source-item", Reference: "SRC-16"}) {
		t.Errorf("dry-run report = %#v", report)
	}
	if len(links.links) != 0 {
		t.Errorf("dry run mutated links: %#v", links.links)
	}
}

func TestRunThrottlesBetweenRealItemsOnly(t *testing.T) {
	first := testItem()
	second := testItem()
	second.ID = "source-item-two"
	second.Identifier = "SRC-17"
	source := &fakeSource{changed: [][]Item{{first, second}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{first.ID: {Key: "key-one"}, second.ID: {Key: "key-two"}}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})
	service.settings.Throttle = 25 * time.Millisecond
	var waits []time.Duration
	service.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	if _, err := service.Run(context.Background(), Options{Mode: Incremental}); err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if !reflect.DeepEqual(waits, []time.Duration{25 * time.Millisecond}) {
		t.Errorf("waits = %v, want one inter-item wait", waits)
	}

	source.changed = [][]Item{{first, second}}
	source.changedCalls = 0
	waits = nil
	if _, err := service.Run(context.Background(), Options{Mode: Incremental, DryRun: true}); err != nil {
		t.Fatalf("dry-run Run(): %v", err)
	}
	if len(waits) != 0 {
		t.Errorf("dry-run waits = %v, want none", waits)
	}

	fullSource := &fakeSource{listed: []Item{first}}
	fullLinks := &fakeLinks{links: map[string]Entry{first.ID: {Key: "key-one"}, "missing-source": {Key: "missing-key"}}}
	fullService := testService(fullSource, &fakeTarget{}, fullLinks, mapResolver{"Started": "In Progress"})
	fullService.settings.Throttle = 25 * time.Millisecond
	waits = nil
	fullService.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	fullReport, err := fullService.Run(context.Background(), Options{Mode: Full})
	if err != nil {
		t.Fatalf("full Run(): %v", err)
	}
	if len(fullReport.Actions) != 2 || !reflect.DeepEqual(waits, []time.Duration{25 * time.Millisecond}) {
		t.Errorf("full actions/waits = %#v / %v", fullReport.Actions, waits)
	}
}

func TestRunThrottleHonorsContextCancellation(t *testing.T) {
	first := testItem()
	second := testItem()
	second.ID = "source-item-two"
	second.Identifier = "SRC-17"
	source := &fakeSource{changed: [][]Item{{first, second}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]Entry{first.ID: {Key: "key-one"}, second.ID: {Key: "key-two"}}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})
	service.settings.Throttle = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	service.wait = func(ctx context.Context, delay time.Duration) error {
		cancel()
		return waitContext(ctx, delay)
	}

	report, err := service.Run(ctx, Options{Mode: Incremental})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
	if len(report.Actions) != 1 || len(target.updates) != 1 {
		t.Errorf("partial report/updates = %#v / %#v", report, target.updates)
	}
}

func testItem() Item {
	return Item{
		ID: "source-item", Identifier: "SRC-16", Title: "Example title", BodyHTML: "<p>Body</p>",
		StateName: "Started", StateGroup: "started", UpdatedAt: time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC),
	}
}

func testService(source Source, target Target, links Links, resolver StatusResolver) *Service {
	return New(source, target, links, resolver, Settings{
		TitlePrefix: "[team]", TargetProject: "DST", TargetIssueType: "Task",
		BodyFormat: "rich", DeletedStatus: "Removed", DeletedResolution: "Declined",
		ContentSalt: "test-salt",
	})
}

type fakeSource struct {
	changed      [][]Item
	listed       []Item
	changedCalls int
	listCalls    int
}

func (f *fakeSource) ChangedSince(context.Context, time.Time) ([]Item, error) {
	index := f.changedCalls
	f.changedCalls++
	if index >= len(f.changed) {
		return nil, nil
	}
	return f.changed[index], nil
}

func (f *fakeSource) List(context.Context) ([]Item, error) {
	f.listCalls++
	return f.listed, nil
}

type updateCall struct {
	key  string
	spec UpdateSpec
}

type statusCall struct {
	key, status, resolution string
}

type fakeTarget struct {
	createKey string
	creates   []CreateSpec
	updates   []updateCall
	statuses  []statusCall
	statusErr error
}

func (f *fakeTarget) Create(_ context.Context, spec CreateSpec) (string, error) {
	f.creates = append(f.creates, spec)
	return f.createKey, nil
}

func (f *fakeTarget) Update(_ context.Context, key string, spec UpdateSpec) error {
	f.updates = append(f.updates, updateCall{key: key, spec: spec})
	return nil
}

func (f *fakeTarget) SetStatus(_ context.Context, key, status, resolution string) error {
	f.statuses = append(f.statuses, statusCall{key: key, status: status, resolution: resolution})
	return f.statusErr
}

type fakeLinks struct {
	links     map[string]Entry
	saved     map[string]Entry
	saveCalls int
}

func (f *fakeLinks) Load() (map[string]Entry, error) {
	return f.links, nil
}

func (f *fakeLinks) Save(links map[string]Entry) error {
	f.saveCalls++
	f.saved = make(map[string]Entry, len(links))
	for key, value := range links {
		f.saved[key] = value
	}
	return nil
}

type mapResolver map[string]string

func (r mapResolver) Resolve(stateName, _ string) (string, string, bool) {
	status, ok := r[stateName]
	resolution := ""
	if stateName == "Started" {
		resolution = "Fixed"
	}
	return status, resolution, ok
}

func TestFakeLinksCopiesOnSave(t *testing.T) {
	fake := &fakeLinks{}
	input := map[string]Entry{"source": {Key: "target"}}
	if err := fake.Save(input); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	input["source"] = Entry{Key: "changed"}
	if reflect.DeepEqual(fake.saved, input) {
		t.Fatal("fake link snapshot unexpectedly aliases input")
	}
}
