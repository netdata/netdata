// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"testing"
)

func TestValidateProfileRejectsDiscardAfterExactLabelMapMetricNameRewrite(t *testing.T) {
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: metric_name
        replacement: __name__
        action: labelmap
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_never_exported_.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "tainted_relabel_name_discard")
}

func TestValidateProfileRejectsExactScopeExceptionAfterEarlierMetricRename(t *testing.T) {
	job := `
relabeling:
  - match: app_future_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_future_.+
        target_label: __name__
        replacement: app_temperature
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        target_label: __name__
        replacement: ''
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unbounded_metric_name_rewrite")
}

func TestValidateProfileAcceptsDisjointExactRelabelPaths(t *testing.T) {
	dump := validDump + `
# TYPE app_never gauge
app_never 1
# TYPE app_other gauge
app_other{kind="private"} 1
`
	job := `
relabeling:
  - match: app_never
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        target_label: __name__
        replacement: app_normalized
      - source_labels: [__name__]
        regex: app_never
        action: drop
  - match: app_other
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("disjoint exact blocks retain their original bounded scopes\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsDiscardAfterSameBlockLabelDerivedMetricRename(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [tenant]
        regex: (.+)
        target_label: __name__
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: private-.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "tainted_relabel_name_discard")
	if hasFinding(result.report, "future_metric_blocked_by_job_relabel", "error") {
		t.Fatalf("empty-label canaries do not take the label-derived rename path: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsDiscardAfterCrossBlockLabelDerivedMetricRename(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [tenant]
        regex: (.+)
        target_label: __name__
        replacement: ${1}
  - match: private-*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: private-.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "tainted_relabel_name_discard")
	if hasFinding(result.report, "future_metric_blocked_by_job_relabel", "error") {
		t.Fatalf("empty-label canaries do not take the cross-block rename path: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsDiscardAfterChainedTaintedMetricRename(t *testing.T) {
	job := `
relabeling:
  - match: app_alpha
    metric_relabel_configs:
      - source_labels: [tenant]
        regex: (.+)
        target_label: __name__
        replacement: bridge_${1}
      - source_labels: [__name__]
        regex: bridge_(.+)
        target_label: __name__
        replacement: final_${1}
  - match: final_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: final_.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "tainted_relabel_name_discard")
}

func TestValidateProfileAcceptsDiscardAfterBoundedMetricNameDerivedRename(t *testing.T) {
	dump := validDump + `
# TYPE app_deprecated gauge
app_deprecated 1
`
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: (app_deprecated)
        target_label: __name__
        replacement: ${1}
      - source_labels: [__name__]
        regex: app_deprecated
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("name-only rewrite preserves discard provenance\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}
