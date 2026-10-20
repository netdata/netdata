// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"strings"
	"testing"
)

func TestValidateProfileRejectsOpenEndedMetricNameDiscardDespiteObservedMatch(t *testing.T) {
	dump := validDump + `
# TYPE app_signal gauge
app_signal 1
`
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_s.+
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "open_ended_relabel_name_discard")
}

func TestValidateProfileRejectsBoundedMetricNameDiscardWithoutFixtureEvidence(t *testing.T) {
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_.+_(deprecated|legacy)
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unproven_relabel_name_discard")
}

func TestValidateProfileDefersBoundedMetricNameDiscardEvidenceForAggregateReplay(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_.+_created
        action: drop
`, 1)
	result := runValidationWithAggregateEvidence(t, profile, validDump, "", true)
	if result.exitCode != 0 {
		t.Fatalf("aggregate semantic replay defers per-case profile source presence\nreport:\n%s", result.stdout)
	}
}

func TestValidateProfileRequiresEveryBoundedMetricNameDiscardBranchInFixture(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_deprecated gauge
app_worker_alpha_deprecated 1
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_.+_(deprecated|legacy)
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unproven_relabel_name_discard")
}

func TestValidateProfileAcceptsBoundedMetricNameDiscardWithCompleteFixtureEvidence(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_deprecated gauge
app_worker_alpha_deprecated 1
# TYPE app_worker_beta_legacy gauge
app_worker_beta_legacy 1
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_.+_(deprecated|legacy)
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("bounded source-evidenced alias drops remain valid\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsBoundedMetricNameDiscardWhenBlockScopeIsTheExcludedClass(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_created gauge
app_worker_alpha_created 1
`
	job := `
relabeling:
  - match: app_*_created
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_.+_created
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("bounded excluded classes do not require surviving future witnesses\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsWildcardMetricNameDropEqual(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        target_label: __name__
        action: dropequal
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "open_ended_relabel_name_discard")
}

func TestValidateProfileRejectsClosedRelabelFilterWithoutWildcardScope(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: app_temperature", 1)
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_temperature
        action: keep
`
	result := runValidation(t, profile, validDump, job)
	requireFinding(t, result, "closed_relabel_filter")
}

func TestValidateProfileAcceptsUTF8MetricAndLabelNames(t *testing.T) {
	profile := `
match: 'my.noncompliant.*'
app: utf8
template:
  family: Test
  context_namespace: utf8
  chart_defaults:
    instances:
      by_labels: ['label.name']
  metrics: ['my.noncompliant.metric']
  charts:
    - title: UTF-8 Metric
      context: metric
      units: value
      dimensions:
        - selector: 'my.noncompliant.metric'
          name: value
`
	dump := `
# HELP "my.noncompliant.metric" help text
# TYPE "my.noncompliant.metric" gauge
{"my.noncompliant.metric","label.name"="value"} 1
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("UTF-8-valid metric/label names must follow production semantics\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}
