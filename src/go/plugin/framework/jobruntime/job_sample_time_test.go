// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// Every collector chart sample ends with its run's start time
// (`END <seconds> <microseconds> 1`) and its BEGIN carries no interval, so the
// Agent places each sample at its collection time, whatever happened to the
// chart's earlier samples: a failed cycle, a cycle that did not observe the chart
// or a rejected host scope.
func TestJobV2ChartSampleTimes(t *testing.T) {
	type run struct {
		failed     bool // the collector returns an error
		skipIdle   bool // the idle chart's series are not written
		rejectNode bool // the node scope's output is rejected
	}
	// Runs are ten seconds apart. Keys are "<host>/<chart>"; the default host is "".
	runs := []run{{}, {}, {failed: true}, {skipIdle: true}, {}, {rejectNode: true}, {}}
	node := metrix.HostScope{
		ScopeKey: "node",
		GUID:     sharedGUID,
		Hostname: "node",
	}

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		at := func(s int) string { return endAt(start.Add(time.Duration(s) * time.Second)) }
		want := []map[string]string{
			{"/workers_busy": at(0), "/workers_idle": at(0), "node/workers_busy": at(0)},
			{"/workers_busy": at(10), "/workers_idle": at(10), "node/workers_busy": at(10)},
			{},
			{"/workers_busy": at(30), "node/workers_busy": at(30)},
			{"/workers_busy": at(40), "/workers_idle": at(40), "node/workers_busy": at(40)},
			{"/workers_busy": at(50), "/workers_idle": at(50)},
			{"/workers_busy": at(60), "/workers_idle": at(60), "node/workers_busy": at(60)},
		}
		var out bytes.Buffer
		var current run
		output := writeFunc(func(payload []byte) (int, error) {
			if current.rejectNode && bytes.Contains(payload, []byte("HOST '"+sharedGUID+"'")) {
				return 0, errors.New("node output rejected")
			}
			return out.Write(payload)
		})
		store := metrix.NewCollectorStore()
		job := NewJobV2(JobV2Config{
			PluginName: "go.d",
			ModuleName: "test",
			Name:       "job",
			FullName:   "test_job",
			Module: &mockModuleV2{
				store:    store,
				template: chartTemplateV2BusyIdle(),
				collectFunc: func(context.Context) error {
					if current.failed {
						return errors.New("scrape failed")
					}
					meter := store.Write().SnapshotMeter("apache")
					meter.Gauge("workers_busy").Observe(7)
					if !current.skipIdle {
						meter.Gauge("workers_idle").Observe(3)
					}
					meter.WithHostScope(node).Gauge("workers_busy").Observe(5)
					return nil
				},
			},
			Out:         output,
			UpdateEvery: 10,
		})
		require.NoError(t, job.AutoDetectionManaged(context.Background()))

		var got []map[string]string
		for i, r := range runs {
			if i > 0 {
				time.Sleep(10 * time.Second)
			}
			current = r
			out.Reset()
			job.runOnce()
			got = append(got, collectorSampleTimes(out.String(), "test_job"))
		}
		assert.Equal(t, want, got)
	})
}

// A scrape slower than the update interval drops the next tick and delivers its
// sample late; the Agent would re-anchor the chart to that late receipt time if
// the sample did not carry its collection time.
func TestJobV2ChartSampleTimesAfterSlowScrape(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		got := map[string][]string{}
		out := writeFunc(func(frame []byte) (int, error) {
			for key, end := range collectorSampleTimes(string(frame), "test_job") {
				got[key] = append(got[key], end)
			}
			return len(frame), nil
		})
		store := metrix.NewCollectorStore()
		job := NewJobV2(JobV2Config{
			PluginName:  "go.d",
			ModuleName:  "test",
			Name:        "job",
			FullName:    "test_job",
			UpdateEvery: 10,
			Out:         out,
			Module: &mockModuleV2{
				store:    store,
				template: chartTemplateV2BusyIdle(),
				collectFunc: func(context.Context) error {
					offset := time.Since(start)
					meter := store.Write().SnapshotMeter("apache")
					meter.Gauge("workers_busy").Observe(7)
					if offset != 30*time.Second {
						meter.Gauge("workers_idle").Observe(3)
					}
					if offset == 10*time.Second {
						time.Sleep(15 * time.Second)
					}
					return nil
				},
			},
		})
		require.NoError(t, job.AutoDetectionManaged(context.Background()))
		run := NewManagedRun(context.Background(), nil)
		done := make(chan struct{})
		go func() {
			defer close(done)
			job.StartManaged(run)
		}()
		defer func() { run.Stop(nil); <-done }()
		synctest.Wait()

		// Ticks every ten seconds; the tick at 20 s arrives while run 10 s is busy.
		for clock := 10; clock <= 70; clock += 10 {
			job.Tick(clock)
			synctest.Wait()
			if clock < 70 {
				time.Sleep(10 * time.Second)
			}
		}
		at := func(s int) string { return endAt(start.Add(time.Duration(s) * time.Second)) }
		assert.Equal(t, map[string][]string{
			"/workers_busy": {at(0), at(10), at(30), at(40), at(50), at(60)},
			"/workers_idle": {at(0), at(10), at(40), at(50), at(60)},
		}, got)
	})
}

// V1 collector charts follow the same rule. A failed run (empty metrics), a
// SkipGaps chart without values, a vnode run without values (its frame is dropped)
// and a rejected write deliver no sample; a chart without values still delivers an
// empty sample.
func TestJobV1ChartSampleTimes(t *testing.T) {
	type run struct {
		failed   bool // Collect returns no metrics
		noSparse bool // the SkipGaps chart's dimension is missing
		reject   bool // the collector frame is rejected
	}
	runs := []run{{}, {failed: true}, {noSparse: true}, {reject: true}, {}}

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		at := func(s int) string { return endAt(start.Add(time.Duration(s) * time.Second)) }
		want := []map[string]string{
			{"/work": at(0), "/empty": at(0), "/sparse": at(0)},
			{},
			{"/work": at(20), "/empty": at(20)},
			{},
			{"/work": at(40), "/empty": at(40), "/sparse": at(40)},
		}
		var out bytes.Buffer
		var current run
		output := writeFunc(func(payload []byte) (int, error) {
			if current.reject && bytes.Contains(payload, []byte("test_job.work")) {
				return 0, errors.New("collector output rejected")
			}
			return out.Write(payload)
		})
		charts := v1SampleTimeCharts()
		job := NewJob(JobConfig{
			PluginName: "go.d",
			ModuleName: "test",
			Name:       "job",
			FullName:   "test_job",
			Module: &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts { return &charts },
				CollectFunc: func(context.Context) map[string]int64 {
					if current.failed {
						return nil
					}
					mx := map[string]int64{"value": 7}
					if !current.noSparse {
						mx["sparse"] = 1
					}
					return mx
				},
			},
			Out:         output,
			UpdateEvery: 10,
		})
		require.NoError(t, job.AutoDetectionManaged(context.Background()))

		var got []map[string]string
		for i, r := range runs {
			if i > 0 {
				time.Sleep(10 * time.Second)
			}
			current = r
			out.Reset()
			job.runOnce()
			got = append(got, collectorSampleTimes(out.String(), "test_job"))
		}
		assert.Equal(t, want, got)
	})

	t.Run("vnode run without values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var out bytes.Buffer
			charts := v1SampleTimeCharts()
			values := true
			job := NewJob(JobConfig{
				PluginName: "go.d",
				ModuleName: "test",
				Name:       "job",
				FullName:   "test_job",
				Module: &collectorapi.MockCollectorV1{
					ChartsFunc: func() *collectorapi.Charts { return &charts },
					CollectFunc: func(context.Context) map[string]int64 {
						if values {
							return map[string]int64{"value": 7, "sparse": 1}
						}
						return map[string]int64{"other": 1}
					},
				},
				Out:         &out,
				UpdateEvery: 10,
				Vnode: vnodes.VirtualNode{
					Name:     "device",
					Hostname: "device",
					GUID:     sharedGUID,
				},
			})
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce() // creates the charts
			require.NotEmpty(t, collectorSampleTimes(out.String(), "test_job"))
			time.Sleep(10 * time.Second)
			values = false
			out.Reset()
			job.runOnce()
			assert.Empty(t, collectorSampleTimes(out.String(), "test_job"))
		})
	})
}

func chartTemplateV2BusyIdle() string {
	return `
version: v1
groups:
  - family: Workers
    metrics:
      - apache.workers_busy
      - apache.workers_idle
    charts:
      - id: workers_busy
        title: Workers Busy
        context: workers_busy
        units: workers
        dimensions:
          - selector: apache.workers_busy
            name: busy
      - id: workers_idle
        title: Workers Idle
        context: workers_idle
        units: workers
        dimensions:
          - selector: apache.workers_idle
            name: idle
`
}

func v1SampleTimeCharts() collectorapi.Charts {
	return collectorapi.Charts{
		{ID: "work", Title: "Work", Units: "units", Dims: collectorapi.Dims{{ID: "value"}}},
		{ID: "empty", Title: "Empty", Units: "units", Dims: collectorapi.Dims{{ID: "missing"}}},
		{ID: "sparse", Title: "Sparse", Units: "units", SkipGaps: true, Dims: collectorapi.Dims{{ID: "sparse"}}},
	}
}

// collectorSampleTimes is sampleTimes for one job's collector charts, keyed
// "<host>/<chart id>" with the vnode GUID shown as "node"; job self charts are
// left out.
func collectorSampleTimes(wire, typeID string) map[string]string {
	out := make(map[string]string)
	for key, end := range sampleTimes(wire) {
		host, chart, _ := bytes.Cut([]byte(key), []byte("/"))
		id, ok := bytes.CutPrefix(chart, []byte(typeID+"."))
		if !ok {
			continue
		}
		h := string(host)
		if h == sharedGUID {
			h = "node"
		}
		out[h+"/"+string(id)] = end
	}
	return out
}
