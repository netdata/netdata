// SPDX-License-Identifier: GPL-3.0-or-later

package metricsaudit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func TestAuditorSeparatesSameJobNameAcrossModules(t *testing.T) {
	da := New()

	da.RegisterJob("shared", "modA", "")
	da.RegisterJob("shared", "modB", "")

	chartsA := testCharts("chart_a", "ctx_a", "dim_a")
	chartsB := testCharts("chart_b", "ctx_b", "dim_b")
	da.RecordJobStructure("shared", "modA", &chartsA)
	da.RecordJobStructure("shared", "modB", &chartsB)

	da.RecordCollection("shared", "modA", map[string]int64{"dim_a": 1})
	da.RecordCollection("shared", "modB", map[string]int64{"dim_b": 2})

	idA := newJobID("modA", "shared")
	idB := newJobID("modB", "shared")

	if got := len(da.jobs); got != 2 {
		t.Fatalf("expected 2 jobs, got %d", got)
	}
	if da.jobs[idA] == nil || da.jobs[idB] == nil {
		t.Fatalf("expected both module-scoped jobs to exist: %+v", da.jobs)
	}
	if da.jobs[idA].CollectionCount != 1 || da.jobs[idB].CollectionCount != 1 {
		t.Fatalf("expected per-module collection counts to be isolated, got %d and %d",
			da.jobs[idA].CollectionCount, da.jobs[idB].CollectionCount)
	}
	if !da.jobs[idA].AllSeenMetrics["dim_a"] {
		t.Fatalf("expected modA metrics to include dim_a")
	}
	if !da.jobs[idB].AllSeenMetrics["dim_b"] {
		t.Fatalf("expected modB metrics to include dim_b")
	}
}

func TestAuditorOnCompleteAfterAllRegisteredJobsCollected(t *testing.T) {
	da := New()
	baseDir := t.TempDir()

	done := make(chan struct{}, 1)
	da.EnableDataCapture(baseDir, func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})

	job1Dir := filepath.Join(baseDir, "mod", "job1")
	job2Dir := filepath.Join(baseDir, "mod", "job2")
	da.RegisterJob("job1", "mod", job1Dir)
	da.RegisterJob("job2", "mod", job2Dir)

	charts1 := testCharts("chart1", "ctx1", "dim1")
	charts2 := testCharts("chart2", "ctx2", "dim2")
	da.RecordJobStructure("job1", "mod", &charts1)
	da.RecordJobStructure("job2", "mod", &charts2)

	da.RecordCollection("job1", "mod", map[string]int64{"dim1": 10})
	select {
	case <-done:
		t.Fatalf("onComplete fired before all registered jobs collected")
	default:
	}

	da.RecordCollection("job2", "mod", map[string]int64{"dim2": 20})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for onComplete callback")
	}

	if !da.flushWriteQueue(2 * time.Second) {
		t.Fatalf("failed to flush write queue")
	}

	manifestPath := filepath.Join(baseDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest manifestPayload
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if got := len(manifest.Jobs); got != 2 {
		t.Fatalf("expected 2 jobs in manifest, got %d", got)
	}
}

func testCharts(chartID, context, dimID string) collectorapi.Charts {
	return collectorapi.Charts{
		{
			ID:  chartID,
			Ctx: context,
			Dims: collectorapi.Dims{
				{ID: dimID},
			},
		},
	}
}
