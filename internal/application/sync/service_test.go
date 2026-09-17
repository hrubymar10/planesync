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
	links := &fakeLinks{links: map[string]string{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental, Since: item.UpdatedAt.Add(-time.Hour)})
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
	if created.Project != "DST" || created.IssueType != "Task" || created.Summary != "[team] Example title" || created.Labels[0] != "plane-source-item" {
		t.Errorf("create spec = %#v", created)
	}
	if created.Reference != "SRC-16" {
		t.Errorf("reference = %q", created.Reference)
	}
	if links.saved["source-item"] != "created-key" || links.saveCalls != 2 {
		t.Errorf("saved links = %#v, calls = %d", links.saved, links.saveCalls)
	}
}

func TestRunUpdatesMappedItem(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]string{"source-item": "mapped-key"}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Updated != 1 || len(target.updates) != 1 || target.updates[0].key != "mapped-key" || target.updates[0].spec.Reference != "SRC-16" {
		t.Errorf("report/updates = %#v / %#v", report, target.updates)
	}
	if target.findCalls != 0 || len(target.creates) != 0 {
		t.Errorf("unexpected lookup/create calls: find=%d create=%d", target.findCalls, len(target.creates))
	}
	if len(target.statuses) != 1 || target.statuses[0].resolution != "Fixed" {
		t.Errorf("status calls = %#v", target.statuses)
	}
}

func TestRunSecondIncrementalWithNoChangesIsNoOp(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}, {}}}
	target := &fakeTarget{createKey: "created-key"}
	links := &fakeLinks{links: map[string]string{}}
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

func TestRunRecoversLinkByLabel(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{labelKey: "recovered-key", labelFound: true}
	links := &fakeLinks{links: map[string]string{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Updated != 1 || target.findLabel != "plane-source-item" || len(target.updates) != 1 {
		t.Errorf("report/target = %#v / %#v", report, target)
	}
	if links.saved["source-item"] != "recovered-key" || links.saveCalls != 2 {
		t.Errorf("saved links = %#v", links.saved)
	}
}

func TestRunPersistsCreatedLinkBeforeStatusFailure(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{createKey: "created-key", statusErr: errors.New("status unavailable")}
	links := &fakeLinks{links: map[string]string{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	_, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err == nil || !strings.Contains(err.Error(), "status unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
	if links.saveCalls != 1 || links.saved["source-item"] != "created-key" {
		t.Fatalf("durable links after status failure = %#v, calls=%d", links.saved, links.saveCalls)
	}
}

func TestRunPersistsRecoveredLinkBeforeStatusFailure(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{labelKey: "recovered-key", labelFound: true, statusErr: errors.New("status unavailable")}
	links := &fakeLinks{links: map[string]string{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	_, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err == nil || !strings.Contains(err.Error(), "status unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
	if links.saveCalls != 1 || links.saved["source-item"] != "recovered-key" {
		t.Fatalf("durable links after status failure = %#v, calls=%d", links.saved, links.saveCalls)
	}
}

func TestRunSkipsUnmappedStatus(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]string{"source-item": "mapped-key"}}
	service := testService(source, target, links, mapResolver{})

	report, err := service.Run(context.Background(), Options{Mode: Incremental})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if report.Skipped != 1 || report.StatusSet != 0 || len(report.Actions) != 1 || report.Actions[0].Kind != ActionSkipStatus || len(target.statuses) != 0 {
		t.Errorf("report/statuses = %#v / %#v", report, target.statuses)
	}
}

func TestRunFullReconcilesDeletedItem(t *testing.T) {
	source := &fakeSource{listed: []Item{}}
	target := &fakeTarget{}
	links := &fakeLinks{links: map[string]string{"missing-source": "target-key"}}
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
}

func TestRunDryRunPerformsNoWrites(t *testing.T) {
	source := &fakeSource{changed: [][]Item{{testItem()}}}
	target := &fakeTarget{createKey: "must-not-be-used"}
	links := &fakeLinks{links: map[string]string{}}
	service := testService(source, target, links, mapResolver{"Started": "In Progress"})

	report, err := service.Run(context.Background(), Options{Mode: Incremental, DryRun: true})
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if target.findCalls != 1 || len(target.creates) != 0 || len(target.updates) != 0 || len(target.statuses) != 0 || links.saveCalls != 0 {
		t.Errorf("dry-run calls: target=%#v save=%d", target, links.saveCalls)
	}
	if report.Created != 1 || report.StatusSet != 1 || len(report.Actions) != 2 || report.Actions[0].Kind != ActionCreate || report.Actions[1].Kind != ActionSetStatus {
		t.Errorf("dry-run report = %#v", report)
	}
	if len(links.links) != 0 {
		t.Errorf("dry run mutated links: %#v", links.links)
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
	labelKey   string
	labelFound bool
	createKey  string
	findCalls  int
	findLabel  string
	creates    []CreateSpec
	updates    []updateCall
	statuses   []statusCall
	statusErr  error
}

func (f *fakeTarget) FindByLabel(_ context.Context, label string) (string, bool, error) {
	f.findCalls++
	f.findLabel = label
	return f.labelKey, f.labelFound, nil
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
	links     map[string]string
	saved     map[string]string
	saveCalls int
}

func (f *fakeLinks) Load() (map[string]string, error) {
	return f.links, nil
}

func (f *fakeLinks) Save(links map[string]string) error {
	f.saveCalls++
	f.saved = make(map[string]string, len(links))
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
	input := map[string]string{"source": "target"}
	if err := fake.Save(input); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	input["source"] = "changed"
	if reflect.DeepEqual(fake.saved, input) {
		t.Fatal("fake link snapshot unexpectedly aliases input")
	}
}
