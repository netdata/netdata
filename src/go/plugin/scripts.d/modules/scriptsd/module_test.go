// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	specYAML, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		t.Fatalf("decode template: %v", err)
	}
	if err := specYAML.Validate(); err != nil {
		t.Fatalf("validate template: %v", err)
	}
	if _, err := chartengine.Compile(specYAML, 1); err != nil {
		t.Fatalf("compile template: %v", err)
	}
}

func TestCollector_InitCollectCleanup(t *testing.T) {
	reg := newFakeRegistry()
	coll := NewWithRegistry(reg)
	coll.Config.JobConfig = spec.JobConfig{
		Name:   "check_disk",
		Plugin: "/bin/true",
	}

	if err := coll.Init(context.Background()); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if len(reg.ensured) != 1 {
		t.Fatalf("expected one ensure call, got %d", len(reg.ensured))
	}
	if len(reg.attached) != 1 {
		t.Fatalf("expected one attach call, got %d", len(reg.attached))
	}

	reg.snapshots["default"] = runtime.SchedulerSnapshot{
		Scheduler: "default",
		Running:   1,
		Queued:    2,
		Scheduled: 3,
		Started:   4,
		Finished:  5,
		Skipped:   6,
		Jobs: []runtime.JobMetricsSnapshot{
			{
				JobID:      "",
				JobName:    "check_disk",
				State:      "OK",
				Attempt:    1,
				MaxAttempt: 3,
				PerfSamples: []output.PerfDatum{
					{Label: "used", Unit: "KB", Value: 30},
				},
			},
		},
	}

	cc := mustCycleController(t, coll.MetricStore())
	cc.BeginCycle()
	if err := coll.Collect(context.Background()); err != nil {
		cc.AbortCycle()
		t.Fatalf("collect failed: %v", err)
	}
	cc.CommitCycleSuccess()

	read := coll.MetricStore().Read(metrix.ReadRaw())
	assertMetricValue(t, read, "scriptsd.scheduler.running", metrix.Labels{"nagios_scheduler": "default"}, 1)
	assertMetricValue(t, read, "scriptsd.job.state.ok", metrix.Labels{"nagios_scheduler": "default", "nagios_job": "check_disk"}, 1)
	assertMetricValue(t, read, "scriptsd.perf_bytes_used_value", metrix.Labels{"nagios_scheduler": "default", "nagios_job": "check_disk"}, 30000)
	meta, ok := read.MetricMeta("scriptsd.perf_bytes_used_value")
	if !ok {
		t.Fatalf("expected metric metadata for scriptsd.perf_bytes_used_value")
	}
	if meta.Unit != "bytes" {
		t.Fatalf("unexpected unit metadata: %q", meta.Unit)
	}
	if !meta.Float {
		t.Fatalf("expected float metadata to be true")
	}

	coll.Cleanup(context.Background())
	if reg.detached != 1 {
		t.Fatalf("expected one detach call, got %d", reg.detached)
	}
	if len(reg.removed) != 1 || reg.removed[0] != "default" {
		t.Fatalf("unexpected remove calls: %v", reg.removed)
	}
}

type fakeRegistry struct {
	ensured  []schedulers.Definition
	attached []runtime.JobRegistration
	removed  []string
	detached int

	snapshots map[string]runtime.SchedulerSnapshot
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		snapshots: make(map[string]runtime.SchedulerSnapshot),
	}
}

func (f *fakeRegistry) Ensure(def schedulers.Definition, _ *logger.Logger) error {
	f.ensured = append(f.ensured, def)
	return nil
}

func (f *fakeRegistry) Remove(name string) error {
	f.removed = append(f.removed, name)
	return nil
}

func (f *fakeRegistry) Attach(_ string, reg runtime.JobRegistration, _ *logger.Logger) (*schedulers.SchedulerJobHandle, error) {
	f.attached = append(f.attached, reg)
	return &schedulers.SchedulerJobHandle{}, nil
}

func (f *fakeRegistry) Detach(_ *schedulers.SchedulerJobHandle) {
	f.detached++
}

func (f *fakeRegistry) Collect(string) map[string]int64 {
	return nil
}

func (f *fakeRegistry) Snapshot(name string) (runtime.SchedulerSnapshot, bool) {
	s, ok := f.snapshots[name]
	return s, ok
}

func (f *fakeRegistry) Get(name string) (schedulers.Definition, bool) {
	for _, def := range f.ensured {
		if def.Name == name {
			return def, true
		}
	}
	return schedulers.Definition{}, false
}

func (f *fakeRegistry) All() []schedulers.Definition {
	out := make([]schedulers.Definition, len(f.ensured))
	copy(out, f.ensured)
	return out
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatalf("store does not expose cycle control")
	}
	return managed.CycleController()
}

func assertMetricValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	if !ok {
		t.Fatalf("missing metric %s labels=%v", name, labels)
	}
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("metric mismatch %s labels=%v got=%f want=%f", name, labels, got, want)
	}
}

func TestCollector_CheckValidatesConfig(t *testing.T) {
	coll := NewWithRegistry(newFakeRegistry())
	coll.Config.JobConfig = spec.JobConfig{Name: "invalid-without-plugin"}
	if err := coll.Check(context.Background()); err == nil {
		t.Fatalf("expected check to fail for missing plugin")
	}

	coll.Config.JobConfig.Plugin = "/bin/true"
	if err := coll.Check(context.Background()); err != nil {
		t.Fatalf("expected check to pass, got %v", err)
	}
}

func TestCollector_DefaultTimingDefaultsFromSpec(t *testing.T) {
	coll := NewWithRegistry(newFakeRegistry())
	coll.Config.JobConfig = spec.JobConfig{
		Name:   "defaults",
		Plugin: "/bin/true",
	}
	if err := coll.Init(context.Background()); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if coll.jobSpec.CheckInterval != 5*time.Minute {
		t.Fatalf("unexpected check interval default: %s", coll.jobSpec.CheckInterval)
	}
	if coll.jobSpec.RetryInterval != time.Minute {
		t.Fatalf("unexpected retry interval default: %s", coll.jobSpec.RetryInterval)
	}
}
