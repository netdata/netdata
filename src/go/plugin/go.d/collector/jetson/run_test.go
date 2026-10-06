// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux || darwin

package jetson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

const testEnvelope = "RAM 20/100MB (lfb 3x4MB) CPU [0%@100] "

type sourceCommand struct {
	Line  string
	Exit  *int
	Spawn bool
}

// The real subprocess only copies test-controlled lines to stdout. It knows
// neither the collector's state nor the expected metrics.
func TestMain(m *testing.M) {
	if socket := os.Getenv("NETDATA_JETSON_TEST_SOCKET"); socket != "" {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			os.Exit(121)
		}
		defer conn.Close()
		if os.Getenv("NETDATA_JETSON_TEST_CHILD") == "1" {
			_, _ = io.Copy(io.Discard, conn)
			return
		}
		if len(os.Args) != 3 || os.Args[1] != "--interval" || os.Args[2] != "1000" {
			os.Exit(122)
		}
		decoder := json.NewDecoder(conn)
		for {
			var cmd sourceCommand
			if err := decoder.Decode(&cmd); err != nil {
				return
			}
			if cmd.Exit != nil {
				os.Exit(*cmd.Exit)
			}
			if cmd.Spawn {
				child := exec.Command(os.Args[0])
				child.Env = append(os.Environ(), "NETDATA_JETSON_TEST_CHILD=1")
				if child.Start() != nil {
					os.Exit(123)
				}
			}
			if cmd.Line != "" {
				_, _ = fmt.Fprintln(os.Stdout, cmd.Line)
			}
		}
	}
	os.Exit(m.Run())
}

type fixtureSource struct {
	listener  *net.UnixListener
	collector *Collector
}

func newFixtureSource(t *testing.T) *fixtureSource {
	t.Helper()
	// Unix socket names must fit the platform's sockaddr_un path limit.
	dir, err := os.MkdirTemp("", "jetson-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "source.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	t.Setenv("NETDATA_JETSON_TEST_SOCKET", socket)
	wrapper := filepath.Join(dir, "nd-run")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$@\"\n"), 0700))
	restore := ndexec.SetRunnerPathsForTests(wrapper, "")
	t.Cleanup(restore)
	binary, err := os.Executable()
	require.NoError(t, err)
	c := New()
	c.findBinary = func() (string, error) { return binary, nil }
	return &fixtureSource{
		listener:  listener,
		collector: c,
	}
}

func (f *fixtureSource) accept(t *testing.T) net.Conn {
	t.Helper()
	require.NoError(t, f.listener.SetDeadline(time.Now().Add(5*time.Second)))
	conn, err := f.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func sendLine(t *testing.T, conn net.Conn, fields string) {
	t.Helper()
	require.NoError(t, json.NewEncoder(conn).Encode(sourceCommand{
		Line: testEnvelope + fields,
	}))
}

func waitObservation(t *testing.T, c *Collector, check func(sample) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		latest := c.latest.Load()
		return latest != nil && check(latest.sample)
	}, 3*time.Second, 5*time.Millisecond)
}

func startFixture(t *testing.T, f *fixtureSource) net.Conn {
	t.Helper()
	c := f.collector
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(t.Context(), func() { close(ready) }) }()
	t.Cleanup(func() {
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("Run did not join owned process")
		}
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("Run failed before readiness: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("readiness waited for telemetry")
	}
	return f.accept(t)
}

func cycle(t *testing.T, c *Collector) error {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	control := managed.CycleController()
	control.BeginCycle()
	if err := c.Collect(t.Context()); err != nil {
		control.AbortCycle()
		return err
	}
	return control.CommitCycleSuccess()
}

func values(c *Collector) map[string]float64 {
	result := make(map[string]float64)
	c.store.Read().ForEachSeries(func(name string, labels metrix.LabelView, value float64) {
		if gpc, ok := labels.Get("gpc"); ok {
			name += ":" + gpc
		}
		result[name] = value
	})
	return result
}

func TestCollectionTransitions(t *testing.T) {
	f := newFixtureSource(t)
	c := f.collector
	conn := startFixture(t, f)
	require.ErrorContains(t, cycle(t, c), "no fresh")

	sendLine(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitObservation(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	require.NoError(t, cycle(t, c))
	assert.Equal(t, map[string]float64{
		"gpu_utilization": 25, "gpu_frequency": 900, "emc_utilization": 30, "emc_frequency": 1600,
	}, values(c))
	require.NoError(t, cycle(t, c)) // publication phase can reuse a fresh observation

	sendLine(t, conn, "GR3D_FREQ @[300,off,600] EMC_FREQ 0%")
	waitObservation(t, c, func(s sample) bool { return len(s.GPCFrequencies) == 3 })
	require.NoError(t, cycle(t, c))
	assert.Equal(t, map[string]float64{
		"gpu_gpc_frequency:0": 300, "gpu_gpc_frequency:2": 600, "emc_utilization": 0,
	}, values(c)) // no retained scalar or utilization from the previous line
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	sendLine(t, conn, "TEMP@42C")
	waitObservation(t, c, func(s sample) bool { return !s.hasMetrics() })
	require.ErrorContains(t, cycle(t, c), "no supported")

	sendLine(t, conn, "GR3D_FREQ 0%@0 EMC_FREQ 0%@0")
	waitObservation(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	require.NoError(t, cycle(t, c))
	assert.Equal(t, map[string]float64{
		"gpu_utilization": 0, "gpu_frequency": 0, "emc_utilization": 0, "emc_frequency": 0,
	}, values(c))
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestSourceRecovery(t *testing.T) {
	for name, tc := range map[string]struct {
		fail    func(*testing.T, net.Conn)
		silence bool
	}{
		"nonzero exit": {fail: func(t *testing.T, conn net.Conn) {
			code := 7
			require.NoError(t, json.NewEncoder(conn).Encode(sourceCommand{
				Exit: &code,
			}))
		}},
		"output EOF": {fail: func(t *testing.T, conn net.Conn) { require.NoError(t, conn.Close()) }},
		"silence":    {silence: true},
		"unrecognized output": {silence: true, fail: func(t *testing.T, conn net.Conn) {
			require.NoError(t, json.NewEncoder(conn).Encode(sourceCommand{
				Line: "warning GR3D_FREQ 90%",
			}))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixtureSource(t)
			c := f.collector
			c.timing = sourceTiming{
				freshFor:   100 * time.Millisecond,
				stallAfter: 300 * time.Millisecond,
				retryMin:   100 * time.Millisecond,
				retryMax:   200 * time.Millisecond,
			}
			conn := startFixture(t, f)
			sendLine(t, conn, "GR3D_FREQ 50%")
			waitObservation(t, c, func(s sample) bool { return s.GPUUtilization != nil })
			require.NoError(t, cycle(t, c))
			if tc.fail != nil {
				tc.fail(t, conn)
			}
			if tc.silence {
				require.Eventually(t, func() bool { return cycle(t, c) != nil }, time.Second, 5*time.Millisecond)
			} else {
				require.Eventually(t, func() bool { return c.latest.Load() == nil }, time.Second, 5*time.Millisecond)
			}
			replacement := f.accept(t)
			sendLine(t, replacement, "EMC_FREQ 12%@800")
			waitObservation(t, c, func(s sample) bool { return s.EMCFrequency != nil })
			require.NoError(t, cycle(t, c))
			assert.Equal(t, map[string]float64{"emc_utilization": 12, "emc_frequency": 800}, values(c))
		})
	}
}

func TestCleanupContainsDescendants(t *testing.T) {
	f := newFixtureSource(t)
	conn := startFixture(t, f)
	require.NoError(t, json.NewEncoder(conn).Encode(sourceCommand{
		Spawn: true,
	}))
	child := f.accept(t)
	f.collector.Cleanup(t.Context())
	for _, connection := range []net.Conn{conn, child} {
		require.NoError(t, connection.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, err := connection.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF) // every owned process closed its socket
	}
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
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

func TestManagedRuntime(t *testing.T) {
	f := newFixtureSource(t)
	c := f.collector
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
		c.Cleanup(context.Background())
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("managed startup blocked")
	}
	require.NoError(t, run.StartupErr())
	require.True(t, run.Running())
	conn := f.accept(t)
	sendLine(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitObservation(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	require.Eventually(t, func() bool {
		job.Tick(1)
		return bytes.Contains([]byte(out.String()), []byte("jetson.gpu_utilization"))
	}, 3*time.Second, 20*time.Millisecond)
	assert.Contains(t, out.String(), "SET 'utilization' = 25")
	run.Stop(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("managed Run not joined")
	}
	require.Nil(t, c.latest.Load())
	assert.False(t, run.Running())
}

func TestStartupAndLateExecFailure(t *testing.T) {
	for name, tc := range map[string]struct{ missingWrapper bool }{
		"helper acquisition fails":              {missingWrapper: true},
		"target exec fails after helper starts": {},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixtureSource(t)
			c := f.collector
			c.findBinary = func() (string, error) { return filepath.Join(t.TempDir(), "missing-tegrastats"), nil }
			require.NoError(t, c.Check(t.Context()))
			if tc.missingWrapper {
				restore := ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-wrapper"), "")
				defer restore()
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan struct{})
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx, func() { close(ready) }) }()
			if tc.missingWrapper {
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
			} else {
				select {
				case <-ready:
				case <-time.After(3 * time.Second):
					t.Fatal("helper did not signal readiness")
				}
				require.Error(t, cycle(t, c))
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
		})
	}
}
