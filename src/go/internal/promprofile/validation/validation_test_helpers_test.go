// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDump = `
# HELP app_temperature Current temperature.
# TYPE app_temperature gauge
app_temperature{instance="node-a"} 42
# HELP app_requests_total Completed requests.
# TYPE app_requests_total counter
app_requests_total{instance="node-a"} 10
# HELP app_latency_seconds Request latency.
# TYPE app_latency_seconds histogram
app_latency_seconds_bucket{instance="node-a",le="0.5"} 2
app_latency_seconds_bucket{instance="node-a",le="+Inf"} 3
app_latency_seconds_sum{instance="node-a"} 0.9
app_latency_seconds_count{instance="node-a"} 3
# HELP app_size_bytes Response size.
# TYPE app_size_bytes summary
app_size_bytes{instance="node-a",quantile="0.5"} 100
app_size_bytes{instance="node-a",quantile="0.9"} 200
app_size_bytes_sum{instance="node-a"} 500
app_size_bytes_count{instance="node-a"} 3
`

const validProfile = `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [instance]
  metrics:
    - app_temperature
    - app_requests_total
    - app_latency_seconds_bucket
    - app_latency_seconds_count
    - app_latency_seconds_sum
    - app_size_bytes
    - app_size_bytes_count
    - app_size_bytes_sum
  charts:
    - title: Temperature
      context: temperature
      units: celsius
      dimensions:
        - selector: app_temperature
          name: temperature
    - title: Requests
      context: requests
      units: requests/s
      algorithm: incremental
      dimensions:
        - selector: app_requests_total
          name: requests
    - title: Latency Distribution
      context: latency
      units: observations/s
      algorithm: incremental
      type: heatmap
      dimensions:
        - selector: app_latency_seconds_bucket
          name_from_label: le
        - selector: app_latency_seconds_count
          name: observations
        - selector: app_latency_seconds_sum
          name: total
    - title: Size Summary
      context: size
      units: observations/s
      algorithm: incremental
      dimensions:
        - selector: app_size_bytes
          name_from_label: quantile
        - selector: app_size_bytes_count
          name: observations
        - selector: app_size_bytes_sum
          name: total
`

const singleInstanceValueGaugeProfile = `
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
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`

const singleDynamicValueGaugeProfile = `
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
          name_from_label: state
`

const twoValueGaugesDump = "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"

const currentCapacityGaugesDump = `
# TYPE app_current gauge
app_current 1
# TYPE app_capacity gauge
app_capacity 1000
`

type validationResult struct {
	exitCode int
	stdout   string
	stderr   string
	report   Report
}

func replaceOnce(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if count := strings.Count(source, old); count != 1 {
		t.Fatalf("fixture replacement target occurs %d times, want exactly 1: %q", count, old)
	}
	return strings.Replace(source, old, replacement, 1)
}

func runValidation(t *testing.T, profile, dump, job string) validationResult {
	return runValidationWithAggregateEvidence(t, profile, dump, job, false)
}

func runValidationWithAggregateEvidence(t *testing.T, profile, dump, job string, aggregate bool) validationResult {
	return runValidationWithEvidenceModes(t, profile, dump, job, aggregate, false)
}

func runValidationWithSemanticCoverage(t *testing.T, profile, dump, job string) validationResult {
	return runValidationWithEvidenceModes(t, profile, dump, job, true, true)
}

func runValidationWithEvidenceModes(
	t *testing.T,
	profile, dump, job string,
	aggregate, semanticCoverage bool,
) validationResult {
	t.Helper()
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.txt")
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}

	jobPath := ""
	if job != "" {
		jobPath = filepath.Join(dir, "job.yaml")
		if err := os.WriteFile(jobPath, []byte(job), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opts := Options{ProfilePath: profilePath, DumpPath: dumpPath, JobPath: jobPath}
	reports, err := validateSequence(context.Background(), opts, []string{dumpPath}, validationMode{
		aggregateProfileEvidence: aggregate,
		semanticCoverageReplay:   semanticCoverage,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := reports[0]
	return validationResultFromReport(t, got)
}

func runValidationFiles(t *testing.T, profilePath, dumpPath, jobPath string) validationResult {
	return runValidationFilesWithSupports(t, profilePath, nil, dumpPath, jobPath)
}

func validateTestMode(ctx context.Context, opts Options, mode validationMode) Report {
	reports, _ := validateSequence(ctx, opts, []string{opts.DumpPath}, mode)
	return reports[0]
}

func runValidationFilesWithSupports(
	t *testing.T,
	profilePath string,
	supportingProfilePaths []string,
	dumpPath string,
	jobPath string,
) validationResult {
	t.Helper()
	got := Validate(context.Background(), Options{
		ProfilePath:            profilePath,
		SupportingProfilePaths: supportingProfilePaths,
		DumpPath:               dumpPath,
		JobPath:                jobPath,
	})
	return validationResultFromReport(t, got)
}

func validationResultFromReport(t *testing.T, got Report) validationResult {
	t.Helper()
	var stdout bytes.Buffer
	if err := writeJSONReport(&stdout, got); err != nil {
		t.Fatalf("write report: %v", err)
	}
	exitCode := 0
	if got.Verdict != verdictPass {
		exitCode = 1
	}

	var decoded Report
	if decodeErr := json.Unmarshal(stdout.Bytes(), &decoded); decodeErr != nil {
		t.Fatalf("decode report: %v\nstdout:\n%s", decodeErr, stdout.String())
	}
	return validationResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		report:   decoded,
	}
}

func requireFinding(t *testing.T, result validationResult, code string) {
	t.Helper()
	if result.exitCode != 1 {
		t.Fatalf("expected failure exit 1, got %d\nstderr:\n%s\nreport:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	for _, item := range result.report.Findings {
		if item.Code == code {
			return
		}
	}
	t.Fatalf("missing finding %q in %#v", code, result.report.Findings)
}

func hasFinding(r Report, code, severity string) bool {
	for _, item := range r.Findings {
		if item.Code == code && item.Severity == severity {
			return true
		}
	}
	return false
}
