// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestAcquisitionAdmission(t *testing.T) {
	a := NewAcquisition(t.Name(), "job")
	const n = 24
	var wg sync.WaitGroup
	leases := make(chan *acquisitionLease, n)
	for range n {
		wg.Go(func() {
			l, err := NewAcquisition(t.Name(), "job").acquire()
			if err == nil {
				leases <- l
			} else {
				require.ErrorIs(t, err, ErrAcquisitionBusy)
			}
		})
	}
	wg.Wait()
	close(leases)
	require.Len(t, leases, 1)
	for l := range leases {
		l.release()
	}
	l, err := a.acquire()
	require.NoError(t, err)
	defer l.release()
	for _, owner := range []*Acquisition{NewAcquisition(t.Name(), ""), NewAcquisition(t.Name(), "anonymous"), NewAcquisition(t.Name(), "renamed"), NewAcquisition(t.Name()+"other", "job")} {
		l, err := owner.acquire()
		require.NoError(t, err)
		defer l.release()
	}
	_, err = NewAcquisition(t.Name(), "").acquire()
	require.ErrorIs(t, err, ErrAcquisitionBusy)
}

func fakeSupervisedHelper(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ndsudo")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+text), 0700))
	return path
}

func TestSupervisedUnavailableAndUnverified(t *testing.T) {
	tests := map[string]struct {
		script         string
		safe, released bool
	}{
		"missing-helper":          {safe: true, released: true},
		"refused":                 {script: "printf 'NDSUP1 refused privilege\\n' >&4\nexit 126\n", safe: true, released: true},
		"old-helper-stdout-spoof": {script: "printf 'NDSUP1 refused privilege\\n'\nexit 126\n"},
		"wrong-exit":              {script: "printf 'NDSUP1 refused missing\\n' >&4\nexit 0\n"},
		"extra-data":              {script: "printf 'NDSUP1 refused missing\\nextra\\n' >&4\nexit 126\n"},
		"unsafe":                  {script: "printf 'NDSUP1 refused unsafe\\n' >&4\nexit 126\n", released: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "absent")
			if tc.script != "" {
				path = fakeSupervisedHelper(t, tc.script)
			}
			a := NewAcquisition(t.Name(), "job")
			l, err := a.acquire()
			require.NoError(t, err)
			p, err := startSupervisedNDSudo(context.Background(), ProcessOptions{}, path, "tegrastats", l)
			require.Nil(t, p)
			require.Error(t, err)
			require.Equal(t, tc.safe, errors.Is(err, ErrSupervisedUnavailable))
			if tc.released {
				l, err := a.acquire()
				require.NoError(t, err)
				l.release()
			} else {
				_, err := a.acquire()
				require.ErrorIs(t, err, ErrAcquisitionBusy)
			}
		})
	}
}

func TestSupervisedStartupCancellation(t *testing.T) {
	helper := fakeSupervisedHelper(t, "read ignored <&3\nexit 0\n")
	a := NewAcquisition(t.Name(), "job")
	l, err := a.acquire()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	before := time.Now()
	p, err := startSupervisedNDSudo(ctx, ProcessOptions{}, helper, "tegrastats", l)
	require.Nil(t, p)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(before), time.Second)
	_, err = a.acquire()
	require.ErrorIs(t, err, ErrAcquisitionBusy)
}

func TestSupervisedLostTerminal(t *testing.T) {
	helper := fakeSupervisedHelper(t, "printf 'NDSUP1 started\\n' >&4\nread ignored <&3\nexit 0\n")
	a := NewAcquisition(t.Name(), "job")
	l, err := a.acquire()
	require.NoError(t, err)
	p, err := startSupervisedNDSudo(context.Background(), ProcessOptions{}, helper, "tegrastats", l)
	require.NoError(t, err)
	require.ErrorIs(t, p.Close(), ErrSupervisedNotDrained)
	_, err = NewAcquisition(t.Name(), "job").StartUnprivileged(context.Background(), ProcessOptions{}, "/bin/true")
	require.ErrorIs(t, err, ErrAcquisitionBusy)
}

// Opt in only inside a disposable root Linux container. This suite installs its
// own ELF at the production's fixed path, refusing to replace an existing file.
// Every client runs as UID/GID 65534 through the actual setuid-root ndsudo.
func TestSupervisedPrivilegeBoundary(t *testing.T) {
	if os.Getenv("NETDATA_TEST_NDSUDO_SUPERVISION") != "1" {
		t.Skip("requires explicit disposable-container opt-in")
	}
	require.Zero(t, os.Geteuid())
	_, err := os.Stat("/.dockerenv")
	require.NoError(t, err, "run only in a disposable Docker container")
	_, err = os.Lstat("/usr/bin/tegrastats")
	require.True(t, os.IsNotExist(err), "never replace an existing vendor executable")
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../../"))
	build, err := os.MkdirTemp("", "ndsudo-boundary-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(build) })
	require.NoError(t, os.Chmod(build, 0755))
	helper := filepath.Join(build, "ndsudo")
	compile := func(args ...string) {
		t.Helper()
		data, err := exec.Command("cc", args...).CombinedOutput()
		require.NoError(t, err, "%s", data)
	}
	compile("-std=gnu11", "-Wall", "-Wextra", "-Werror", filepath.Join(root, "src/collectors/utils/ndsudo.c"), filepath.Join(root, "src/collectors/utils/ndsudo-supervisor.c"), "-o", helper)
	require.NoError(t, os.Chmod(helper, 0755|os.ModeSetuid))
	modePath := filepath.Join(build, "mode")
	probe := filepath.Join(build, "probe")
	compile("-std=gnu11", "-Wall", "-Wextra", "-Werror", "-DPROBE_MODE_FILE="+fmt.Sprintf("%q", modePath), filepath.Join(filepath.Dir(source), "testdata/supervised_probe.c"), "-o", probe)
	probeData, err := os.ReadFile(probe)
	require.NoError(t, err)
	target, err := os.OpenFile("/usr/bin/tegrastats", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	require.NoError(t, err)
	require.NoError(t, target.Close())
	t.Cleanup(func() { _ = os.Remove("/usr/bin/tegrastats") })
	exe, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(exe)
	require.NoError(t, err)
	client := filepath.Join(build, "client")
	require.NoError(t, os.WriteFile(client, data, 0755))
	forward := filepath.Join(build, "nd-run")
	require.NoError(t, os.WriteFile(forward, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755))
	for _, mode := range []string{"silent", "blocked", "group", "adopted", "escaped", "spoof", "exit", "loss", "missing", "privilege", "unsafe", "symlink", "exec-failed", "script", "owner", "directory", "arguments", "status-loss", "parent-loss", "signal", "opath-control", "opath-status"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, os.WriteFile(modePath, []byte(mode), 0644))
			require.NoError(t, os.WriteFile("/usr/bin/tegrastats", probeData, 0755))
			require.NoError(t, os.Chmod("/usr/bin/tegrastats", 0755))
			require.NoError(t, os.Chown("/usr/bin/tegrastats", 0, 0))
			switch mode {
			case "missing":
				require.NoError(t, os.Remove("/usr/bin/tegrastats"))
			case "privilege":
				require.NoError(t, os.Chmod(helper, 0755))
			case "unsafe":
				require.NoError(t, os.Chmod("/usr/bin/tegrastats", 0777))
			case "symlink":
				require.NoError(t, os.Remove("/usr/bin/tegrastats"))
				require.NoError(t, os.Symlink(probe, "/usr/bin/tegrastats"))
			case "script":
				require.NoError(t, os.WriteFile("/usr/bin/tegrastats", []byte("#!/bin/sh\nexit 0\n"), 0755))
			case "owner":
				require.NoError(t, os.Chown("/usr/bin/tegrastats", 65534, 65534))
			case "directory":
				info, err := os.Stat("/usr/bin")
				require.NoError(t, err)
				require.NoError(t, os.Chmod("/usr/bin", info.Mode().Perm()|0002))
				defer func() { require.NoError(t, os.Chmod("/usr/bin", info.Mode().Perm())) }()
			case "exec-failed":
				require.NoError(t, os.WriteFile("/usr/bin/tegrastats", []byte("\x7fELFmalformed"), 0755))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, client, "-test.run=^TestSupervisedBoundaryClient$", "-test.v")
			cmd.Env = append(os.Environ(), "NDSUDO_BOUNDARY_HELPER="+helper, "NDSUDO_BOUNDARY_MODE="+mode, "NDSUDO_BOUNDARY_FORWARD="+forward)
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			if mode == "parent-loss" {
				for _, line := range strings.Split(string(output), "\n") {
					if !strings.HasPrefix(line, "owned ") {
						continue
					}
					var pid int
					_, err := fmt.Sscanf(line, "owned %d", &pid)
					require.NoError(t, err)
					require.Eventually(t, func() bool { return errors.Is(unix.Kill(pid, 0), unix.ESRCH) }, 3*time.Second, 10*time.Millisecond)
				}
				require.Contains(t, string(output), "owned ")
			}
			require.NoError(t, os.Chmod(helper, 0755|os.ModeSetuid))
			if mode == "symlink" {
				require.NoError(t, os.Remove("/usr/bin/tegrastats"))
			}
		})
	}
}

func TestSupervisedBoundaryClient(t *testing.T) {
	helper := os.Getenv("NDSUDO_BOUNDARY_HELPER")
	if helper == "" {
		t.Skip("launched by boundary driver")
	}
	require.Equal(t, 65534, os.Geteuid())
	defer SetRunnerPathsForTests(os.Getenv("NDSUDO_BOUNDARY_FORWARD"), helper)()
	mode := os.Getenv("NDSUDO_BOUNDARY_MODE")
	if mode == "arguments" || mode == "status-loss" || strings.HasPrefix(mode, "opath-") {
		rawSupervisedBoundary(t, helper, mode)
		return
	}
	a := NewAcquisition("boundary", "job")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, write, err := os.Pipe()
	require.NoError(t, err)
	defer read.Close()
	p, err := a.StartNDSudo(ctx, ProcessOptions{Stdout: write}, "tegrastats")
	require.NoError(t, write.Close())
	switch mode {
	case "missing", "privilege", "unsafe", "symlink", "exec-failed", "script", "owner", "directory":
		require.Nil(t, p)
		require.Error(t, err)
		require.Equal(t, mode == "missing" || mode == "privilege", errors.Is(err, ErrSupervisedUnavailable))
		fallback, err := NewAcquisition("boundary", "job").StartUnprivileged(ctx, ProcessOptions{}, "/bin/true")
		require.NoError(t, err)
		require.NoError(t, fallback.Wait())
		return
	}
	require.NoError(t, err)
	require.NoError(t, read.SetReadDeadline(time.Now().Add(5*time.Second)))
	line, err := bufio.NewReader(read).ReadString('\n')
	require.NoError(t, err)
	var pid, parent, uid, gid, extra, env, leaf int
	var cwd string
	n, err := fmt.Sscan(line, &pid, &parent, &uid, &gid, &extra, &env, &cwd, &leaf)
	require.NoError(t, err)
	require.Equal(t, 8, n)
	require.Zero(t, uid)
	require.Zero(t, gid)
	require.Zero(t, extra)
	require.Zero(t, env)
	require.Equal(t, "/", cwd)
	if mode == "parent-loss" {
		fmt.Printf("owned %d\n", pid)
		os.Exit(0) // Simulate plugin loss, without invoking cancel/Close or defers.
	}
	if mode == "signal" {
		require.Error(t, p.Wait())
		require.ErrorIs(t, unix.Kill(pid, 0), unix.ESRCH)
		return
	}
	if mode == "exit" {
		require.NoError(t, p.Wait())
		return
	}
	require.ErrorIs(t, unix.Kill(pid, unix.SIGKILL), unix.EPERM, "must cross a real privilege boundary")
	_, err = NewAcquisition("boundary", "job").StartUnprivileged(ctx, ProcessOptions{}, "/bin/true")
	require.ErrorIs(t, err, ErrAcquisitionBusy)
	if mode == "loss" {
		require.ErrorIs(t, p.Wait(), ErrSupervisedNotDrained)
		_, err = NewAcquisition("boundary", "job").StartNDSudo(ctx, ProcessOptions{}, "tegrastats")
		require.ErrorIs(t, err, ErrAcquisitionBusy)
		require.Eventually(t, func() bool {
			b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			return os.IsNotExist(err) || strings.Contains(string(b), ") Z ")
		}, 3*time.Second, 10*time.Millisecond)
		return
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	if mode == "escaped" {
		select {
		case err := <-done:
			t.Fatalf("escaped descendant falsely drained: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		_, err = NewAcquisition("boundary", "job").StartUnprivileged(context.Background(), ProcessOptions{}, "/bin/true")
		require.ErrorIs(t, err, ErrAcquisitionBusy)
	}
	select {
	case err := <-done:
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrSupervisedNotDrained)
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not finish cleanup")
	}
	require.ErrorIs(t, unix.Kill(pid, 0), unix.ESRCH)
	if leaf != 0 {
		require.ErrorIs(t, unix.Kill(leaf, 0), unix.ESRCH)
	}
	next, err := NewAcquisition("boundary", "job").StartUnprivileged(context.Background(), ProcessOptions{}, "/bin/true")
	require.NoError(t, err)
	require.NoError(t, next.Wait())
}

// Exercise the private C entry without the Go adapter, including descriptor loss.
func rawSupervisedBoundary(t *testing.T, helper, mode string) {
	cr, cw, err := os.Pipe()
	require.NoError(t, err)
	defer cw.Close()
	sr, sw, err := os.Pipe()
	require.NoError(t, err)
	defer sr.Close()
	if strings.HasPrefix(mode, "opath-") {
		endpoint := &cr
		if mode == "opath-status" {
			endpoint = &sw
		}
		fd, err := unix.Open(fmt.Sprintf("/proc/self/fd/%d", (*endpoint).Fd()), unix.O_PATH|unix.O_CLOEXEC, 0)
		require.NoError(t, err)
		require.NoError(t, (*endpoint).Close())
		*endpoint = os.NewFile(uintptr(fd), "opath-endpoint")
	}

	out, writer, err := os.Pipe()
	require.NoError(t, err)
	defer out.Close()
	args := []string{"tegrastats"}
	if mode == "arguments" {
		args = append(args, "--interval", "10")
	}
	cmd := exec.Command(helper, args...)
	cmd.ExtraFiles = []*os.File{cr, sw}
	cmd.Stdout = writer
	require.NoError(t, cmd.Start())
	require.NoError(t, cr.Close())
	require.NoError(t, sw.Close())
	require.NoError(t, writer.Close())
	require.NoError(t, sr.SetReadDeadline(time.Now().Add(3*time.Second)))
	if strings.HasPrefix(mode, "opath-") {
		data, err := io.ReadAll(sr)
		require.NoError(t, err)
		require.Error(t, cmd.Wait())
		require.Equal(t, 126, cmd.ProcessState.ExitCode())
		require.Empty(t, data, "invalid pipes must be rejected before payload startup")
		data, err = io.ReadAll(out)
		require.NoError(t, err)
		require.Empty(t, data, "invalid pipes must not execute the payload")
		return
	}

	if mode == "arguments" {
		data, err := io.ReadAll(sr)
		require.NoError(t, err)
		require.Equal(t, "NDSUP1 refused arguments\n", string(data))
		require.Error(t, cmd.Wait())
		return
	}
	first, err := bufio.NewReader(sr).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "NDSUP1 started\n", first)
	require.NoError(t, out.SetReadDeadline(time.Now().Add(3*time.Second)))
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	var pid int
	_, err = fmt.Sscan(line, &pid)
	require.NoError(t, err)
	require.NoError(t, sr.Close())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("status loss did not cancel payload")
	}
	require.ErrorIs(t, unix.Kill(pid, 0), unix.ESRCH)
}

func TestAcquisitionConcurrentStartsAndIndependentOwners(t *testing.T) {
	helper := fakeSupervisedHelper(t, "printf 'NDSUP1 started\\n' >&4\nread ignored <&3\nprintf 'NDSUP1 cleaned 0\\n' >&4\nexit 0\n")
	defer SetRunnerPathsForTests("", helper)()
	const n = 16
	winners := make(chan *Process, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			p, err := NewAcquisition(t.Name(), "job").StartNDSudo(context.Background(), ProcessOptions{}, "tegrastats")
			if err == nil {
				winners <- p
			} else {
				require.ErrorIs(t, err, ErrAcquisitionBusy)
			}
		})
	}
	wg.Wait()
	close(winners)
	require.Len(t, winners, 1)
	for _, name := range []string{"", "anonymous", "renamed"} {
		p, err := NewAcquisition(t.Name(), name).StartNDSudo(context.Background(), ProcessOptions{}, "tegrastats")
		require.NoError(t, err)
		require.NoError(t, p.Close())
	}
	for p := range winners {
		require.NoError(t, p.Close())
	}
}
