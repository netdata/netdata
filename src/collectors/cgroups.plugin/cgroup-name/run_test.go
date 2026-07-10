// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunContract(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("NETDATA_HOST_PREFIX", "")
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	tests := map[string]struct {
		args   []string
		stdout string
		code   int
	}{
		"missing cgroup is fatal": {
			args: []string{"cgroup-name"},
			code: exitFatal,
		},
		"raw fallback normalizes slashes": {
			args:   []string{"cgroup-name", "/plain/group", "plain/group"},
			stdout: "plain_group\n",
			code:   exitSuccess,
		},
		"non-kubernetes fallback is limited to 100 bytes": {
			args:   []string{"cgroup-name", "/plain/group", strings.Repeat("x", 101)},
			stdout: strings.Repeat("x", 100) + "\n",
			code:   exitSuccess,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if code := run(tt.args, &out); code != tt.code {
				t.Fatalf("exit code: want %d got %d", tt.code, code)
			}
			if got := out.String(); got != tt.stdout {
				t.Fatalf("stdout:\nwant %q\n got %q", tt.stdout, got)
			}
		})
	}
}
