// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
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
      priority: 100
      dimensions:
        - selector: app_temperature
          name: temperature
    - title: Requests
      context: requests
      units: requests/s
      algorithm: incremental
      priority: 110
      dimensions:
        - selector: app_requests_total
          name: requests
    - title: Latency Distribution
      context: latency
      units: observations/s
      algorithm: incremental
      type: heatmap
      priority: 120
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
      priority: 130
      dimensions:
        - selector: app_size_bytes
          name_from_label: quantile
        - selector: app_size_bytes_count
          name: observations
        - selector: app_size_bytes_sum
          name: total
`

func TestValidatorHelperProcess(t *testing.T) {
	if os.Getenv("NETDATA_PROFILE_VALIDATOR_HELPER") != "1" {
		return
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		os.Exit(2)
	}
	os.Exit(runCLI(os.Args[separator+1:], os.Stdout, os.Stderr))
}

func TestCLIHelpSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := runCLI([]string{"--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("help exit code: got %d, want 0; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("help output missing usage: %s", stderr.String())
	}
}

func TestValidateProfilePassesThroughRealPipeline(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS (exit 0), got %d\nstderr:\n%s\nreport:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	if result.report.Verdict != verdictPass {
		t.Fatalf("expected PASS report, got %#v", result.report.Findings)
	}
	if result.report.Counts.RawFamilies != 4 {
		t.Fatalf("raw family count: got %d, want 4", result.report.Counts.RawFamilies)
	}
	if result.report.Counts.SeriesAutogen != 0 || result.report.Counts.SeriesUnmatched != 0 {
		t.Fatalf("unexpected routing gaps: %#v", result.report.Counts)
	}
	if result.report.Counts.AuthoredCharts != 4 || result.report.Counts.CuratedCharts != 4 {
		t.Fatalf("unexpected chart counts: %#v", result.report.Counts)
	}
	if !hasFinding(result.report, "default_validation_job", "warning") {
		t.Fatalf("missing warning that the deployable job policy was not validated: %#v", result.report.Findings)
	}
	for _, chart := range result.report.Charts {
		if !strings.HasPrefix(chart.IDFingerprint, "sha256:") {
			t.Fatalf("materialized chart ID was not fingerprinted: %#v", chart)
		}
	}
}

func TestValidateProfileRejectsUnsafeJobFields(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "url: http://example.invalid/metrics\n")
	requireFinding(t, result, "job_policy")
	if !strings.Contains(result.report.Findings[0].Message, "field url not found") {
		t.Fatalf("unexpected strict-policy error: %s", result.report.Findings[0].Message)
	}
}

func TestValidateProfileAppliesRelabelAndFallbackPolicy(t *testing.T) {
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
      priority: 100
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	dump := "app_value{node=\"node-a\"} 7\n"
	job := `
fallback_type:
  gauge: [app_*]
relabeling:
  - match: app_value
    metric_relabel_configs:
      - source_labels: [node]
        target_label: instance
        regex: (.*)
        replacement: $1
        action: replace
`
	result := runValidation(t, profile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("expected PASS, got %d\nstderr:\n%s\nreport:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	if result.report.Job.RelabelBlocks != 1 || len(result.report.Job.FallbackGauge) != 1 {
		t.Fatalf("job policy was not applied: %#v", result.report.Job)
	}
}

func TestValidateProfileReportsWriterRejectedSummaryWithoutQuantiles(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_temperature]
  charts:
    - title: Temperature
      context: temperature
      units: celsius
      priority: 100
      dimensions:
        - selector: app_temperature
          name: temperature
`
	dump := `
# TYPE app_temperature gauge
app_temperature 42
# TYPE app_size_bytes summary
app_size_bytes_sum 500
app_size_bytes_count 3
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("writer-rejected family should be transparent but not a false coverage failure\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
	if len(result.report.PipelineExcluded) != 1 ||
		result.report.PipelineExcluded[0].Category != "writer_requires_summary_quantiles" {
		t.Fatalf("unexpected pipeline exclusion report: %#v", result.report.PipelineExcluded)
	}
}

func TestValidateProfileFindsCoverageGap(t *testing.T) {
	dump := validDump + "\n# TYPE app_extra gauge\napp_extra{instance=\"node-a\"} 1\n"
	result := runValidation(t, validProfile, dump, "")
	requireFinding(t, result, "unexpected_autogen")
	if result.report.Counts.SeriesAutogen != 1 {
		t.Fatalf("autogen series: got %d, want 1", result.report.Counts.SeriesAutogen)
	}
}

func TestValidateProfileFindsDeadChart(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"    - title: Temperature",
		"    - title: Missing\n      context: missing\n      units: values\n      priority: 90\n      dimensions:\n        - selector: app_missing\n          name: missing\n    - title: Temperature",
		1,
	)
	profile = strings.Replace(profile, "  metrics:\n", "  metrics:\n    - app_missing\n", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "dead_chart")
}

func TestValidateProfileFindsDeadDimensionInsideLiveChart(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_live]
  charts:
    - title: Mixed
      context: mixed
      units: values
      priority: 100
      dimensions:
        - selector: app_live
          name: live
        - selector: app_live
          name_from_label: missing_label
`
	result := runValidation(t, profile, "# TYPE app_live gauge\napp_live 1\n", "")
	requireFinding(t, result, "dead_dimension")
	if len(result.report.DeadCharts) != 0 || len(result.report.DeadDimensions) != 1 {
		t.Fatalf("expected one dead dimension in a live chart: charts=%#v dimensions=%#v", result.report.DeadCharts, result.report.DeadDimensions)
	}
}

func TestValidateProfileFindsMissingExplicitInstanceIdentity(t *testing.T) {
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
      priority: 100
      instances:
        by_labels: [node]
      dimensions:
        - selector: app_value
          name: value
`
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value{instance=\"a\"} 1\n", "")
	requireFinding(t, result, "dead_chart")
	if len(result.report.DeadCharts) != 1 {
		t.Fatalf("missing explicit identity label should make the chart unroutable: %#v", result.report.DeadCharts)
	}
}

func TestValidateProfileFindsObservedSameTemplateInstanceIDCollision(t *testing.T) {
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
      priority: 100
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	dump := "# TYPE app_value gauge\napp_value{instance=\"a.b\"} 1\napp_value{instance=\"a_b\"} 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "instance_id_collision_observed")
	if len(result.report.InstanceLosses) != 1 {
		t.Fatalf("expected one observed instance materialization loss: %#v", result.report.InstanceLosses)
	}
	if result.report.InstanceLosses[0].ObservedIdentities != 2 || result.report.InstanceLosses[0].RenderedIDs != 1 {
		t.Fatalf("unexpected instance collision counts: %#v", result.report.InstanceLosses[0])
	}
	if strings.Contains(result.stdout, "a.b") || strings.Contains(result.stdout, "a_b") {
		t.Fatalf("report leaked label-derived instance values:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsObservedDimensionWireIDCollision(t *testing.T) {
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
      priority: 100
      dimensions:
        - selector: app_value
          name_from_label: state
`
	dump := "# TYPE app_value gauge\napp_value{state=\"a\"} 1\napp_value{state=\"'a\"} 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_id_collision_observed")
	if len(result.report.DimensionCollisions) != 1 {
		t.Fatalf("expected one emitted dimension ID collision: %#v", result.report.DimensionCollisions)
	}
	if strings.Contains(result.stdout, "\"'a\"") {
		t.Fatalf("report leaked label-derived dimension value:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsDimensionLostAtWireEmission(t *testing.T) {
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
      priority: 100
      dimensions:
        - selector: app_value
          name_from_label: state
`
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value{state=\"'\"} 1\n", "")
	requireFinding(t, result, "dimension_wire_emission_loss")
	if strings.Contains(result.stdout, "state=\\\"'\\\"") {
		t.Fatalf("report leaked label-derived dimension value:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsObservedChartWireIDCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: one
      units: values
      priority: 100
      dimensions:
        - selector: app_one
          name: one
    - id: "'one"
      title: Two
      context: two
      units: values
      priority: 110
      dimensions:
        - selector: app_two
          name: two
`
	dump := "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "chart_wire_id_collision_observed")
	if len(result.report.ChartWireCollisions) != 1 ||
		result.report.ChartWireCollisions[0].Occurrences != 2 {
		t.Fatalf("expected one two-chart wire collision: %#v", result.report.ChartWireCollisions)
	}
}

func TestValidateProfileFindsObservedContextWireCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: shared
      units: values
      priority: 100
      dimensions:
        - selector: app_one
          name: one
    - id: two
      title: Two
      context: "'shared"
      units: values
      priority: 110
      dimensions:
        - selector: app_two
          name: two
`
	dump := "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "context_wire_collision_observed")
	if len(result.report.ContextCollisions) != 1 ||
		len(result.report.ContextCollisions[0].RawContextFingerprints) != 2 {
		t.Fatalf("expected two distinct raw contexts to collapse: %#v", result.report.ContextCollisions)
	}
}

func TestValidateProfileAllowsIntentionalRawContextReuse(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: shared
      units: values
      priority: 100
      dimensions:
        - selector: app_one
          name: one
    - id: two
      title: Two
      context: shared
      units: values
      priority: 110
      dimensions:
        - selector: app_two
          name: two
`
	dump := "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("intentional raw context reuse is not a wire-normalization collision\nreport:\n%s", result.stdout)
	}
	if len(result.report.ContextCollisions) != 0 {
		t.Fatalf("same raw context must not be reported as a normalization collision: %#v", result.report.ContextCollisions)
	}
}

func TestValidateProfileFindsEmptyChartWireID(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_value]
  charts:
    - id: "'"
      title: Value
      context: value
      units: values
      priority: 100
      dimensions:
        - selector: app_value
          name: value
`
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value 1\n", "")
	requireFinding(t, result, "chart_wire_id_empty")
}

func TestInspectEmittedPlanFindsEmptyContextWireValue(t *testing.T) {
	action := chartengine.CreateChartAction{ChartID: "value"}
	action.Meta.Context = "'"
	result, err := inspectEmittedPlan(chartengine.Plan{Actions: []chartengine.EngineAction{action}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.emptyContexts) != 1 {
		t.Fatalf("expected one empty emitted context: %#v", result)
	}
}

func TestValidateProfileDoesNotLeakChartIDFromEmitterError(t *testing.T) {
	const sentinel = "sensitive-instance-value-"
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      priority: 100
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	labelValue := sentinel + strings.Repeat("x", 1300)
	dump := fmt.Sprintf("# TYPE app_value gauge\napp_value{instance=%q} 1\n", labelValue)
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "chart_emit")
	if strings.Contains(result.stdout, sentinel) || strings.Contains(result.stderr, sentinel) {
		t.Fatalf("emitter failure leaked a label-derived chart ID\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileFindsLifecycleDimensionMaterializationLoss(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      priority: 100
      lifecycle:
        dimensions:
          max_dims: 2
      dimensions:
        - selector: app_value
          name_from_label: state
`
	dump := "# TYPE app_value gauge\napp_value{state=\"a\"} 1\napp_value{state=\"b\"} 2\napp_value{state=\"c\"} 3\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 3 ||
		result.report.DimensionLosses[0].PlannedDimensions != 2 {
		t.Fatalf("unexpected lifecycle dimension loss report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsLifecycleLossAcrossSiblingDimensions(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_a, app_b, app_c]
  charts:
    - title: Values
      context: values
      units: values
      priority: 100
      lifecycle:
        dimensions:
          max_dims: 2
      dimensions:
        - selector: app_a
          name: a
        - selector: app_b
          name: b
        - selector: app_c
          name: c
`
	dump := "# TYPE app_a gauge\napp_a 1\n# TYPE app_b gauge\napp_b 2\n# TYPE app_c gauge\napp_c 3\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 3 ||
		result.report.DimensionLosses[0].PlannedDimensions != 2 {
		t.Fatalf("unexpected sibling dimension loss report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsObservedSiblingDimensionNameCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - title: Values
      context: values
      units: values
      priority: 100
      dimensions:
        - selector: app_one
          name_from_label: state
        - selector: app_two
          name: x
`
	dump := "# TYPE app_one gauge\napp_one{state=\"x\"} 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 2 ||
		result.report.DimensionLosses[0].PlannedDimensions != 1 {
		t.Fatalf("unexpected sibling dimension collision report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsRenderedIDCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_one, app_two]
  charts:
    - title: One
      context: shared
      units: values
      priority: 100
      dimensions:
        - selector: app_one
          name: one
    - title: Two
      context: shared
      units: values
      priority: 110
      dimensions:
        - selector: app_two
          name: two
`
	dump := "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "rendered_id_collision")
	if len(result.report.Collisions) != 1 {
		t.Fatalf("collisions: got %#v", result.report.Collisions)
	}
	if !strings.HasPrefix(result.report.Collisions[0].RenderedIDFingerprint, "sha256:") {
		t.Fatalf("collision ID was not fingerprinted: %#v", result.report.Collisions[0])
	}
}

func TestValidateProfileRequiresExplicitPositivePriority(t *testing.T) {
	profile := strings.Replace(validProfile, "      priority: 110\n", "", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "priority_missing")
}

func TestValidateProfileKeepsPriorityTiesAsReviewWarnings(t *testing.T) {
	profile := strings.Replace(validProfile, "      priority: 110\n", "      priority: 100\n", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("priority ties preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "priority_duplicate", "warning") {
		t.Fatalf("missing priority tie review prompt: %#v", result.report.Findings)
	}
	if hasFinding(result.report, "priority_source_order", "error") {
		t.Fatalf("a tie is not descending source order: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsDescendingPrioritySourceOrder(t *testing.T) {
	profile := strings.Replace(validProfile, "      priority: 110\n", "      priority: 90\n", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "priority_source_order")
}

func TestValidateProfileReportsObservedDenyImpactWithoutLeakingLabels(t *testing.T) {
	dump := validDump + `
# TYPE app_removed gauge
app_removed{instance="sensitive-value"} 1
`
	result := runValidation(t, validProfile, dump, "selector:\n  deny: [app_removed]\n")
	if result.exitCode != 0 {
		t.Fatalf("deny review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "job_deny_review", "warning") {
		t.Fatalf("missing deny review prompt: %#v", result.report.Findings)
	}
	if !hasFinding(result.report, "job_policy_exclusion_summary", "warning") {
		t.Fatalf("missing job-policy denominator summary: %#v", result.report.Findings)
	}
	if strings.Contains(result.stdout, "sensitive-value") {
		t.Fatalf("deny review leaked an observed label value:\n%s", result.stdout)
	}
}

func TestValidateProfileDoesNotPromptForWriterSkippedInfoDeny(t *testing.T) {
	dump := validDump + `
# TYPE app_build_info gauge
app_build_info{version="test"} 1
`
	result := runValidation(t, validProfile, dump, "selector:\n  deny: ['*_info']\n")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "job_deny_review", "warning") {
		t.Fatalf("writer-skipped info family should not be presented as lost chart surface: %#v", result.report.Findings)
	}
}

func TestValidateProfileReportsObservedAllowListExclusions(t *testing.T) {
	dump := validDump + `
# TYPE runtime_resource gauge
runtime_resource 7
`
	result := runValidation(t, validProfile, dump, "selector:\n  allow: ['app_*']\n")
	if result.exitCode != 0 {
		t.Fatalf("allow-list impact review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "job_allow_exclusion_review", "warning") {
		t.Fatalf("missing allow-list exclusion prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsOnUnusedMetricDeclaration(t *testing.T) {
	profile := strings.Replace(validProfile, "  metrics:\n", "  metrics:\n    - app_unused\n", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("unused scope declaration is advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "unused_metric_declaration", "warning") {
		t.Fatalf("missing unused declaration prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenAuthoredChartIntentDiffersFromVisualSemantics(t *testing.T) {
	profile := strings.Replace(validProfile, "      type: heatmap\n", "      type: line\n", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("visual-semantic review prompts are advisory\nreport:\n%s", result.stdout)
	}
	for _, code := range []string{
		"distribution_role_mixing",
		"histogram_type_runtime_override",
	} {
		if !hasFinding(result.report, code, "warning") {
			t.Fatalf("missing review prompt %q: %#v", code, result.report.Findings)
		}
	}
}

func TestValidateProfileRejectsIncorrectHistogramBucketPresentation(t *testing.T) {
	tests := map[string]struct {
		profile string
		code    string
	}{
		"units": {
			profile: strings.Replace(
				validProfile,
				"      context: latency\n      units: observations/s\n",
				"      context: latency\n      units: seconds\n",
				1,
			),
			code: "histogram_bucket_units",
		},
		"algorithm": {
			profile: strings.Replace(
				validProfile,
				"      context: latency\n      units: observations/s\n      algorithm: incremental\n",
				"      context: latency\n      units: observations/s\n      algorithm: absolute\n",
				1,
			),
			code: "histogram_bucket_algorithm",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := runValidation(t, tc.profile, validDump, "")
			requireFinding(t, result, tc.code)
		})
	}
}

func TestValidateProfileRejectsChartWithEveryDimensionHidden(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_current, app_capacity]
  charts:
    - title: Invisible
      context: invisible
      units: items
      priority: 100
      dimensions:
        - selector: app_current
          name: current
          options:
            hidden: true
        - selector: app_capacity
          name: capacity
          options:
            hidden: true
`
	dump := `
# TYPE app_current gauge
app_current 1
# TYPE app_capacity gauge
app_capacity 1000
`
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "all_dimensions_hidden")
}

func TestValidateProfileWarnsOnObservedAbsoluteDimensionScaleGap(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_current, app_capacity]
  charts:
    - title: Current and Capacity
      context: current_capacity
      units: items
      priority: 100
      dimensions:
        - selector: app_current
          name: current
        - selector: app_capacity
          name: capacity
`
	dump := `
# TYPE app_current gauge
app_current 1
# TYPE app_capacity gauge
app_capacity 1000
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("observed scale review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "observed_scale_gap", "warning") {
		t.Fatalf("missing observed scale prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileDoesNotTreatHiddenDimensionAsVisibleScale(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_current, app_capacity]
  charts:
    - title: Current and Capacity
      context: current_capacity
      units: items
      priority: 100
      dimensions:
        - selector: app_current
          name: current
        - selector: app_capacity
          name: capacity
          options:
            hidden: true
`
	dump := `
# TYPE app_current gauge
app_current 1
# TYPE app_capacity gauge
app_capacity 1000
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "observed_scale_gap", "warning") {
		t.Fatalf("hidden dimensions do not control the default visible axis: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsFilledNonVolumeGauge(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_waiting]
  charts:
    - title: Waiting
      context: waiting
      units: requests
      type: stacked
      priority: 100
      dimensions:
        - selector: app_waiting
          name_from_label: reason
`
	dump := `
# TYPE app_waiting gauge
app_waiting{reason="capacity"} 1
app_waiting{reason="deferred"} 2
`
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "filled_nonvolume_units")
}

func TestValidateProfileKeepsPhysicalRateFillAsReviewWarning(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_read_bytes_total, app_write_bytes_total]
  charts:
    - title: I/O
      context: io
      units: bytes/s
      algorithm: incremental
      type: area
      priority: 100
      dimensions:
        - selector: app_read_bytes_total
          name: read
        - selector: app_write_bytes_total
          name: write
`
	dump := `
# TYPE app_read_bytes_total counter
app_read_bytes_total 100
# TYPE app_write_bytes_total counter
app_write_bytes_total 50
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("physical flow fill remains a judgment-preserving review\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "rate_filled_type_review", "warning") {
		t.Fatalf("missing physical-rate fill review prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsFilledPhysicalStorageUnit(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_resident_memory_bytes]
  charts:
    - title: Resident Memory
      context: resident_memory
      units: MiB
      type: area
      priority: 100
      dimensions:
        - selector: app_resident_memory_bytes
          name: resident
          options:
            divisor: 1048576
`
	dump := `
# TYPE app_resident_memory_bytes gauge
app_resident_memory_bytes 1048576
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("physical storage fill should pass\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "nonvolume_filled_type_review", "warning") {
		t.Fatalf("MiB is an unambiguous physical storage unit: %#v", result.report.Findings)
	}
}

func TestReportStatesExactModeDoesNotProveAutomaticProfileSelection(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	for _, limit := range result.report.EvidenceLimits {
		if strings.Contains(limit, "profile.match") && strings.Contains(limit, "auto-selects") {
			return
		}
	}
	t.Fatalf("missing automatic profile-selection evidence limit: %#v", result.report.EvidenceLimits)
}

func TestValidateProfileWarnsWhenMatchAcceptsGenericRuntimeFamilies(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: 'app_* process_* python_*'", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("generic detection review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "generic_profile_match", "warning") {
		t.Fatalf("missing generic profile-match review prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsAboutObservedLabelsWithoutAnAuthoredRole(t *testing.T) {
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
      priority: 100
      instances:
        by_labels: [instance]
      dimensions:
        - selector: 'app_value{mode="sync"}'
          name: value
`
	dump := `
# TYPE app_value gauge
app_value{instance="node-a",mode="sync",engine="sensitive-engine-value"} 1
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("label-role review must preserve author judgment\nreport:\n%s", result.stdout)
	}

	var message string
	for _, item := range result.report.Findings {
		if item.Code == "observed_label_aggregation" && item.Severity == "warning" {
			message = item.Message
			break
		}
	}
	if message == "" {
		t.Fatalf("missing observed-label aggregation prompt: %#v", result.report.Findings)
	}
	if !strings.Contains(message, "engine") {
		t.Fatalf("unaccounted label key is absent from warning: %q", message)
	}
	if strings.Contains(message, "instance") || strings.Contains(message, "mode") {
		t.Fatalf("identity and selector-routing labels were misreported as aggregated: %q", message)
	}
	if strings.Contains(result.stdout, "sensitive-engine-value") {
		t.Fatalf("label-role review leaked an observed label value:\n%s", result.stdout)
	}
}

func TestValidateProfileWarnsOnSiblingIdentityMismatchWithoutReplacingJudgment(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: Per Node
      metrics: [app_node_value]
      charts:
        - title: Node Value
          context: node_value
          units: values
          priority: 100
          instances:
            by_labels: [node]
          dimensions:
            - selector: app_node_value
              name: value
    - family: Global
      metrics: [app_global_value]
      charts:
        - title: Global Value
          context: global_value
          units: values
          priority: 110
          dimensions:
            - selector: app_global_value
              name: value
`
	dump := "# TYPE app_node_value gauge\napp_node_value{node=\"a\"} 1\n# TYPE app_global_value gauge\napp_global_value 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("heuristic must remain a warning, not a false mechanical failure\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "sibling_identity_mismatch", "warning") {
		t.Fatalf("missing sibling identity review prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileTreatsGlobalSiblingChartsAsCommonIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: First
      metrics: [app_one]
      charts:
        - title: One
          context: one
          units: values
          priority: 100
          dimensions:
            - selector: app_one
              name: one
    - family: Second
      metrics: [app_two]
      charts:
        - title: Two
          context: two
          units: values
          priority: 110
          dimensions:
            - selector: app_two
              name: two
`
	dump := "# TYPE app_one gauge\napp_one 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "sibling_identity_mismatch", "warning") {
		t.Fatalf("global siblings share the same global identity: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenDisplayedFamilyMixesEntityIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_global_value, app_server_value]
  charts:
    - title: Global Value
      context: global_value
      units: values
      priority: 100
      dimensions:
        - selector: app_global_value
          name: value
    - title: Server Value
      context: server_value
      units: values
      priority: 110
      instances:
        by_labels: [server]
      dimensions:
        - selector: app_server_value
          name: value
`
	dump := "# TYPE app_global_value gauge\napp_global_value 1\n# TYPE app_server_value gauge\napp_server_value{server=\"a\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("mixed family identity must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "family_identity_mixed", "warning") {
		t.Fatalf("missing mixed displayed-family identity warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenChildDropsDeclaredParentIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_server_value, app_database_value]
  charts:
    - title: Server Value
      family: Servers
      context: server_value
      units: values
      priority: 100
      dimensions:
        - selector: app_server_value
          name: value
    - title: Database Value
      family: Databases
      context: database_value
      units: values
      priority: 110
      instances:
        by_labels: [database]
      dimensions:
        - selector: app_database_value
          name: value
`
	dump := "# TYPE app_server_value gauge\napp_server_value{server=\"sensitive-server\"} 1\n# TYPE app_database_value gauge\napp_database_value{server=\"sensitive-server\",database=\"sensitive-database\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("parent identity loss must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "identity_parent_labels_dropped", "warning") {
		t.Fatalf("missing parent identity loss warning: %#v", result.report.Findings)
	}
	if strings.Contains(result.stdout, "sensitive-") {
		t.Fatalf("identity hierarchy warning leaked an observed label value:\n%s", result.stdout)
	}
}

func TestValidateProfileWarnsWhenGroupOverrideDropsParentIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  groups:
    - family: Databases
      chart_defaults:
        instances:
          by_labels: [database]
      metrics: [app_database_value]
      charts:
        - title: Database Value
          context: database_value
          units: values
          priority: 100
          dimensions:
            - selector: app_database_value
              name: value
`
	dump := "# TYPE app_database_value gauge\napp_database_value{server=\"a\",database=\"main\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("group parent identity loss must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "identity_parent_labels_dropped", "warning") {
		t.Fatalf("missing group parent identity loss warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsMonotonicNestedEntityIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_server_value]
  charts:
    - title: Server Value
      context: server_value
      units: values
      priority: 100
      dimensions:
        - selector: app_server_value
          name: value
  groups:
    - family: Databases
      chart_defaults:
        instances:
          by_labels: [server, database]
      metrics: [app_database_value]
      charts:
        - title: Database Value
          context: database_value
          units: values
          priority: 110
          dimensions:
            - selector: app_database_value
              name: value
      groups:
        - family: Tables
          chart_defaults:
            instances:
              by_labels: [server, database, table]
          metrics: [app_table_value]
          charts:
            - title: Table Value
              context: table_value
              units: values
              priority: 120
              dimensions:
                - selector: app_table_value
                  name: value
`
	dump := "# TYPE app_server_value gauge\napp_server_value{server=\"a\"} 1\n# TYPE app_database_value gauge\napp_database_value{server=\"a\",database=\"main\"} 2\n# TYPE app_table_value gauge\napp_table_value{server=\"a\",database=\"main\",table=\"orders\"} 3\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	for _, code := range []string{
		"family_identity_mixed",
		"identity_parent_labels_dropped",
		"sibling_identity_mismatch",
	} {
		if hasFinding(result.report, code, "warning") {
			t.Fatalf("unexpected %s warning: %#v", code, result.report.Findings)
		}
	}
}

func TestValidateProfileWarnsWhenSiblingFamilyPathIsRepeated(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: Storage
      metrics: [app_reads]
      charts:
        - title: Reads
          context: reads
          units: operations/s
          algorithm: incremental
          priority: 100
          dimensions:
            - selector: app_reads
              name: reads
    - family: Storage
      metrics: [app_writes]
      charts:
        - title: Writes
          context: writes
          units: operations/s
          algorithm: incremental
          priority: 110
          dimensions:
            - selector: app_writes
              name: writes
`
	dump := "# TYPE app_reads counter\napp_reads 1\n# TYPE app_writes counter\napp_writes 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("duplicate sibling family must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "duplicate_sibling_family", "warning") {
		t.Fatalf("missing duplicate sibling family warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsMalformedAndEmptyDumps(t *testing.T) {
	tests := map[string]string{
		"malformed": "# TYPE app_temperature gauge\napp_temperature not-a-number\n",
		"empty":     "# HELP app_temperature no samples follow\n",
	}
	for name, dump := range tests {
		t.Run(name, func(t *testing.T) {
			result := runValidation(t, validProfile, dump, "")
			if result.exitCode != 1 {
				t.Fatalf("expected failure, got %d\nreport:\n%s", result.exitCode, result.stdout)
			}
			if result.report.Verdict != verdictFail {
				t.Fatalf("expected FAIL report, got %q", result.report.Verdict)
			}
		})
	}
}

func TestValidateProfileRejectsDuplicateYAMLKeys(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*\n", "match: app_*\nmatch: app_*\n", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "profile_load")
}

func TestValidateProfileRejectsInvalidMatchAndDimensionSelector(t *testing.T) {
	tests := map[string]struct {
		profile string
		code    string
	}{
		"match": {
			profile: strings.Replace(validProfile, "match: app_*", "match: 'ab\\'", 1),
			code:    "profile_load",
		},
		"selector": {
			profile: strings.Replace(
				validProfile,
				"selector: app_temperature",
				"selector: 'app_temperature{instance=\"node-a\",}'",
				1,
			),
			code: "engine_load",
		},
		"selector RE2 regex": {
			profile: strings.Replace(
				validProfile,
				"selector: app_temperature",
				"selector: 'app_temperature{instance=~\"(?=node)\"}'",
				1,
			),
			code: "engine_load",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := runValidation(t, tc.profile, validDump, "")
			requireFinding(t, result, tc.code)
		})
	}
}

func TestValidateProfileAppliesInheritedInstanceDefaults(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [instance]
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      priority: 100
      dimensions:
        - selector: app_value
          name: value
`
	dump := "# TYPE app_value gauge\napp_value{instance=\"a\"} 1\napp_value{instance=\"b\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("inherited instances should route both observed identities\nreport:\n%s", result.stdout)
	}
	if result.report.Counts.CuratedCharts != 2 {
		t.Fatalf("inherited identity produced %d charts, want 2", result.report.Counts.CuratedCharts)
	}
}

func TestValidateProfileReportsUnavailableInheritedInstanceLabels(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_http_requests]
  charts:
    - title: HTTP Requests
      context: http_requests
      units: requests/s
      algorithm: incremental
      priority: 100
      dimensions:
        - selector: app_http_requests
          name_from_label: status
`
	dump := "# TYPE app_http_requests counter\napp_http_requests{handler=\"sensitive-handler\",status=\"200\"} 1\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "instance_identity_label_unavailable")
	for _, item := range result.report.Findings {
		if item.Code != "instance_identity_label_unavailable" {
			continue
		}
		for _, expected := range []string{"HTTP Requests", "app_http_requests", "server"} {
			if !strings.Contains(item.Message, expected) {
				t.Fatalf("identity finding %q does not contain %q", item.Message, expected)
			}
		}
		if strings.Contains(item.Message, "sensitive-handler") {
			t.Fatalf("identity finding leaked an observed label value: %q", item.Message)
		}
		return
	}
	t.Fatal("missing direct unavailable instance identity finding")
}

func TestValidateProfileInstanceIdentityExclusionWins(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_http_requests]
  charts:
    - title: HTTP Requests
      context: http_requests
      units: requests/s
      algorithm: incremental
      priority: 100
      instances:
        by_labels: [server, "!server", "*"]
      dimensions:
        - selector: app_http_requests
          name_from_label: status
`
	dump := "# TYPE app_http_requests counter\napp_http_requests{handler=\"api\",status=\"200\"} 1\n"
	result := runValidation(t, profile, dump, "")
	for _, item := range result.report.Findings {
		if item.Code == "instance_identity_label_unavailable" {
			t.Fatalf("excluded identity label must not be required: %q", item.Message)
		}
	}
}

func TestValidateProfileRejectsMultipleJobDocuments(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "name: first\n---\nname: second\n")
	requireFinding(t, result, "job_policy")
}

func TestValidateProfileDoesNotMisattributeSelectorDenialToUntypedWriterPolicy(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_live]
  charts:
    - title: Live
      context: live
      units: values
      priority: 100
      dimensions:
        - selector: app_live
          name: live
`
	dump := "# TYPE app_live gauge\napp_live 1\napp_untyped 2\n"
	job := `
selector:
  deny: [app_untyped]
fallback_type:
  gauge: [app_untyped]
`
	result := runValidation(t, profile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("expected PASS with intentional job-policy exclusion\nreport:\n%s", result.stdout)
	}
	if len(result.report.PipelineExcluded) != 1 ||
		result.report.PipelineExcluded[0].Category != "not_materialized_after_job_policy_or_writer" {
		t.Fatalf("selector denial was misattributed: %#v", result.report.PipelineExcluded)
	}
}

func TestValidateProfileReportsPartialWriterMaterialization(t *testing.T) {
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
      priority: 100
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	dump := "# TYPE app_value gauge\napp_value{instance=\"finite\"} 1\napp_value{instance=\"nan\"} NaN\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("one rejected source series should be reported without inventing a profile coverage gap\nreport:\n%s", result.stdout)
	}
	if len(result.report.PipelineExcluded) != 1 {
		t.Fatalf("missing partial pipeline exclusion: %#v", result.report.PipelineExcluded)
	}
	item := result.report.PipelineExcluded[0]
	if item.Category != "writer_partially_materialized_family" ||
		item.RawLogicalSeries != 2 ||
		item.WriterSourceSeries != 1 {
		t.Fatalf("unexpected partial exclusion evidence: %#v", item)
	}
}

func TestRuntimeMetricIntRejectsMissingEvidence(t *testing.T) {
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeMetricInt(engine, "series_scanned_total"); err == nil {
		t.Fatal("missing runtime store must not be interpreted as a zero counter")
	}
}

func TestWriteTextReportReturnsWriterError(t *testing.T) {
	if err := writeTextReport(errorWriter{}, newReport()); err == nil {
		t.Fatal("expected output writer failure")
	}
}

type errorWriter struct{}

func (errorWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

type validationResult struct {
	exitCode int
	stdout   string
	stderr   string
	report   report
}

func runValidation(t *testing.T, profile, dump, job string) validationResult {
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

	args := []string{
		"-test.run=^TestValidatorHelperProcess$",
		"--",
		"--profile", profilePath,
		"--dump", dumpPath,
		"--output", "json",
	}
	if job != "" {
		jobPath := filepath.Join(dir, "job.yaml")
		if err := os.WriteFile(jobPath, []byte(job), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--job", jobPath)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(
		withoutEnvironmentKeys(os.Environ(), "NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR"),
		"NETDATA_PROFILE_VALIDATOR_HELPER=1",
		"NETDATA_USER_CONFIG_DIR=/hostile/ambient/user/config",
		"NETDATA_STOCK_CONFIG_DIR=/hostile/ambient/stock/config",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run helper: %v\nstderr:\n%s", err, stderr.String())
		}
		exitCode = exitErr.ExitCode()
	}

	var got report
	if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
		t.Fatalf("decode report: %v\nstdout:\n%s\nstderr:\n%s", decodeErr, stdout.String(), stderr.String())
	}
	return validationResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		report:   got,
	}
}

func withoutEnvironmentKeys(environ []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	out := make([]string, 0, len(environ))
	for _, item := range environ {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := blocked[key]; !ok {
			out = append(out, item)
		}
	}
	return out
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

func hasFindingPrefix(r report, prefix string) bool {
	for _, item := range r.Findings {
		if strings.HasPrefix(item.Code, prefix) {
			return true
		}
	}
	return false
}

func hasFinding(r report, code, severity string) bool {
	for _, item := range r.Findings {
		if item.Code == code && item.Severity == severity {
			return true
		}
	}
	return false
}
