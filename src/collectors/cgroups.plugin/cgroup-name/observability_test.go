// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
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

func TestExpiredKubernetesResolutionRetriesWithoutFallback(t *testing.T) {
	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		tmpDir:   t.TempDir(),
		logLevel: ndlpEmerg,
	})
	r.budget.expiresAt = time.Now().Add(-time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	id := "kubepods-burstable-pod11111111_2222_3333_4444_555555555555_docker-" + strings.Repeat("8", 64) + ".scope"
	result := r.resolve(ctx, "missing", id)
	if result.exitCode != exitRetry {
		t.Fatalf("exit code = %d, want retry", result.exitCode)
	}
	if result.name != "" || len(result.labels.items) != 0 {
		t.Fatalf("expired resolution emitted fallback: name=%q labels=%q", result.name, result.labels.String())
	}
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
