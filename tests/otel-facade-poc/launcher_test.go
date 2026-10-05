// SPDX-License-Identifier: GPL-3.0-or-later

package otelfacadepoc_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func stageLauncher(t *testing.T, binary string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "launcher 'space $; literal")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("otel-facade.plugin")
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "otel-facade.plugin")
	if err := os.WriteFile(launcher, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, filepath.Join(dir, "otel-facade")); err != nil {
		t.Fatal(err)
	}
	return launcher
}

func TestLauncherContract(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "fake-collector")
	if err := os.WriteFile(binary, []byte(`#!/bin/sh
printf '%s\n' "$#" "$@" "$NETDATA_OTEL_POC_STATE_DIR"
exit "${POC_TEST_EXIT:-0}"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := stageLauncher(t, binary)
	libDir := filepath.Join(t.TempDir(), "lib $; literal")
	t.Setenv("NETDATA_LIB_DIR", libDir)
	t.Setenv("NETDATA_OTEL_POC_STATE_DIR", "")
	t.Setenv("POC_TEST_EXIT", "0")
	for _, args := range [][]string{nil, {"1"}, {"10"}} {
		cmd := exec.Command(launcher, args...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("args %v: %v: %s", args, err, stderr.String())
		}
		want := "2\n--config=netdata:local\n--feature-gates=service.AllowNoPipelines\n" + libDir + "/otel-facade-poc\n"
		if string(output) != want {
			t.Fatalf("wrong argv/state or polluted stdout: %q", output)
		}
	}
	// Environment settings can contain sensitive data; the launcher logs only
	// its fixed arguments, never environment values.
	t.Setenv("NETDATA_OTEL_POC_ENDPOINT", "synthetic-secret.invalid:4317")
	t.Setenv("NETDATA_OTEL_POC_STATE_DIR", "/tmp/explicit-state")
	t.Setenv("POC_TEST_EXIT", "23")
	cmd := exec.Command(launcher, "1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if cmd.ProcessState.ExitCode() != 23 || !strings.HasSuffix(string(output), "/tmp/explicit-state\n") {
		t.Fatalf("override or exit status lost: %q, %v", output, err)
	}
	if strings.Contains(stderr.String(), "synthetic-secret") {
		t.Fatal("environment leaked into command log")
	}
}

func TestLauncherRejectsInvalidInvocation(t *testing.T) {
	launcher := stageLauncher(t, filepath.Join(t.TempDir(), "missing-binary"))
	t.Setenv("NETDATA_LIB_DIR", "")
	t.Setenv("NETDATA_OTEL_POC_STATE_DIR", "")
	for _, args := range [][]string{{"0"}, {"bad"}, {"1", "extra"}, {"1"}} {
		cmd := exec.Command(launcher, args...)
		output, err := cmd.Output()
		if err == nil || cmd.ProcessState.ExitCode() != 2 || len(output) != 0 {
			t.Fatalf("invalid invocation %v: %q, %v", args, output, err)
		}
	}
	t.Setenv("NETDATA_OTEL_POC_STATE_DIR", "/tmp/state")
	cmd := exec.Command(launcher, "1")
	output, err := cmd.CombinedOutput()
	if err == nil || cmd.ProcessState.ExitCode() != 127 || !strings.Contains(string(output), "failed in") {
		t.Fatalf("missing executable: %q, %v", output, err)
	}
}
