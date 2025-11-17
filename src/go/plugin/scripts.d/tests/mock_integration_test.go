//go:build linux

package tests

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	ndexec "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	runtimepkg "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	specpkg "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
	"github.com/stretchr/testify/require"
)

const (
	testScheduler  = "mock-scheduler"
	mockPluginDir  = "plugins"
	schedulerStart = 100 * time.Millisecond
)

func TestMockPluginsProduceExpectedStatesAndPerfdata(t *testing.T) {
	sched, emitter, metas := startTestScheduler(t, []specpkg.JobSpec{
		newJobSpec(t, "mock_ok", "check_mock_ok.sh", nil),
		newJobSpec(t, "mock_warn", "check_mock_warn.sh", func(sp *specpkg.JobSpec) { sp.MaxCheckAttempts = 1 }),
		newJobSpec(t, "mock_crit", "check_mock_crit.sh", func(sp *specpkg.JobSpec) { sp.MaxCheckAttempts = 1 }),
	})
	waitForJobs(t, emitter, []string{"mock_ok", "mock_warn", "mock_crit"})

	metrics := sched.CollectMetrics()
	assertMetricEquals(t, metrics, metas["mock_ok"], "state", "ok", 1)
	assertMetricEquals(t, metrics, metas["mock_warn"], "state", "warning", 1)
	assertMetricEquals(t, metrics, metas["mock_crit"], "state", "critical", 1)

	// perfdata hidden dims exist
	labelID := ids.Sanitize("value")
	ck := metas["mock_ok"].PerfdataMetricID(labelID, "warn_defined")
	require.Equal(t, int64(1), metrics[ck], "warn metadata missing")
}

func TestMockPluginMacrosExposeExpectedValues(t *testing.T) {
	spec := newJobSpec(t, "mock_macro", "check_mock_macro.sh", func(sp *specpkg.JobSpec) {
		sp.Vnode = "mock-host"
		sp.ArgValues = []string{"8080"}
		sp.MaxCheckAttempts = 3
	})
	if spec.Vnode == "" {
		t.Fatalf("vnode not set on job spec")
	}
	_, emitter, _ := startTestScheduler(t, []specpkg.JobSpec{spec})
	results := waitForResults(t, emitter, 1)
	require.NoError(t, results[0].Err)
	output := string(results[0].Output)
	require.Contains(t, output, "HOSTNAME=mock-host")
	require.Contains(t, output, "HOSTADDRESS=203.0.113.10")
	require.Contains(t, output, "HOSTALIAS=mock-host-alias")
	require.Contains(t, output, "SERVICEATTEMPT=1")
	require.Contains(t, output, "MAXSERVICEATTEMPTS=3")
	require.Contains(t, output, "HOSTLABEL_REGION=testlab")
	require.Contains(t, output, "HOST_CUSTOM_DC=east")
	require.Contains(t, output, "ARG1=8080")
}

func TestMockPluginHandlesLongOutputAndLogs(t *testing.T) {
	_, emitter, _ := startTestScheduler(t, []specpkg.JobSpec{newJobSpec(t, "mock_long", "check_mock_long.sh", nil)})
	results := waitForResults(t, emitter, 1)
	longOut := results[0].Parsed.LongOutput
	require.Contains(t, longOut, "line-one details")
	require.Contains(t, longOut, "line-two")
}

func TestMockSlowPluginRaisesSkipMetric(t *testing.T) {
	spec := newJobSpec(t, "mock_slow", "check_mock_slow.sh", func(sp *specpkg.JobSpec) {
		sp.Args = []string{"2"}
		sp.CheckInterval = 200 * time.Millisecond
		sp.RetryInterval = 200 * time.Millisecond
	})
	sched, _, _ := startTestScheduler(t, []specpkg.JobSpec{spec})
	time.Sleep(2500 * time.Millisecond)
	metrics := sched.CollectMetrics()
	key := charts.SchedulerMetricKey(testScheduler, charts.ChartSchedulerRate, "skipped")
	if metrics[key] == 0 {
		t.Fatalf("scheduler skip counter not incremented: %d", metrics[key])
	}
}

// --- helpers ---

type recordingEmitter struct {
	mu        sync.Mutex
	results   []runtimepkg.ExecutionResult
	snapshots []runtimepkg.JobSnapshot
}

func newRecordingEmitter() *recordingEmitter {
	return &recordingEmitter{}
}

func (r *recordingEmitter) Emit(job runtimepkg.JobRuntime, res runtimepkg.ExecutionResult, snap runtimepkg.JobSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resCopy := res
	resCopy.Job = job
	r.results = append(r.results, resCopy)
	r.snapshots = append(r.snapshots, snap)
}

func (r *recordingEmitter) Close() error { return nil }

func waitForResults(t *testing.T, emitter *recordingEmitter, want int) []runtimepkg.ExecutionResult {
	deadline := time.Now().Add(5 * time.Second)
	for {
		emitter.mu.Lock()
		if len(emitter.results) >= want {
			out := append([]runtimepkg.ExecutionResult(nil), emitter.results...)
			emitter.mu.Unlock()
			return out
		}
		emitter.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %d results (have %d)", want, len(emitter.results))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForJobs(t *testing.T, emitter *recordingEmitter, jobs []string) []runtimepkg.ExecutionResult {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	target := make(map[string]struct{}, len(jobs))
	for _, j := range jobs {
		target[j] = struct{}{}
	}
	for {
		emitter.mu.Lock()
		copyResults := append([]runtimepkg.ExecutionResult(nil), emitter.results...)
		emitter.mu.Unlock()
		seen := make(map[string]bool, len(copyResults))
		for _, res := range copyResults {
			seen[res.Job.Spec.Name] = true
		}
		missing := false
		for name := range target {
			if !seen[name] {
				missing = true
				break
			}
		}
		if !missing {
			return copyResults
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for jobs %v (seen %v)", jobs, seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func findResult(t *testing.T, results []runtimepkg.ExecutionResult, jobName string) runtimepkg.ExecutionResult {
	for _, res := range results {
		if res.Job.Spec.Name == jobName {
			return res
		}
	}
	names := make([]string, 0, len(results))
	for _, res := range results {
		names = append(names, res.Job.Spec.Name)
	}
	t.Fatalf("no result captured for %s (have %v)", jobName, names)
	return runtimepkg.ExecutionResult{}
}

func startTestScheduler(t *testing.T, jobs []specpkg.JobSpec) (*runtimepkg.Scheduler, *recordingEmitter, map[string]charts.JobIdentity) {
	t.Helper()
	ensureNdRun(t)
	emitter := newRecordingEmitter()
	periods := compileDefaultPeriods(t)
	workers := len(jobs)
	if workers == 0 {
		workers = 1
	}
	identities := make(map[string]charts.JobIdentity, len(jobs))
	for _, job := range jobs {
		identities[job.Name] = charts.NewJobIdentity(testScheduler, job)
	}
	sched, err := runtimepkg.NewScheduler(runtimepkg.SchedulerConfig{
		Workers:       workers,
		SchedulerName: testScheduler,
		UserMacros:    map[string]string{"USER1": "/usr/lib/nagios/plugins"},
		VnodeLookup:   vnodeLookup,
	})
	require.NoError(t, err)
	for _, job := range jobs {
		_, err := sched.RegisterJob(runtimepkg.JobRegistration{
			Spec:             job,
			Emitter:          emitter,
			RegisterPerfdata: func(specpkg.JobSpec, output.PerfDatum) {},
			Periods:          periods,
		})
		require.NoError(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, sched.Start(ctx))
	time.Sleep(schedulerStart)
	t.Cleanup(func() {
		cancel()
		sched.Stop()
	})
	return sched, emitter, identities
}

func vnodeLookup(spec specpkg.JobSpec) runtimepkg.VnodeInfo {
	if spec.Vnode == "" {
		return runtimepkg.VnodeInfo{}
	}
	return runtimepkg.VnodeInfo{
		Hostname: spec.Vnode,
		Address:  "203.0.113.10",
		Alias:    spec.Vnode + "-alias",
		Labels:   map[string]string{"region": "testlab"},
		Custom:   map[string]string{"DC": "east"},
	}
}

func compileDefaultPeriods(t *testing.T) *timeperiod.Set {
	configs := timeperiod.EnsureDefault(nil)
	set, err := timeperiod.Compile(configs)
	require.NoError(t, err)
	return set
}

func newJobSpec(t *testing.T, name, script string, fn func(*specpkg.JobSpec)) specpkg.JobSpec {
	t.Helper()
	path := scriptPath(t, script)
	spec := specpkg.JobSpec{
		Name:             name,
		Plugin:           path,
		Timeout:          5 * time.Second,
		CheckInterval:    1 * time.Second,
		RetryInterval:    500 * time.Millisecond,
		MaxCheckAttempts: 3,
		Args:             []string{},
		ArgValues:        []string{},
		Environment:      map[string]string{},
		CustomVars:       map[string]string{},
	}
	if fn != nil {
		fn(&spec)
	}
	return spec
}

func scriptPath(t *testing.T, name string) string {
	t.Helper()
	rel := filepath.Join(mockPluginDir, name)
	abs, err := filepath.Abs(rel)
	require.NoError(t, err)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("mock plugin %s missing: %v", abs, err)
	}
	return abs
}

func ensureNdRun(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	stubRun := filepath.Join(dir, "nd-run")
	stubSudo := filepath.Join(dir, "ndsudo")
	content := "#!/usr/bin/env bash\ncmd=\"$1\"\nshift\nexec \"$cmd\" \"$@\"\n"
	require.NoError(t, os.WriteFile(stubRun, []byte(content), 0o755))
	require.NoError(t, os.WriteFile(stubSudo, []byte(content), 0o755))
	oldDir := buildinfo.NetdataBinDir
	buildinfo.NetdataBinDir = dir
	ndexec.SetRunnerPathsForTests(stubRun, stubSudo)
	t.Cleanup(func() {
		buildinfo.NetdataBinDir = oldDir
	})
}

func assertMetricEquals(t *testing.T, metrics map[string]int64, meta charts.JobIdentity, chart, dim string, want int64) {
	key := meta.TelemetryMetricID(chart, dim)
	got := metrics[key]
	if got != want {
		t.Fatalf("metric %s = %d, want %d (metrics=%v)", key, got, want, metrics)
	}
}
