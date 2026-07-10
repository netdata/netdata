// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
	"strings"
)

// Keep synchronized with CGROUP_NAME_LINE_MAX in cgroup-name-labels.h.
const maxCgroupNameLineBytes = 8190

func run(args []string, stdout io.Writer) int {
	return runWithConfig(args, stdout, prepareInvocationConfig())
}

func runWithConfig(args []string, stdout io.Writer, config invocationConfig) int {
	r := newResolver(args, config)
	ctx, cancel := r.setupDeadline()
	defer cancel()

	var cgroupPath string
	if len(args) > 1 {
		cgroupPath = args[1]
	}
	var cgroup string
	if len(args) > 2 {
		cgroup = strings.ReplaceAll(args[2], "/", "_")
	}
	if cgroup == "" {
		r.fatalf("called without a cgroup name. Nothing to do.")
		return exitFatal
	}

	result := r.resolve(ctx, cgroupPath, cgroup)
	if result.name == "" {
		return result.exitCode
	}
	return r.writeResolution(stdout, cgroup, result)
}

func (r *resolver) writeResolution(stdout io.Writer, cgroup string, result resolution) int {
	result.name = strings.ReplaceAll(result.name, " ", "_")
	labels := result.labels.String()
	line := result.name
	if labels != "" {
		line += " " + labels
	}
	if len(line) > maxCgroupNameLineBytes {
		r.errorf("cgroup '%s' resolution is %d bytes, maximum is %d", cgroup, len(line), maxCgroupNameLineBytes)
		return exitDisable
	}
	r.infof("cgroup '%s' is called '%s', labels '%s'", cgroup, result.name, labels)
	written, err := io.WriteString(stdout, line+"\n")
	if err != nil || written != len(line)+1 {
		r.errorf("cannot write cgroup '%s' resolution: wrote %d of %d bytes: %v", cgroup, written, len(line)+1, err)
		return exitFatal
	}
	return result.exitCode
}
