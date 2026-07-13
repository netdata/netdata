// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type countingStringer struct {
	calls int
}

func (s *countingStringer) String() string {
	s.calls++
	return "formatted"
}

func TestBuildCmdLinePreservesShellQuotingBug(t *testing.T) {
	got := buildCmdLine([]string{"cgroup-name", "/docker/a'b", "docker_a"})
	want := "'cgroup-name' '/docker/a'b' 'docker_a' "
	if got != want {
		t.Fatalf("unexpected cmd line:\nwant %q\n got %q", want, got)
	}
}

func TestSetupDeadlineBudget(t *testing.T) {
	tests := map[string]struct {
		timeout     time.Duration
		wantExpires bool
	}{
		"unbounded": {},
		"expires": {
			timeout:     10 * time.Millisecond,
			wantExpires: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpInfo, timeout: test.timeout})
			ctx, cancel := r.setupDeadline()
			defer cancel()
			if got := !r.budget.expiresAt.IsZero(); got != test.wantExpires {
				t.Fatalf("deadline configured = %v, want %v", got, test.wantExpires)
			}
			if r.budgetExpired() {
				t.Fatal("budget must not be expired immediately")
			}
			if !test.wantExpires {
				return
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("deadline context did not expire")
			}
			if !r.budgetExpired() {
				t.Fatal("budget must report expired after the deadline passes")
			}
		})
	}
}

func TestLoggerFormatsMessages(t *testing.T) {
	r := newResolver([]string{"cgroup-name", "weird arg"}, invocationConfig{logLevel: ndlpDebug})
	tests := map[string]struct {
		format      string
		args        []any
		wantMessage string
	}{
		"literal escaping": {
			format:      "name has a \" quote and a\nnewline",
			wantMessage: "name has a \" quote and a\nnewline",
		},
		"formatted arguments": {
			format:      "container %q has %d labels",
			args:        []any{"api", 3},
			wantMessage: `container "api" has 3 labels`,
		},
		"literal percent without arguments": {
			format:      "progress is 100%",
			wantMessage: "progress is 100%",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			line := captureStderr(t, func() {
				r.errorf(test.format, test.args...)
			})
			if strings.Count(strings.TrimRight(line, "\n"), "\n") != 0 {
				t.Fatalf("log line must be single-line, got: %q", line)
			}
			if !strings.Contains(line, "level=error") || !strings.Contains(line, "comm=cgroup-name") {
				t.Fatalf("log line missing expected fields: %q", line)
			}
			if want := "msg=" + strconv.Quote(test.wantMessage); !strings.Contains(line, want) {
				t.Fatalf("log line missing %q: %q", want, line)
			}
		})
	}
}

func TestLoggerSkipsFormattingBelowConfiguredLevel(t *testing.T) {
	logger := newInvocationLogger([]string{"cgroup-name"}, ndlpErr)
	value := &countingStringer{}

	logger.logf(ndlpDebug, "value=%s", value)

	if value.calls != 0 {
		t.Fatalf("disabled log formatted its arguments %d times", value.calls)
	}
}

func captureStderr(t *testing.T, write func()) string {
	t.Helper()

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readPipe.Close()
	writeClosed := false
	defer func() {
		if !writeClosed {
			_ = writePipe.Close()
		}
	}()

	old := os.Stderr
	os.Stderr = writePipe
	defer func() { os.Stderr = old }()

	write()
	os.Stderr = old
	if err := writePipe.Close(); err != nil {
		t.Fatal(err)
	}
	writeClosed = true
	out, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
