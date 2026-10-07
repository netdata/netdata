// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// TestMain doubles as the fake tegrastats; see streamexectest.
func TestMain(m *testing.M) {
	streamexectest.RunIfFake()
	os.Exit(m.Run())
}

func TestCollectionTransitions(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	startRun(t, c)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"--interval", "1000"}, conn.Args)
	_, err := collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "no fresh")

	sendRecord(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	want := map[string]metrix.SampleValue{
		"gpu_utilization": 25,
		"gpu_frequency":   900,
		"emc_utilization": 30,
		"emc_frequency":   1600,
	}
	assertCollected(t, c, want)
	assertCollected(t, c, want) // publication can reuse a fresh record

	sendRecord(t, conn, "GR3D_FREQ @[300,off,600] EMC_FREQ 0%")
	waitSample(t, c, func(s sample) bool { return len(s.GPCFrequencies) == 3 })
	assertCollected(t, c, map[string]metrix.SampleValue{
		`gpu_gpc_frequency{gpc="0"}`: 300,
		`gpu_gpc_frequency{gpc="2"}`: 600,
		"emc_utilization":            0,
	}) // no retained scalar or utilization from the previous record
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	sendRecord(t, conn, "TEMP@42C")
	waitSample(t, c, func(s sample) bool { return !s.hasReadings() })
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "no supported")

	sendRecord(t, conn, "GR3D_FREQ 0%@0 EMC_FREQ 0%@0")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	assertCollected(t, c, map[string]metrix.SampleValue{
		"gpu_utilization": 0,
		"gpu_frequency":   0,
		"emc_utilization": 0,
		"emc_frequency":   0,
	})
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestManagedRuntime(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	c.timing.MaxSampleAge = time.Hour // only withdrawal, not age, can remove the record after stop
	out := &lockedBuffer{}
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "go.d",
		Name:        "localhost",
		ModuleName:  "jetson",
		FullName:    "jetson_localhost",
		Module:      c,
		Out:         out,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("managed job failed to stop")
		}
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("managed startup blocked")
	}
	require.NoError(t, run.StartupErr())
	require.True(t, run.Running())

	conn := fake.Accept(t)
	sendRecord(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	// The emitted frame carries the record's only reading equal to 25, the GPU utilization.
	setUtilization := regexp.MustCompile(`(?m)^SET '[^']+' = 25$`)
	require.Eventually(t, func() bool {
		job.Tick(1)
		return setUtilization.MatchString(out.String())
	}, 3*time.Second, 20*time.Millisecond)

	run.Stop(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("managed Run not joined")
	}
	_, ok := c.latestSample()
	require.False(t, ok)
	assert.False(t, run.Running())
}

func TestRunStartupFailure(t *testing.T) {
	c, _ := newCollectorWithFake(t)
	require.NoError(t, c.Check(t.Context()))
	t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-wrapper"), ""))

	ready, done := goRun(t.Context(), c)
	select {
	case err := <-done:
		require.ErrorContains(t, err, "start tegrastats")
	case <-time.After(3 * time.Second):
		t.Fatal("startup failure not returned")
	}
	select {
	case <-ready:
		t.Fatal("ready before acquisition")
	default:
	}
}

func TestRunWithoutCheck(t *testing.T) {
	ready, done := goRun(t.Context(), New())
	select {
	case err := <-done:
		require.EqualError(t, err, "tegrastats executable has not been resolved")
	case <-time.After(3 * time.Second):
		t.Fatal("unresolved executable not reported")
	}
	select {
	case <-ready:
		t.Fatal("ready without an executable")
	default:
	}
}

// newCollectorWithFake returns a collector whose tegrastats is the test binary, started through a pass-through
// nd-run.
func newCollectorWithFake(t *testing.T) (*Collector, *streamexectest.Fake) {
	t.Helper()
	fake := streamexectest.NewFake(t)
	c := New()
	c.findTegrastats = func() (string, error) { return fake.Binary, nil }
	return c, fake
}

// sendRecord makes the fake print a complete record with the given readings.
func sendRecord(t *testing.T, conn *streamexectest.Conn, readings string) {
	t.Helper()
	conn.Line(t, testRecordPrefix+readings)
}

// startRun runs the collector until the test ends or the returned stop is called.
// It returns once Run signals readiness.
func startRun(t *testing.T, c *Collector) (stop context.CancelFunc) {
	t.Helper()
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	ready, done := goRun(ctx, c)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("Run did not join its owned process")
		}
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("Run failed before readiness: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("readiness waited for telemetry")
	}
	return cancel
}

// goRun calls Run in the background and returns its readiness and result channels.
func goRun(ctx context.Context, c *Collector) (ready <-chan struct{}, done <-chan error) {
	readyCh := make(chan struct{})
	doneCh := make(chan error, 1)
	go func() { doneCh <- c.Run(ctx, func() { close(readyCh) }) }()
	return readyCh, doneCh
}

// waitSample waits until the collector holds a fresh record matching match.
func waitSample(t *testing.T, c *Collector, match func(sample) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		s, ok := c.latestSample()
		return ok && match(s)
	}, 3*time.Second, 5*time.Millisecond)
}

func assertCollected(t *testing.T, c *Collector, want map[string]metrix.SampleValue) {
	t.Helper()
	got, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
