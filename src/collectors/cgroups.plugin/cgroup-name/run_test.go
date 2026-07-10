// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture write failure")
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}

func TestRunContract(t *testing.T) {
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

func TestWriteResolutionIsAtomicAndBounded(t *testing.T) {
	tests := map[string]struct {
		result resolution
		writer io.Writer
		want   string
		code   int
	}{
		"boundary-sized line succeeds": {
			result: resolution{name: strings.Repeat("n", maxCgroupNameLineBytes)},
			want:   strings.Repeat("n", maxCgroupNameLineBytes) + "\n",
		},
		"oversized line fails without partial output": {
			result: resolution{name: strings.Repeat("n", maxCgroupNameLineBytes+1)},
			code:   exitDisable,
		},
		"write failure is fatal": {
			result: resolution{name: "name"},
			writer: errorWriter{},
			code:   exitFatal,
		},
		"short write is fatal": {
			result: resolution{name: "name"},
			writer: shortWriter{},
			code:   exitFatal,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpEmerg})
			var out bytes.Buffer
			writer := test.writer
			if writer == nil {
				writer = &out
			}
			code := r.writeResolution(writer, "fixture", test.result)
			if code != test.code {
				t.Fatalf("exit code = %d, want %d", code, test.code)
			}
			if got := out.String(); got != test.want {
				t.Fatalf("stdout = %q, want %q", got, test.want)
			}
		})
	}
}
