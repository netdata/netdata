// SPDX-License-Identifier: GPL-3.0-or-later

package privileged

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
)

const procRoot = "/proc"

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

func bootID(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "sys/kernel/random/boot_id"))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if len(id) != 36 {
		return "", fmt.Errorf("invalid boot identity")
	}
	return id, nil
}

func namespaceReason(root, proc string) string {
	for _, ns := range []string{"pid", "user", "net"} {
		own, err := os.Stat(filepath.Join(root, "self/ns", ns))
		if err != nil {
			return "Cannot inspect host namespaces"
		}
		target, err := os.Stat(filepath.Join(proc, "ns", ns))
		if err != nil {
			return "Cannot inspect process namespaces"
		}
		if !os.SameFile(own, target) {
			return "Different " + ns + " namespace is unsupported"
		}
	}
	own, err := os.Stat(filepath.Join(root, "self/root"))
	if err != nil {
		return "Cannot inspect host root filesystem"
	}
	target, err := os.Stat(filepath.Join(proc, "root"))
	if err != nil {
		return "Cannot inspect process root filesystem"
	}
	if !os.SameFile(own, target) {
		return "Different root filesystem is unsupported"
	}
	return ""
}

func inspect(root string, pid int, boot string) (*protocol.Process, error) {
	proc := filepath.Join(root, strconv.Itoa(pid))
	exe, err := os.Readlink(filepath.Join(proc, "exe"))
	if err != nil {
		return nil, err
	}
	if filepath.Base(strings.TrimSuffix(exe, " (deleted)")) != "java" {
		return nil, nil
	}
	stat, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return nil, err
	}
	start, err := startTime(string(stat))
	if err != nil {
		return nil, err
	}
	status, err := os.ReadFile(filepath.Join(proc, "status"))
	if err != nil {
		return nil, err
	}
	uid, gid, credentialErr := credentials(string(status))
	if credentialErr != nil && !errors.Is(credentialErr, errMixedCredentials) {
		return nil, credentialErr
	}
	// argv is untrusted and only used for display. Do not read environments or expose raw arguments.
	cmd, err := readBounded(filepath.Join(proc, "cmdline"), 64*1024)
	if err != nil {
		return nil, err
	}
	name, helper := application(strings.Split(string(cmd), "\x00"))
	if helper {
		return nil, nil
	}
	p := &protocol.Process{PID: pid, StartTime: start, BootID: boot, UID: uid, GID: gid, Application: name, Location: "Native host"}
	p.Reason = namespaceReason(root, proc)
	if p.Reason != "" {
		p.Location = "Unsupported namespace/root filesystem"
	} else if credentialErr != nil {
		p.Reason = "Mixed process credentials are unsupported"
	} else if uid == 0 {
		p.Reason = "Root-owned JVMs are unsupported"
	}
	p.Eligible = p.Reason == ""
	return p, nil
}

func discover(root string) (protocol.Discovery, error) {
	out := protocol.Discovery{Processes: []protocol.Process{}}
	boot, err := bootID(root)
	if err != nil {
		return out, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		p, err := inspect(root, pid, boot)
		if err == nil && p != nil {
			out.Processes = append(out.Processes, *p)
		}
	}
	sort.Slice(out.Processes, func(i, j int) bool { return out.Processes[i].PID < out.Processes[j].PID })
	return out, nil
}
