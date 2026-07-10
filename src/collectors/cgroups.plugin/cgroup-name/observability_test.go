// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildCmdLinePreservesShellQuotingBug(t *testing.T) {
	got := buildCmdLine([]string{"cgroup-name", "/docker/a'b", "docker_a"})
	want := "'cgroup-name' '/docker/a'b' 'docker_a' "
	if got != want {
		t.Fatalf("unexpected cmd line:\nwant %q\n got %q", want, got)
	}
}

func TestSetupDeadlineBudget(t *testing.T) {
	t.Run("unbounded", func(t *testing.T) {
		r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpInfo})
		_, cancel := r.setupDeadline()
		defer cancel()
		if !r.budget.expiresAt.IsZero() {
			t.Fatal("expiresAt must be zero when no timeout is configured")
		}
		if r.budgetExpired() {
			t.Fatal("an unbounded budget must never report expired")
		}
	})

	t.Run("expires", func(t *testing.T) {
		r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpInfo, timeout: 10 * time.Millisecond})
		ctx, cancel := r.setupDeadline()
		defer cancel()
		if r.budget.expiresAt.IsZero() {
			t.Fatal("expiresAt must be set when a timeout is configured")
		}
		if r.budgetExpired() {
			t.Fatal("budget must not be expired immediately")
		}
		time.Sleep(20 * time.Millisecond)
		if !r.budgetExpired() {
			t.Fatal("budget must report expired after the deadline passes")
		}
		if ctx.Err() == nil {
			t.Fatal("the deadline context must be done after the budget expires")
		}
	})
}

func TestLogfmtIsSingleLineAndQuoted(t *testing.T) {
	r := newResolver([]string{"cgroup-name", "weird arg"}, invocationConfig{logLevel: ndlpDebug})

	old := os.Stderr
	readPipe, writePipe, _ := os.Pipe()
	os.Stderr = writePipe
	r.error("name has a \" quote and a\nnewline")
	_ = writePipe.Close()
	os.Stderr = old

	out, _ := io.ReadAll(readPipe)
	line := string(out)
	if strings.Count(strings.TrimRight(line, "\n"), "\n") != 0 {
		t.Fatalf("log line must be single-line, got: %q", line)
	}
	if !strings.Contains(line, "level=error") || !strings.Contains(line, "comm=cgroup-name") {
		t.Fatalf("log line missing expected fields: %q", line)
	}
	if !strings.Contains(line, `\"`) || !strings.Contains(line, `\n`) {
		t.Fatalf("quotes and newlines in the message must be escaped: %q", line)
	}
}
