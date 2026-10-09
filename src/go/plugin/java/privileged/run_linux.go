// SPDX-License-Identifier: GPL-3.0-or-later

package privileged

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
	"golang.org/x/sys/unix"
)

const attachDeadline = 25 * time.Second

type work struct {
	Request protocol.AttachRequest
	Process protocol.Process
}

func result(status, detail string) protocol.AttachResult {
	return protocol.AttachResult{Status: status, Detail: detail}
}

// Run exposes only fixed operations. The worker entry has no elevated privileges.
func Run(args []string, input io.Reader, output io.Writer) error {
	if len(args) != 1 {
		return errors.New("expected discover or attach")
	}
	if args[0] == "worker" {
		return runWorker(input, output)
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return errors.New("java-helper requires ndsudo root execution")
	}
	switch args[0] {
	case "discover":
		found, err := discover(procRoot)
		if err != nil {
			return errors.New("cannot discover Java processes")
		}
		return json.NewEncoder(output).Encode(found)
	case "attach":
		var req protocol.AttachRequest
		if err := decodeWithTimeout(input, &req, 5*time.Second); err != nil {
			return errors.New("invalid attach request")
		}
		if err := validateRequest(req); err != nil {
			return err
		}
		r := attach(req)
		return json.NewEncoder(output).Encode(r)
	default:
		return errors.New("unsupported operation")
	}
}

func decodeWithTimeout(input io.Reader, value any, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- decode(input, value) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("attach input deadline exceeded")
	}
}

func decode(input io.Reader, value any) error {
	// The fixed request contains a 64-character token and a process identity, not arbitrary payloads.
	d := json.NewDecoder(io.LimitReader(input, 4096))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing request data")
	}
	return nil
}

func validateRequest(req protocol.AttachRequest) error {
	token, err := hex.DecodeString(req.Token)
	if req.PID <= 0 || req.StartTime == 0 || req.Port < 1024 || len(req.BootID) != 36 || err != nil || len(token) != 32 {
		return errors.New("invalid attach identity, port or token")
	}
	return nil
}

func current(req protocol.AttachRequest) (*protocol.Process, error) {
	boot, err := bootID(procRoot)
	if err != nil || boot != req.BootID {
		return nil, errors.New("boot identity changed")
	}
	p, err := inspect(procRoot, req.PID, boot)
	if err != nil || p == nil || p.StartTime != req.StartTime {
		return nil, errors.New("process identity changed or cannot be inspected")
	}
	if !p.Eligible {
		return nil, errors.New(p.Reason)
	}
	return p, nil
}

func trusted(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return errors.New("bundled runtime is missing")
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&0022 != 0 || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("bundled runtime must be root-owned and not writable by other users")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func bundle() string { return filepath.Join(buildinfo.StockDataDir, "java") }

func attach(req protocol.AttachRequest) protocol.AttachResult {
	p, err := current(req)
	if err != nil {
		return result(protocol.Blocked, err.Error())
	}
	for _, name := range []string{"runtime/bin/java", "helper/NetdataAttach.class", "otel.jar", "hikari-extension.jar"} {
		if err := trusted(filepath.Join(bundle(), name)); err != nil {
			return result(protocol.Blocked, err.Error())
		}
	}
	payload, _ := json.Marshal(work{Request: req, Process: *p})
	cmd := exec.Command("/proc/self/exe", "worker")
	cmd.Env = []string{"LANG=C", "LC_ALL=C"}
	cmd.Dir = "/"
	cmd.Stdin = bytes.NewReader(payload)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: &syscall.Credential{Uid: p.UID, Gid: p.GID, Groups: []uint32{}}}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	// The fork/exec thread passes no_new_privs to every worker thread and the JVM.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var capabilities [2]unix.CapUserData
	if err := unix.Capget(&header, &capabilities[0]); err != nil {
		return result(protocol.Blocked, "Cannot inspect attachment capabilities")
	}
	capabilities[0].Inheritable, capabilities[1].Inheritable = 0, 0
	if err := unix.Capset(&header, &capabilities[0]); err != nil {
		return result(protocol.Blocked, "Cannot clear inherited capabilities")
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return result(protocol.Blocked, "Cannot restrict attachment privileges")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	ctx, deadline := context.WithTimeout(ctx, attachDeadline)
	defer deadline()
	if err := supervise(ctx, cmd); err != nil {
		return result(protocol.Unknown, "Attachment interrupted or failed before acknowledgement; no automatic retry")
	}
	var r protocol.AttachResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || (r.Status != protocol.Attached && r.Status != protocol.Blocked && r.Status != protocol.Unknown) {
		return result(protocol.Unknown, "Attachment outcome unavailable; no automatic retry")
	}
	return r
}

func supervise(ctx context.Context, cmd *exec.Cmd) error {
	// Adopt the helper JVM if the worker dies first, so cancellation leaves no
	// unreaped descendant for an unrelated init process to handle.
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	// Keep the leader unreaped until its process group has been killed. Reaping first
	// permits PID/PGID reuse and could target an unrelated process group.
	go func() {
		var info unix.Siginfo
		var err error
		for {
			err = unix.Waitid(unix.P_PID, cmd.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
			if err != unix.EINTR {
				break
			}
		}
		done <- err
	}()
	var cause error
	select {
	case cause = <-done:
	case <-ctx.Done():
		cause = ctx.Err()
		_ = killGroup(cmd)
		<-done
	}
	_ = killGroup(cmd)
	err := cmd.Wait()
	for {
		var status unix.WaitStatus
		_, reapErr := unix.Wait4(-cmd.Process.Pid, &status, 0, nil)
		if reapErr == unix.EINTR {
			continue
		}
		if reapErr != nil {
			break
		}
	}
	if cause != nil {
		return cause
	}
	return err
}

// netdata.service permits SETUID but does not require CAP_KILL. Signal the
// dropped-UID group as that UID, while the supervisor retains real/saved root.
func killGroup(cmd *exec.Cmd) error {
	if cmd.SysProcAttr.Credential == nil || os.Geteuid() != 0 {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	runtime.LockOSThread()
	uid := uintptr(cmd.SysProcAttr.Credential.Uid)
	_, _, err := syscall.RawSyscall(syscall.SYS_SETRESUID, ^uintptr(0), uid, ^uintptr(0))
	if err != 0 {
		runtime.UnlockOSThread()
		return err
	}
	killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_, _, err = syscall.RawSyscall(syscall.SYS_SETRESUID, ^uintptr(0), 0, ^uintptr(0))
	if err != 0 {
		// Continuing on a thread with unexpected credentials is unsafe.
		os.Exit(1)
	}
	runtime.UnlockOSThread()
	return killErr
}

func workerPrivileges() error {
	if os.Getuid() == 0 || os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return errors.New("worker credentials are not dropped")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return err
	}
	if _, _, err = credentials(string(status)); err != nil {
		return err
	}
	for _, line := range strings.Split(string(status), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "Groups:":
			if len(f) != 1 {
				return errors.New("worker supplementary groups remain")
			}
		case "CapInh:", "CapPrm:", "CapEff:", "CapAmb:":
			if len(f) != 2 || strings.Trim(f[1], "0") != "" {
				return errors.New("worker capabilities remain")
			}
		case "NoNewPrivs:":
			if len(f) != 2 || f[1] != "1" {
				return errors.New("worker no_new_privs is unset")
			}
		}
	}
	return nil
}

func runWorker(input io.Reader, output io.Writer) error {
	if err := workerPrivileges(); err != nil {
		return err
	}
	var w work
	if err := decode(input, &w); err != nil {
		return errors.New("invalid worker input")
	}
	if err := validateRequest(w.Request); err != nil {
		return err
	}
	p, err := current(w.Request)
	if err != nil || p.UID != uint32(os.Getuid()) || p.GID != uint32(os.Getgid()) || *p != w.Process {
		return json.NewEncoder(output).Encode(result(protocol.Blocked, "Process identity or credentials changed before attachment"))
	}
	// Native targets share the root filesystem. Require both JARs to resolve to
	// the bundled files in the target's mount namespace (including PrivateTmp).
	for _, name := range []string{"otel.jar", "hikari-extension.jar"} {
		path := filepath.Join(bundle(), name)
		file, err := os.Open(path)
		if err != nil {
			return json.NewEncoder(output).Encode(result(protocol.Blocked, "Bundled instrumentation is not readable by the target user"))
		}
		installed, statErr := file.Stat()
		_ = file.Close()
		visible, err := os.Stat(filepath.Join("/proc", fmt.Sprint(p.PID), "root", path))
		if statErr != nil || err != nil || !os.SameFile(installed, visible) {
			return json.NewEncoder(output).Encode(result(protocol.Blocked, "Bundled instrumentation is not visible in the target filesystem"))
		}
	}
	visible := bundle()
	p2, err := current(w.Request)
	if err != nil || *p2 != *p {
		return json.NewEncoder(output).Encode(result(protocol.Blocked, "Process changed during instrumentation delivery"))
	}
	cmd := exec.Command(filepath.Join(bundle(), "runtime/bin/java"), "-Xms16m", "-Xmx64m", "--add-modules=jdk.attach", "-cp", filepath.Join(bundle(), "helper"), "NetdataAttach")
	cmd.Dir = "/"
	cmd.Env = []string{"LANG=C", "LC_ALL=C"}
	// No secret or target-derived option is ever present in a command line.
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%d\n%d\n%s\n%d\n%d\n%s\n%s\n%d\n%s\n", p.PID, p.StartTime, p.BootID, p.UID, p.GID, p.Application, w.Request.Token, w.Request.Port, visible))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err = cmd.Run()
	r := result(protocol.Unknown, "Agent loading outcome unknown; no automatic retry")
	if err == nil && strings.TrimSpace(out.String()) == "ACK" {
		r = result(protocol.Attached, "Agent load acknowledged; waiting for authenticated metrics")
	}
	if strings.TrimSpace(out.String()) == "BLOCKED" {
		r = result(protocol.Blocked, "JVM attach is unavailable, unsupported, or existing OpenTelemetry instrumentation was detected")
	}
	return json.NewEncoder(output).Encode(r)
}
