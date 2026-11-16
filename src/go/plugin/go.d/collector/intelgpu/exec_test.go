// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package intelgpu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// TestMain doubles as the fake intel_gpu_top; see streamexectest.
func TestMain(m *testing.M) {
	streamexectest.RunIfFake()
	os.Exit(m.Run())
}

// newTopCollector returns an initialized collector whose ndsudo starts the test binary as intel_gpu_top. go.d.plugin
// cannot signal it, as with the root intel_gpu_top that ndsudo runs.
func newTopCollector(t *testing.T, updateEvery int, device string) (*Collector, *streamexectest.Fake) {
	t.Helper()
	fake := streamexectest.NewFake(t)
	t.Cleanup(ndexec.DenyNDSudoSignalsForTests())
	collr := New()
	collr.UpdateEvery = updateEvery
	collr.Device = device
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })
	return collr, fake
}

// goCheck runs Check in the background, since it waits for the first sample.
func goCheck(collr *Collector) <-chan error {
	done := make(chan error, 1)
	go func() { done <- collr.Check(context.Background()) }()
	return done
}

func waitCheck(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Check did not return")
		return nil
	}
}

// sendSample makes the fake print a sample as intel_gpu_top does once the next one starts: its object, with "," after
// the closing brace.
func sendSample(t *testing.T, conn *streamexectest.Conn, data []byte) {
	t.Helper()
	conn.Raw(t, strings.TrimRight(string(data), "\n")+",\n")
}

// collectFromMock returns what a collector reading data through the mock exec collects.
func collectFromMock(t *testing.T, data []byte) map[string]int64 {
	t.Helper()
	collr := New()
	collr.exec = &mockIntelGpuTop{
		gpuSummaryJson: data,
	}
	mx := collr.Collect(context.Background())
	require.NotEmpty(t, mx)
	return mx
}

func TestTopArgs(t *testing.T) {
	for name, tc := range map[string]struct {
		updateEvery int
		device      string
		wantArgs    []string
	}{
		"every second":      {updateEvery: 1, wantArgs: []string{"igt-json", "--interval", "900"}},
		"every two seconds": {updateEvery: 2, wantArgs: []string{"igt-json", "--interval", "1500"}},
		"slower collection": {updateEvery: 10, wantArgs: []string{"igt-json", "--interval", "2500"}},
		"selected GPU":      {updateEvery: 1, device: "drm:/dev/dri/card0", wantArgs: []string{"igt-device-json", "--interval", "900", "--device", "drm:/dev/dri/card0"}},
	} {
		t.Run(name, func(t *testing.T) {
			collr, fake := newTopCollector(t, tc.updateEvery, tc.device)
			done := goCheck(collr)
			conn := fake.Accept(t)
			assert.Equal(t, tc.wantArgs, conn.Args)
			conn.Exit(t, 0)
			require.Error(t, waitCheck(t, done))
		})
	}
}

func TestTopCollects(t *testing.T) {
	// The first sample may take the stall timeout (7.5s at this interval), which leaves room for a slow fake start.
	collr, fake := newTopCollector(t, 3, "")
	done := goCheck(collr)
	conn := fake.Accept(t)
	conn.Raw(t, "[\n\n") // intel_gpu_top opens its JSON array before the first sample
	sendSample(t, conn, dataIntelTopGpuJSON)
	require.NoError(t, waitCheck(t, done))
	assert.Equal(t, collectFromMock(t, dataIntelTopGpuJSON), collr.Collect(context.Background()))

	// intel_gpu_top keeps writing, so its next write after Cleanup closes its output ends it.
	conn.Noise(t, "noise")
	began := time.Now()
	collr.Cleanup(context.Background())
	assert.Less(t, time.Since(began), 2*time.Second, "Cleanup waited for a writing intel_gpu_top")
	conn.RequireExited(t)
}

func TestTopCheckFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(*testing.T)
		act     func(*testing.T, *streamexectest.Fake)
		wantErr string
	}{
		"intel_gpu_top not installed": {
			// ndsudo exits with 4 when it does not find the command.
			act:     func(t *testing.T, fake *streamexectest.Fake) { fake.Accept(t).Exit(t, 4) },
			wantErr: "intel_gpu_top exited: exit status 4",
		},
		"ndsudo not installed": {
			prepare: func(t *testing.T) {
				t.Cleanup(ndexec.SetRunnerPathsForTests("", filepath.Join(t.TempDir(), "missing-ndsudo")))
			},
			act:     func(*testing.T, *streamexectest.Fake) {},
			wantErr: "start intel_gpu_top",
		},
	} {
		t.Run(name, func(t *testing.T) {
			// The stall timeout at this interval (7.5s) leaves room for a slow fake start before the failure.
			collr, fake := newTopCollector(t, 3, "")
			if tc.prepare != nil {
				tc.prepare(t)
			}
			timing := sampleTiming(calcInterval(3))
			done := goCheck(collr)
			tc.act(t, fake)
			began := time.Now()

			require.ErrorContains(t, waitCheck(t, done), tc.wantErr)
			assert.Less(t, time.Since(began), timing.StallTimeout, "a failure waited for the stall timeout")
			fake.RequireNoStart(t, timing.RestartDelayMin+500*time.Millisecond) // no restart after a failed Check
		})
	}
}

func TestTopStaleSampleIsAGapAndRestarts(t *testing.T) {
	// A sample stays current for two sampling intervals (3s); intel_gpu_top is replaced after three (4.5s) without one.
	collr, fake := newTopCollector(t, 2, "")
	done := goCheck(collr)
	conn := fake.Accept(t)
	sendSample(t, conn, dataIntelTopGpuJSON)
	require.NoError(t, waitCheck(t, done))
	require.NotEmpty(t, collr.Collect(context.Background()))

	// intel_gpu_top keeps running and writing, but completes no sample: the sample ages out before the stall timeout
	// withdraws it, and intel_gpu_top is replaced.
	conn.Noise(t, "noise")
	stallTimeout := sampleTiming(calcInterval(2)).StallTimeout
	began := time.Now()
	require.Eventually(t, func() bool { return collr.Collect(context.Background()) == nil }, stallTimeout,
		20*time.Millisecond)
	assert.Less(t, time.Since(began), stallTimeout-500*time.Millisecond, "the sample did not age out before the stall")
	replacement := fake.Accept(t)
	conn.RequireExited(t)

	sendSample(t, replacement, dataIntelTopGpuJSON)
	require.Eventually(t, func() bool { return collr.Collect(context.Background()) != nil }, 3*time.Second,
		20*time.Millisecond)
	assert.Equal(t, collectFromMock(t, dataIntelTopGpuJSON), collr.Collect(context.Background()))
}

func TestTopCleanupIsBoundedForSilentTop(t *testing.T) {
	collr, fake := newTopCollector(t, 2, "")
	done := goCheck(collr)
	conn := fake.Accept(t)
	sendSample(t, conn, dataIntelTopGpuJSON)
	require.NoError(t, waitCheck(t, done))

	// An intel_gpu_top that stops writing cannot be ended: Cleanup waits for it at most the stall timeout.
	stallTimeout := sampleTiming(calcInterval(2)).StallTimeout
	began := time.Now()
	collr.Cleanup(context.Background())
	elapsed := time.Since(began)
	assert.GreaterOrEqual(t, elapsed, stallTimeout)
	assert.Less(t, elapsed, stallTimeout+2*time.Second)

	conn.RequireRunning(t)
	conn.Line(t, "late") // its next write ends it
	conn.WaitExited(t)
}

func TestCalcInterval(t *testing.T) {
	for updateEvery := 1; updateEvery <= 300; updateEvery++ {
		interval := calcInterval(updateEvery)
		timing := sampleTiming(interval)

		// Every collection finds a new sample.
		require.Less(t, interval, time.Duration(updateEvery)*time.Second, "update_every %d", updateEvery)
		// A sample arrives every interval; one missed sample is neither a gap nor a stall.
		require.Greater(t, timing.MaxSampleAge, interval, "update_every %d", updateEvery)
		require.Greater(t, timing.StallTimeout, timing.MaxSampleAge, "update_every %d", updateEvery)
		// Cleanup waits up to the stall timeout for an intel_gpu_top that cannot be signaled.
		require.Less(t, timing.StallTimeout, lifecycle.DefaultShutdownTimeout, "update_every %d", updateEvery)
	}
}

func TestSampleDecoder(t *testing.T) {
	dec := &sampleDecoder{}
	var samples [][]byte
	for _, line := range []string{
		"[",
		"",
		"{",
		"\t\"period\": {",
		"\t\t\"duration\": 900.1,",
		"\t\t\"unit\": \"ms\"",
		"\t},",
		"\t\"engines\": {",
		"\t}",
		"},",
		"{",
		"\t\"period\": {",
		"\t}",
		"},", // the line of a sample's closing brace ends when the next sample starts
	} {
		if sample, ok := dec.Decode([]byte(line)); ok {
			samples = append(samples, sample)
		}
	}
	// Samples are compared after decoding ends: a sample must not change when the decoder reuses its buffer.
	var got []string
	for _, sample := range samples {
		got = append(got, string(sample))
	}
	assert.Equal(t, []string{
		"{\n\t\"period\": {\n\t\t\"duration\": 900.1,\n\t\t\"unit\": \"ms\"\n\t},\n\t\"engines\": {\n\t}\n}\n",
		"{\n\t\"period\": {\n\t}\n}\n",
	}, got)
}
