package scheduler

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
)

func TestCollectorInitRegistersDefinition(t *testing.T) {
	c := New()
	c.Name = "test-scheduler"
	if err := c.Init(nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer c.Cleanup(nil)
	if def, ok := schedulers.Get("test-scheduler"); !ok || def.Workers != c.Workers {
		t.Fatalf("scheduler definition not registered: %+v ok=%v", def, ok)
	}
}

func TestCollectorCollectFiltersMetrics(t *testing.T) {
	c := New()
	c.Name = "collect-test"
	if err := c.Init(nil); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	defer c.Cleanup(nil)
	orig := collectSchedulerMetrics
	collectSchedulerMetrics = func(string) map[string]int64 {
		return map[string]int64{
			"collect-test.scheduler.jobs.active": 1,
			"other.scheduler.jobs.active":        2,
		}
	}
	defer func() { collectSchedulerMetrics = orig }()
	metrics := c.Collect(nil)
	if metrics == nil || metrics["collect-test.scheduler.jobs.active"] != 1 {
		t.Fatalf("expected filtered scheduler metrics, got %v", metrics)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected only matching prefix, got %v", metrics)
	}
}
