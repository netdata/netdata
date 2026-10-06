// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package jetson

import (
	"bytes"
	"context"
	"net"
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
)

func TestCollectionTransitions(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	startRun(t, c)
	conn := fake.accept(t)
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

func TestSourceRecovery(t *testing.T) {
	for name, tc := range map[string]struct {
		fail func(*testing.T, net.Conn)
	}{
		"nonzero exit": {fail: func(t *testing.T, conn net.Conn) {
			sendCommand(t, conn, fakeCommand{
				Exit: new(7),
			})
		}},
		"clean exit": {fail: func(t *testing.T, conn net.Conn) {
			sendCommand(t, conn, fakeCommand{
				Exit: new(0),
			})
		}},
		"stall": {fail: func(*testing.T, net.Conn) {}},
		"unrecognized output": {fail: func(t *testing.T, conn net.Conn) {
			sendCommand(t, conn, fakeCommand{
				Line: "warning GR3D_FREQ 90%",
			})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			c, fake := newCollectorWithFake(t)
			c.timing = sourceTiming{
				maxSampleAge:    100 * time.Millisecond,
				stallTimeout:    300 * time.Millisecond,
				restartDelayMin: 100 * time.Millisecond,
				restartDelayMax: 200 * time.Millisecond,
			}
			startRun(t, c)
			conn := fake.accept(t)
			sendRecord(t, conn, "GR3D_FREQ 50%")
			waitSample(t, c, func(s sample) bool { return s.GPUUtilization != nil })
			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)

			tc.fail(t, conn)
			require.Eventually(t, func() bool { return c.latest.Load() == nil }, time.Second, 5*time.Millisecond)
			replacement := fake.accept(t)
			requireExited(t, conn) // the failed process never overlaps its replacement

			sendRecord(t, replacement, "EMC_FREQ 12%@800")
			waitSample(t, c, func(s sample) bool { return s.EMCFrequency != nil })
			assertCollected(t, c, map[string]metrix.SampleValue{
				"emc_utilization": 12,
				"emc_frequency":   800,
			})
		})
	}
}

func TestCancelTerminatesDescendants(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	stop := startRun(t, c)
	conn := fake.accept(t)
	sendCommand(t, conn, fakeCommand{
		Spawn: true,
	})
	child := fake.accept(t)
	stop()
	requireExited(t, conn)
	requireExited(t, child)
}

func TestManagedRuntime(t *testing.T) {
	c, fake := newCollectorWithFake(t)
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

	conn := fake.accept(t)
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
	require.Nil(t, c.latest.Load())
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

func TestRunSurvivesLateExecFailure(t *testing.T) {
	c, _ := newCollectorWithFake(t)
	c.findTegrastats = func() (string, error) { return filepath.Join(t.TempDir(), "missing-tegrastats"), nil }
	require.NoError(t, c.Check(t.Context()))

	// The helper starts, so Run is ready before the missing executable fails.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready, done := goRun(ctx, c)
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("helper did not signal readiness")
	}
	_, err := collecttest.CollectScalarSeries(c)
	require.Error(t, err)
	select {
	case err := <-done:
		t.Fatalf("late exec failure ended Run: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not cancel")
	}
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

func TestFollowReportsSourceFailure(t *testing.T) {
	// Exit cases wait longer than an exit can take, so only the stall case stalls.
	for name, tc := range map[string]struct {
		cmd          *fakeCommand
		stallTimeout time.Duration
		wantErr      string
	}{
		"nonzero exit": {
			cmd:          &fakeCommand{Exit: new(7)},
			stallTimeout: 10 * time.Second,
			wantErr:      "tegrastats exited: exit status 7",
		},
		"clean exit": {
			cmd:          &fakeCommand{Exit: new(0)},
			stallTimeout: 10 * time.Second,
			wantErr:      "tegrastats exited",
		},
		"stall": {
			stallTimeout: 300 * time.Millisecond,
			wantErr:      "tegrastats stopped producing records",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, fake := newCollectorWithFake(t)
			c.timing.stallTimeout = tc.stallTimeout
			require.NoError(t, c.Check(t.Context()))
			proc, err := startTegrastats(t.Context(), c.tegrastatsPath)
			require.NoError(t, err)
			conn := fake.accept(t)
			if tc.cmd != nil {
				sendCommand(t, conn, *tc.cmd)
			}
			observed, err := c.follow(t.Context(), proc)
			assert.False(t, observed)
			assert.EqualError(t, err, tc.wantErr)
			assert.Nil(t, c.latest.Load())
		})
	}
}
