// SPDX-License-Identifier: GPL-3.0-or-later

package privileged

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestDiscoverNativeBoundary(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(content), 0644))
	}
	link := func(path, target string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755))
		require.NoError(t, os.Symlink(target, filepath.Join(root, path)))
	}
	write("sys/kernel/random/boot_id", "11111111-1111-1111-1111-111111111111\n")
	write("42/stat", "42 (java) S "+strings.Repeat("0 ", 18)+"12345 0")
	write("42/status", "Uid: 1001 1001 1001 1001\nGid: 1002 1002 1002 1002\n")
	write("42/cmdline", "java\x00-jar\x00/private/orders.jar\x00--password=not-returned\x00")
	link("42/exe", "/usr/bin/java")
	for _, ns := range []string{"pid", "user", "net"} {
		write("self/ns/"+ns, "")
		link("42/ns/"+ns, filepath.Join(root, "self/ns", ns))
	}
	link("self/root", root)
	link("42/root", root)
	// Mount namespace alone is deliberately irrelevant (systemd PrivateTmp).
	write("self/ns/mnt", "")
	write("42/ns/mnt", "")
	found, err := discover(root)
	require.NoError(t, err)
	require.Len(t, found.Processes, 1)
	p := found.Processes[0]
	require.True(t, p.Eligible)
	require.Equal(t, "orders.jar", p.Application)
	require.Equal(t, "11111111-1111-1111-1111-111111111111-42:12345", p.Instance())
	for _, ns := range []string{"pid", "user", "net"} {
		require.NoError(t, os.Remove(filepath.Join(root, "42/ns", ns)))
		write("42/ns/"+ns, "")
		p, err := inspect(root, 42, p.BootID)
		require.NoError(t, err)
		require.False(t, p.Eligible)
		require.Contains(t, p.Reason, ns)
		require.NoError(t, os.Remove(filepath.Join(root, "42/ns", ns)))
		link("42/ns/"+ns, filepath.Join(root, "self/ns", ns))
	}
	require.NoError(t, os.Remove(filepath.Join(root, "42/root")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "different-root"), 0755))
	link("42/root", filepath.Join(root, "different-root"))
	different, err := inspect(root, 42, p.BootID)
	require.NoError(t, err)
	require.False(t, different.Eligible)
	require.Contains(t, different.Reason, "root filesystem")
	require.NoError(t, os.Remove(filepath.Join(root, "42/root")))
	link("42/root", root)
	write("42/status", "Uid: 0 1001 0 1001\nGid: 1002 1002 1002 1002\n")
	mixed, err := inspect(root, 42, p.BootID)
	require.NoError(t, err)
	require.False(t, mixed.Eligible)
	require.EqualValues(t, 1001, mixed.UID)
	require.Contains(t, mixed.Reason, "Mixed process credentials")
	write("42/status", "Uid: 0 0 0 0\nGid: 0 0 0 0\n")
	p2, err := inspect(root, 42, p.BootID)
	require.NoError(t, err)
	require.False(t, p2.Eligible)
	require.Contains(t, p2.Reason, "Root-owned")
}

func TestAttachRequestValidation(t *testing.T) {
	req := protocol.AttachRequest{PID: 42, StartTime: 1, BootID: "11111111-1111-1111-1111-111111111111", Token: strings.Repeat("ab", 32), Port: 12345}
	require.NoError(t, validateRequest(req))
	req.Token = "x;otel.javaagent.extensions=/bad"
	require.Error(t, validateRequest(req))
	require.Error(t, decode(strings.NewReader(`{"pid":42,"path":"/bad"}`), &req))
	require.Error(t, decode(strings.NewReader(`{} {}`), &req))
	req.Token = strings.Repeat("ab", 32)
	req.Port = 80
	require.Error(t, validateRequest(req))
}

func TestSupervisorDeadlineReaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	started := time.Now()
	require.ErrorIs(t, supervise(ctx, cmd), context.DeadlineExceeded)
	require.Less(t, time.Since(started), 3*time.Second)
	require.NotNil(t, cmd.ProcessState)
	require.ErrorIs(t, syscall.Kill(cmd.Process.Pid, 0), syscall.ESRCH)
}

func TestWorkerPrivilegesDropped(t *testing.T) {
	if os.Getenv("NETDATA_JAVA_PRIVILEGE_PROBE") == "1" {
		if err := workerPrivileges(); err != nil {
			os.Exit(23)
		}
		os.Exit(0)
	}
	if os.Getuid() != 0 {
		t.Skip("real credential-drop test requires root")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	require.NoError(t, unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0))
	cmd := exec.Command("/proc/self/exe", "-test.run=^TestWorkerPrivilegesDropped$")
	cmd.Env = []string{"NETDATA_JAVA_PRIVILEGE_PROBE=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534, Groups: []uint32{}}}
	require.NoError(t, cmd.Run())
}

func TestSupervisorWithoutKillCapability(t *testing.T) {
	if os.Getenv("NETDATA_JAVA_KILL_PROBE") == "1" {
		runtime.LockOSThread()
		header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
		var caps [2]unix.CapUserData
		if unix.Capget(&header, &caps[0]) != nil {
			os.Exit(21)
		}
		caps[0].Effective &^= 1 << unix.CAP_KILL
		caps[0].Permitted &^= 1 << unix.CAP_KILL
		if unix.Capset(&header, &caps[0]) != nil {
			os.Exit(22)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		cmd := exec.Command("/bin/sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: &syscall.Credential{Uid: 65534, Gid: 65534, Groups: []uint32{}}}
		if supervise(ctx, cmd) != context.DeadlineExceeded || cmd.ProcessState == nil || os.Geteuid() != 0 {
			os.Exit(23)
		}
		os.Exit(0)
	}
	if os.Getuid() != 0 {
		t.Skip("real supervisor capability test requires root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/proc/self/exe", "-test.run=^TestSupervisorWithoutKillCapability$")
	cmd.Env = []string{"NETDATA_JAVA_KILL_PROBE=1"}
	require.NoError(t, cmd.Run())
}

func TestAttachInputDeadline(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	var req protocol.AttachRequest
	started := time.Now()
	require.Error(t, decodeWithTimeout(reader, &req, 50*time.Millisecond))
	require.Less(t, time.Since(started), time.Second)
}
