// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob_test

import (
	"bytes"
	"cmp"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// job is the collector surface the framework drives for every synthetic kind.
type job interface {
	collectorapi.CollectorV2
	collectorapi.CollectorV2Runner
	collectorapi.ConfiguredVnodeConsumer
	collectorapi.StaticChartTemplateProvider
}

var (
	_ job = (*journey.Collector)(nil)
	_ job = (*lighthouse.Collector)(nil)
)

// settings override collector defaults; zero values keep them.
type settings struct {
	name        string
	checkEvery  int
	updateEvery int
}

func (s settings) apply(name *string, checkEvery, updateEvery *int) {
	*name = cmp.Or(s.name, "checkout")
	*checkEvery = cmp.Or(s.checkEvery, *checkEvery)
	*updateEvery = cmp.Or(s.updateEvery, *updateEvery)
}

// kind describes one collector that embeds the shared job, keyed by its module name.
type kind struct {
	kind    synthetic.Kind
	cadence int
	// target is the registered target of the kind configuration built by new.
	target string
	// request is what the executor receives from that configuration with default settings; new sets every option
	// that changes it away from its zero value.
	request synthetic.Request
	new     func(syntheticjob.Executor, *registry.Registry, settings) job
	// measurements are the kind metrics published from fixture(); charts are their contexts and dimensions.
	measurements map[string]float64
	charts       map[string][]string
}

var kinds = map[string]kind{
	"journey": {
		kind:    synthetic.Journey,
		cadence: 900,
		target:  "",
		request: synthetic.Request{
			Kind:                synthetic.Journey,
			Name:                "checkout",
			Script:              "export {};",
			Secrets:             map[string]string{"DEM_SECRET_LOGIN": "synthetic-password"},
			Timeout:             2 * time.Minute,
			Capture:             true,
			Trace:               true,
			BrowserErrorSummary: true,
		},
		new: func(e syntheticjob.Executor, hub *registry.Registry, s settings) job {
			c := journey.New(journey.Dependencies{
				Executor: e,
				Registry: hub,
			})
			c.Script = "export {};"
			c.Secrets = []journey.Secret{{Name: "DEM_SECRET_LOGIN", Value: "synthetic-password"}}
			c.ScreenshotOnFailure, c.TraceOnFailure, c.BrowserErrorSummary = true, true, true
			s.apply(&c.Name, &c.CheckEvery, &c.UpdateEvery)
			return c
		},
		measurements: map[string]float64{
			"tests_declared":         7,
			"tests_passed":           1,
			"tests_failed":           1,
			"tests_timed_out":        1,
			"tests_skipped":          1,
			"tests_expected_failure": 1,
			"tests_not_run":          2,
		},
		charts: map[string][]string{
			"dem_synthetic.journey.declared_tests": {"declared"},
			"dem_synthetic.journey.test_outcomes": {
				"passed",
				"failed",
				"timed_out",
				"skipped",
				"expected_failure",
				"not_run",
			},
		},
	},
	"lighthouse": {
		kind:    synthetic.Lighthouse,
		cadence: 1800,
		target:  "https://example.org/",
		request: synthetic.Request{
			Kind:    synthetic.Lighthouse,
			Name:    "checkout",
			URL:     "https://example.org/",
			Timeout: 3 * time.Minute,
			Capture: true,
		},
		new: func(e syntheticjob.Executor, hub *registry.Registry, s settings) job {
			c := lighthouse.New(lighthouse.Dependencies{
				Executor: e,
				Registry: hub,
			})
			c.URL = "https://example.org/"
			c.SaveReport = true
			s.apply(&c.Name, &c.CheckEvery, &c.UpdateEvery)
			return c
		},
		measurements: map[string]float64{
			"performance": 0,
			"fcp":         100.5,
			"lcp":         200.5,
			"tbt":         0,
			"si":          300.5,
			"cls":         0.12,
		},
		charts: map[string][]string{
			"dem_synthetic.lighthouse.performance": {"performance"},
			"dem_synthetic.lighthouse.timings":     {"fcp", "lcp", "tbt", "speed_index"},
			"dem_synthetic.lighthouse.cls":         {"cls"},
		},
	},
}

func forEachKind(t *testing.T, test func(*testing.T, string, kind)) {
	t.Helper()
	for module, k := range kinds {
		t.Run(module, func(t *testing.T) { test(t, module, k) })
	}
}

// measurementNames lists the kind metrics of every synthetic kind.
func measurementNames() []string {
	var names []string
	for _, k := range kinds {
		for name := range k.measurements {
			names = append(names, name)
		}
	}
	return names
}

// fixture is a completed observation carrying the measurements of every kind.
func fixture() synthetic.Run {
	return synthetic.Run{
		Outcome:    synthetic.Failed,
		DurationMS: new(1250.5),
		Tests: &synthetic.TestCounts{
			Declared:        7,
			Passed:          1,
			Failed:          1,
			TimedOut:        1,
			Skipped:         1,
			ExpectedFailure: 1,
			NotRun:          2,
		},
		Metrics: &synthetic.LabMetrics{
			Performance: new(0.0),
			FCPMS:       new(100.5),
			LCPMS:       new(200.5),
			TBTMS:       new(0.0),
			SIMS:        new(300.5),
			CLS:         new(0.12),
		},
	}
}

func waitLatest(t *testing.T, hub *registry.Registry, id string) synthetic.Run {
	t.Helper()
	var run synthetic.Run
	require.Eventually(t, func() bool {
		for _, j := range hub.Snapshot(time.Now()) {
			if j.JobID == id {
				var ok bool
				run, ok = latestRun(hub, j)
				return ok
			}
		}
		return false
	}, 3*time.Second, time.Millisecond)
	return run
}

// latestRun returns the full latest run of a job listed by hub.
func latestRun(hub *registry.Registry, job registry.Job) (synthetic.Run, bool) {
	if !job.HasObservation {
		return synthetic.Run{}, false
	}
	return hub.Latest(job.JobID, job.Latest.ID)
}

// start initializes c, runs it until ready and returns an idempotent stop that requires Run to join.
func start(t *testing.T, c job) func() {
	t.Helper()
	require.NoError(t, c.Init(t.Context()))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("startup failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("startup timed out")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Error("Run did not join executor")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(4 * time.Second):
		t.Fatal("owned operation did not finish")
		var zero T
		return zero
	}
}

func collect(t *testing.T, c collectorapi.CollectorV2) map[string]float64 {
	t.Helper()
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	return values
}

// output is a plugin output buffer shared by the job goroutine and the test.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}
