// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"strings"
)

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

	result.name = strings.ReplaceAll(result.name, " ", "_")
	labels := result.labels.String()
	r.infof("cgroup '%s' is called '%s', labels '%s'", cgroup, result.name, labels)
	if labels == "" {
		fmt.Fprintf(stdout, "%s\n", result.name)
	} else {
		fmt.Fprintf(stdout, "%s %s\n", result.name, labels)
	}
	return result.exitCode
}
