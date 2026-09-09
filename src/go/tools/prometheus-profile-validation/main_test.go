// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelpSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := runCLI([]string{"--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("help exit code: got %d, want 0; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("help output missing usage: %s", stderr.String())
	}
}

func TestCLICompatibilityFlagsRunValidation(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	jobPath := filepath.Join(dir, "job.yaml")
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      dimensions:
        - selector: app_value
          name: value
`
	for path, content := range map[string]string{
		profilePath: profile,
		dumpPath:    "# TYPE app_value gauge\napp_value 1\n",
		jobPath:     "name: validation_job\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	exitCode := runCLI([]string{
		"--profile", profilePath,
		"--dump", dumpPath,
		"--job", jobPath,
		"--output", "json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code=%d stderr=%s report=%s", exitCode, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"verdict": "PASS"`) {
		t.Fatalf("JSON report missing PASS verdict: %s", stdout.String())
	}
}

func TestCLIComposesRepeatableSupportingProfiles(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	for path, content := range map[string]string{
		profilePath: `
match: app_*
template:
  family: App
  metrics: [app_value]
  charts:
    - title: App Value
      context: app_value
      units: values
      dimensions: [{selector: app_value, name: value}]
`,
		supportPath: `
match: runtime_*
template:
  family: Runtime
  metrics: [runtime_value]
  charts:
    - title: Runtime Value
      context: runtime_value
      units: values
      dimensions: [{selector: runtime_value, name: value}]
`,
		dumpPath: "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_value gauge\nruntime_value 2\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	exitCode := runCLI([]string{
		"--profile", profilePath,
		"--support-profile", supportPath,
		"--dump", dumpPath,
		"--output", "json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code=%d stderr=%s report=%s", exitCode, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"authored_charts": 2`) {
		t.Fatalf("JSON report missing composed authored chart count: %s", stdout.String())
	}
}
