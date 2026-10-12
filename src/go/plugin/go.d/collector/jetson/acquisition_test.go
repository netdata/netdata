// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// This peer exercises the collector through the real ndexec protocol. It does not
// elevate privileges or stand in for ndsudo's separate setuid boundary tests.
func runIfSupervisedPeer() {
	dir := os.Getenv("NETDATA_JETSON_TEST_SUPERVISOR")
	if dir == "" || len(os.Args) != 2 || os.Args[1] != "tegrastats" {
		return
	}
	syscall.Exit(runSupervisedPeer(dir))
}

func runSupervisedPeer(dir string) int {
	f, err := os.OpenFile(filepath.Join(dir, "attempts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return 125
	}
	_, _ = fmt.Fprintln(f, "start")
	_ = f.Close()
	data, err := os.ReadFile(filepath.Join(dir, "mode"))
	if err != nil {
		return 125
	}
	mode := string(data)
	control := os.NewFile(3, "control")
	status := os.NewFile(4, "status")
	stopped := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, control); close(stopped) }()
	switch mode {
	case "privilege", "missing", "unsafe", "setup":
		_, _ = fmt.Fprintf(status, "NDSUP1 refused %s\n", mode)
		return 126
	case "invalid":
		_, _ = fmt.Fprintln(status, "invalid protocol")
		return 126
	case "failed":
		_, _ = fmt.Fprint(status, "NDSUP1 failed\nNDSUP1 cleaned 256\n")
		return 1
	case "cancel":
		<-stopped
		_, _ = fmt.Fprintln(status, "NDSUP1 refused privilege")
		return 126
	}
	child := exec.Command(os.Args[0], "--interval", "1000")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		return 125
	}
	_, _ = fmt.Fprintln(status, "NDSUP1 started")
	done := make(chan struct{})
	go func() { _ = child.Wait(); close(done) }()
	select {
	case <-stopped:
		_ = child.Process.Kill()
		<-done
	case <-done:
	}
	if mode == "hold" {
		// Model late verified cleanup, bounded even if the parent test fails.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	raw := child.ProcessState.Sys().(syscall.WaitStatus)
	_, _ = fmt.Fprintf(status, "NDSUP1 cleaned %d\n", raw)
	if raw.Signaled() {
		return 128 + int(raw.Signal())
	}
	return raw.ExitStatus()
}

type supervisedPeer struct{ dir string }

func newSupervisedPeer(t *testing.T, mode string) (*supervisedPeer, *streamexectest.Fake) {
	t.Helper()
	fake := streamexectest.NewFake(t)
	peer := &supervisedPeer{dir: t.TempDir()}
	peer.setMode(t, mode)
	t.Setenv("NETDATA_JETSON_TEST_SUPERVISOR", peer.dir)
	t.Cleanup(ndexec.SetRunnerPathsForTests("", fake.Binary))
	t.Cleanup(func() { peer.release(t) })
	return peer, fake
}
func (p *supervisedPeer) setMode(t *testing.T, mode string) {
	t.Helper()
	// Rename prevents a restarting helper from observing a truncated mode file.
	next := filepath.Join(p.dir, "mode.next")
	require.NoError(t, os.WriteFile(next, []byte(mode), 0600))
	require.NoError(t, os.Rename(next, filepath.Join(p.dir, "mode")))
}
func (p *supervisedPeer) attempts() int {
	data, _ := os.ReadFile(filepath.Join(p.dir, "attempts"))
	return strings.Count(string(data), "start\n")
}
func (p *supervisedPeer) release(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "release"), nil, 0600))
}
func policyCollector(t *testing.T, fake *streamexectest.Fake) *Collector {
	t.Helper()
	c := New()
	// Unverified-cleanup cases deliberately retain ownership for this process.
	// Keep their keys distinct when tests are repeated with -count.
	c.Name = filepath.Join(os.Getenv("NETDATA_JETSON_TEST_SUPERVISOR"), t.Name())
	c.findTegrastats = func() (string, error) { return fake.Binary, nil }
	c.timing.RestartDelayMin, c.timing.RestartDelayMax = 10*time.Millisecond, 20*time.Millisecond
	return c
}
func requireStartError(t *testing.T, c *Collector, target error, message string) {
	t.Helper()
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready, done := goRun(ctx, c)
	select {
	case err := <-done:
		if target != nil {
			require.ErrorIs(t, err, target)
		} else {
			require.ErrorContains(t, err, message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup failure not returned")
	}
	select {
	case <-ready:
		t.Fatal("failed producer reported ready")
	default:
	}
}

func TestPrivilegedFixedPathOutsidePATH(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "success")
	c := policyCollector(t, fake)
	c.findTegrastats = func() (string, error) { return "", errors.New("no PATH candidate") }
	c.fixedTegrastatsExists = func() bool { return true }
	stop := startRun(t, c)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"--interval", "1000"}, conn.Args)
	assert.Equal(t, 1, peer.attempts())
	// Readiness is independent of telemetry; missing fields do not select fallback.
	sendRecord(t, conn, "GR3D_FREQ 0%")
	waitSample(t, c, func(s sample) bool { return s.GPUUtilization != nil })
	stop()
	conn.RequireExited(t)
}

func TestSafeInitialFallbackIsSticky(t *testing.T) {
	for _, mode := range []string{"privilege", "missing", "missing-helper"} {
		t.Run(mode, func(t *testing.T) {
			peer, fake := newSupervisedPeer(t, mode)
			if mode == "missing-helper" {
				restore := ndexec.SetRunnerPathsForTests("", filepath.Join(t.TempDir(), "absent"))
				t.Cleanup(restore)
			}
			c := policyCollector(t, fake)
			logs := &lockedBuffer{}
			c.Logger = logger.NewWithWriter(logs)
			stop := startRun(t, c)
			first := fake.Accept(t)
			before := peer.attempts()
			// A repaired helper must not switch a running job's chosen route.
			peer.setMode(t, "success")
			first.Exit(t, 1)
			next := fake.Accept(t)
			sendRecord(t, next, "GR3D_FREQ 0%")
			waitSample(t, c, func(s sample) bool { return s.GPUUtilization != nil })
			stop()
			// Repeating Init/Check/Run on this object also retains the selection.
			stop = startRun(t, c)
			fake.Accept(t)
			stop()
			assert.Equal(t, before, peer.attempts())
			assert.Equal(t, 1, strings.Count(logs.String(), "using unprivileged collection"))
			// Reconstructing the job retries the privileged path.
			t.Cleanup(ndexec.SetRunnerPathsForTests("", fake.Binary))
			replacement := policyCollector(t, fake)
			stop = startRun(t, replacement)
			fake.Accept(t)
			stop()
			assert.Equal(t, before+1, peer.attempts())
		})
	}
}

func TestFallbackFailureRetainsSelection(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "privilege")
	c := policyCollector(t, fake)
	c.findTegrastats = func() (string, error) { return "", errors.New("no PATH candidate") }
	c.fixedTegrastatsExists = func() bool { return true }
	requireStartError(t, c, nil, "unprivileged tegrastats unavailable")
	peer.setMode(t, "success")
	// After repairing PATH, this instance still uses the selected unprivileged route.
	c.findTegrastats = func() (string, error) { return fake.Binary, nil }
	stop := startRun(t, c)
	fake.Accept(t)
	stop()
	assert.Equal(t, 1, peer.attempts())
}

func TestUnsafeStartupDoesNotFallback(t *testing.T) {
	for _, mode := range []string{"unsafe", "setup", "failed", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			peer, fake := newSupervisedPeer(t, mode)
			c := policyCollector(t, fake)
			requireStartError(t, c, nil, "start tegrastats")
			fake.RequireNoStart(t, 20*time.Millisecond)
			assert.Equal(t, 1, peer.attempts())
		})
	}
}

func TestPrivilegedRestartNeverFallsBack(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "success")
	c := policyCollector(t, fake)
	stop := startRun(t, c)
	first := fake.Accept(t)
	first.Exit(t, 1)
	second := fake.Accept(t)
	require.Equal(t, 2, peer.attempts())
	peer.setMode(t, "privilege")
	second.Exit(t, 1)
	require.Eventually(t, func() bool { return peer.attempts() >= 3 }, 3*time.Second, 5*time.Millisecond)
	fake.RequireNoStart(t, 100*time.Millisecond)
	stop()
	// Cancellation may precede completion of the last refused startup.
	waitAcquisitionReleased(t, c)
	// A new Run of the same object must not fall back either.
	requireStartError(t, c, ndexec.ErrSupervisedUnavailable, "")
	fake.RequireNoStart(t, 20*time.Millisecond)
}

func TestCancelDuringPrivilegedStartupDoesNotFallback(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "cancel")
	c := policyCollector(t, fake)
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready, done := goRun(ctx, c)
	require.Eventually(t, func() bool { return peer.attempts() == 1 }, 3*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation blocked")
	}
	select {
	case <-ready:
		t.Fatal("canceled startup reported ready")
	default:
	}
	fake.RequireNoStart(t, 20*time.Millisecond)
}

func TestPendingCleanupBlocksUnprivilegedReplacement(t *testing.T) {
	peer, fake := newSupervisedPeer(t, "hold")
	original := policyCollector(t, fake)
	original.timing.StallTimeout = 40 * time.Millisecond
	stop := startRun(t, original)
	conn := fake.Accept(t)
	stop() // Source's bounded wait ends before the helper's cleanup evidence.
	conn.RequireExited(t)

	replacement := policyCollector(t, fake)
	replacement.AllowPrivileged = false
	requireStartError(t, replacement, ndexec.ErrAcquisitionBusy, "")
	fake.RequireNoStart(t, 20*time.Millisecond)
	assert.Equal(t, 1, peer.attempts(), "disabled replacement must not probe the helper")
	peer.release(t)
	waitAcquisitionReleased(t, replacement)
	stop = startRun(t, replacement)
	fake.Accept(t)
	stop()
	assert.Equal(t, 1, peer.attempts())
}

func TestUnverifiedCleanupRetainsOwner(t *testing.T) {
	_, fake := newSupervisedPeer(t, "invalid")
	c := policyCollector(t, fake)
	requireStartError(t, c, nil, "startup protocol")
	c = policyCollector(t, fake)
	c.AllowPrivileged = false
	requireStartError(t, c, ndexec.ErrAcquisitionBusy, "")
	fake.RequireNoStart(t, 20*time.Millisecond)
}

func waitAcquisitionReleased(t *testing.T, c *Collector) {
	t.Helper()
	// Exercise public admission while waiting for the late terminal frame.
	require.Eventually(t, func() bool {
		p, err := c.acquisition.StartUnprivileged(t.Context(), ndexec.ProcessOptions{}, "/bin/true")
		if errors.Is(err, ndexec.ErrAcquisitionBusy) {
			return false
		}
		require.NoError(t, err)
		require.NoError(t, p.Wait())
		return true
	}, 3*time.Second, 5*time.Millisecond)
}
