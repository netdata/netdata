// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

// Package streamexectest provides a fake streaming command for tests of streamexec consumers.
//
// The test binary doubles as the fake: a test package calls RunIfFake first in TestMain, and a consumer under test
// starts Fake.Binary through ndexec. Each started fake connects back to the test over a Unix socket, reports its
// arguments, and then prints, spawns or exits as the test instructs. The fake knows nothing about the consumer.
//
// A fake's connection closes only when its process exits, so end of connection means the process has gone.
package streamexectest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const (
	socketEnv = "NETDATA_STREAMEXEC_TEST_SOCKET"
	childEnv  = "NETDATA_STREAMEXEC_TEST_CHILD"

	waitTimeout = 5 * time.Second
	// exitedNow is how long RequireExited waits for an end of connection that has already happened.
	exitedNow = 10 * time.Millisecond
	// noiseInterval is the period of Noise output.
	noiseInterval = 50 * time.Millisecond
)

// hello is the first message of every fake connection.
type hello struct {
	Args  []string
	Child bool
}

// command is one instruction from a test to a running fake.
type command struct {
	Raw   string // printed as is
	Noise string // printed with a newline every noiseInterval until the fake exits
	Exit  *int
	Spawn bool
}

// RunIfFake makes the test binary act as the fake when a Fake started it, and never returns in that case.
// Call it first in TestMain.
func RunIfFake() {
	if socket := os.Getenv(socketEnv); socket != "" {
		// syscall.Exit skips the race detector's exit delay, which would keep a fake exiting with status 0 alive for
		// about a second, and leaves the connection to close with the process.
		syscall.Exit(runFake(socket))
	}
}

func runFake(socket string) int {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return 121
	}
	child := os.Getenv(childEnv) == "1"
	if err := json.NewEncoder(conn).Encode(hello{
		Args:  os.Args[1:],
		Child: child,
	}); err != nil {
		return 122
	}
	if child {
		// A spawned descendant holds its connection until it is terminated or the test closes it.
		_, _ = io.Copy(io.Discard, conn)
		return 0
	}
	decoder := json.NewDecoder(conn)
	for {
		var cmd command
		if err := decoder.Decode(&cmd); err != nil {
			return 0
		}
		if cmd.Exit != nil {
			return *cmd.Exit
		}
		if cmd.Spawn {
			spawned := exec.Command(os.Args[0])
			spawned.Env = append(os.Environ(), childEnv+"=1")
			if spawned.Start() != nil {
				return 123
			}
		}
		if cmd.Raw != "" {
			_, _ = fmt.Fprint(os.Stdout, cmd.Raw)
		}
		if cmd.Noise != "" {
			go func() {
				for {
					_, _ = fmt.Fprintln(os.Stdout, cmd.Noise)
					time.Sleep(noiseInterval)
				}
			}()
		}
	}
}

// Fake accepts the connection of every fake the consumer under test starts.
type Fake struct {
	// Binary is the test binary, which acts as the fake command.
	Binary string

	listener *net.UnixListener
}

// NewFake prepares the test binary to act as the fake and routes ndexec's unprivileged starts through a
// pass-through nd-run for the rest of the test. It sets process-wide state, so tests using it MUST NOT run in
// parallel.
func NewFake(t *testing.T) *Fake {
	t.Helper()
	// Unix socket names must fit the platform's sockaddr_un path limit, which t.TempDir paths can exceed.
	dir, err := os.MkdirTemp("", "streamexec-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "fake.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	t.Setenv(socketEnv, socket)

	wrapper := filepath.Join(dir, "nd-run")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$@\"\n"), 0700))
	t.Cleanup(ndexec.SetRunnerPathsForTests(wrapper, ""))

	binary, err := os.Executable()
	require.NoError(t, err)
	return &Fake{
		Binary:   binary,
		listener: listener,
	}
}

// Accept returns the connection of the next fake that starts.
func (f *Fake) Accept(t *testing.T) *Conn {
	t.Helper()
	require.NoError(t, f.listener.SetDeadline(time.Now().Add(waitTimeout)))
	conn, err := f.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitTimeout)))
	var h hello
	require.NoError(t, json.NewDecoder(conn).Decode(&h))
	require.NoError(t, conn.SetReadDeadline(time.Time{}))
	return &Conn{
		Args:  h.Args,
		Child: h.Child,
		conn:  conn,
	}
}

// Conn controls one running fake.
type Conn struct {
	// Args are the arguments the fake was started with.
	Args []string
	// Child reports a descendant spawned by another fake.
	Child bool

	conn net.Conn
}

// Line makes the fake print line and a newline.
func (c *Conn) Line(t *testing.T, line string) {
	t.Helper()
	c.send(t, command{
		Raw: line + "\n",
	})
}

// Raw makes the fake print data as is.
func (c *Conn) Raw(t *testing.T, data string) {
	t.Helper()
	c.send(t, command{
		Raw: data,
	})
}

// Noise makes the fake print line and a newline repeatedly, many times per second, until it exits.
func (c *Conn) Noise(t *testing.T, line string) {
	t.Helper()
	c.send(t, command{
		Noise: line,
	})
}

// Exit makes the fake exit with code.
func (c *Conn) Exit(t *testing.T, code int) {
	t.Helper()
	c.send(t, command{
		Exit: &code,
	})
}

// Spawn makes the fake start a descendant, whose connection the next Accept returns.
func (c *Conn) Spawn(t *testing.T) {
	t.Helper()
	c.send(t, command{
		Spawn: true,
	})
}

// RequireExited requires the fake to have already exited, for checking that something happened after its exit.
func (c *Conn) RequireExited(t *testing.T) {
	t.Helper()
	c.requireEOF(t, exitedNow)
}

// WaitExited waits for the fake to exit.
func (c *Conn) WaitExited(t *testing.T) {
	t.Helper()
	c.requireEOF(t, waitTimeout)
}

func (c *Conn) requireEOF(t *testing.T, timeout time.Duration) {
	t.Helper()
	require.NoError(t, c.conn.SetReadDeadline(time.Now().Add(timeout)))
	_, err := c.conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func (c *Conn) send(t *testing.T, cmd command) {
	t.Helper()
	require.NoError(t, json.NewEncoder(c.conn).Encode(cmd))
}
