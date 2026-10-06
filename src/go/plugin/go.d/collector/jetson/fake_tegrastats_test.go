// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package jetson

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const (
	fakeSocketEnv = "NETDATA_JETSON_TEST_SOCKET"
	fakeChildEnv  = "NETDATA_JETSON_TEST_CHILD"
)

// fakeCommand is one instruction from a test to a running fake tegrastats.
type fakeCommand struct {
	Line  string
	Exit  *int
	Spawn bool
}

// TestMain doubles as the fake tegrastats: started by the collector with the fake
// socket set, the test binary prints the lines the test sends. The fake knows
// neither the collector's state nor the expected metrics.
func TestMain(m *testing.M) {
	if socket := os.Getenv(fakeSocketEnv); socket != "" {
		os.Exit(runFakeTegrastats(socket))
	}
	os.Exit(m.Run())
}

func runFakeTegrastats(socket string) int {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return 121
	}
	defer conn.Close()
	if os.Getenv(fakeChildEnv) == "1" {
		// A spawned descendant holds its connection until it is terminated.
		_, _ = io.Copy(io.Discard, conn)
		return 0
	}
	if len(os.Args) != 3 || os.Args[1] != "--interval" || os.Args[2] != "1000" {
		return 122
	}
	decoder := json.NewDecoder(conn)
	for {
		var cmd fakeCommand
		if err := decoder.Decode(&cmd); err != nil {
			return 0
		}
		if cmd.Exit != nil {
			return *cmd.Exit
		}
		if cmd.Spawn {
			child := exec.Command(os.Args[0])
			child.Env = append(os.Environ(), fakeChildEnv+"=1")
			if child.Start() != nil {
				return 123
			}
		}
		if cmd.Line != "" {
			_, _ = fmt.Fprintln(os.Stdout, cmd.Line)
		}
	}
}

// fakeTegrastats accepts the connection of every fake process the collector starts.
type fakeTegrastats struct {
	listener *net.UnixListener
}

// newCollectorWithFake returns a collector whose tegrastats is this test binary,
// started through a pass-through nd-run.
func newCollectorWithFake(t *testing.T) (*Collector, *fakeTegrastats) {
	t.Helper()
	// Unix socket names must fit the platform's sockaddr_un path limit.
	dir, err := os.MkdirTemp("", "jetson-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "fake.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	t.Setenv(fakeSocketEnv, socket)

	wrapper := filepath.Join(dir, "nd-run")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$@\"\n"), 0700))
	t.Cleanup(ndexec.SetRunnerPathsForTests(wrapper, ""))

	binary, err := os.Executable()
	require.NoError(t, err)
	c := New()
	c.findTegrastats = func() (string, error) { return binary, nil }
	return c, &fakeTegrastats{
		listener: listener,
	}
}

// accept returns the connection of the next fake process the collector starts.
func (f *fakeTegrastats) accept(t *testing.T) net.Conn {
	t.Helper()
	require.NoError(t, f.listener.SetDeadline(time.Now().Add(5*time.Second)))
	conn, err := f.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func sendCommand(t *testing.T, conn net.Conn, cmd fakeCommand) {
	t.Helper()
	require.NoError(t, json.NewEncoder(conn).Encode(cmd))
}

// sendRecord makes the fake print a complete record with the given readings.
func sendRecord(t *testing.T, conn net.Conn, readings string) {
	t.Helper()
	sendCommand(t, conn, fakeCommand{
		Line: testRecordPrefix + readings,
	})
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

// requireExited requires the fake process behind conn, and every descendant
// sharing it, to have exited.
func requireExited(t *testing.T, conn net.Conn) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err := conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

// waitSample waits until the collector holds a record matching match.
func waitSample(t *testing.T, c *Collector, match func(sample) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		obs := c.latest.Load()
		return obs != nil && match(obs.sample)
	}, 3*time.Second, 5*time.Millisecond)
}
