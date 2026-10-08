// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateProfileAcceptsAndPropagatesExplicitPriority(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"    - title: Temperature\n",
		"    - title: Temperature\n      priority: 100\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("explicit chart priority must pass\nreport:\n%s", result.stdout)
	}
	found := false
	for _, chart := range result.report.Charts {
		if chart.Title != "Temperature" {
			continue
		}
		found = true
		if chart.Priority != 100 {
			t.Fatalf("authored priority was not propagated: got %d, want 100: %#v", chart.Priority, chart)
		}
	}
	if !found {
		t.Fatal("Temperature chart was not reported")
	}
}

func TestValidateProfileAcceptsAndPropagatesGroupDefaultPriority(t *testing.T) {
	profile := strings.Replace(validProfile, "  chart_defaults:\n", "  chart_defaults:\n    priority: 100\n", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("group default priority must pass\nreport:\n%s", result.stdout)
	}
	for _, chart := range result.report.Charts {
		if chart.Priority != 100 {
			t.Fatalf("inherited priority was not propagated: got %d, want 100: %#v", chart.Priority, chart)
		}
	}
}

func TestValidateProfileUsesOneRuntimePriorityWhenPrioritiesAreOmitted(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("omitted priorities must pass\nreport:\n%s", result.stdout)
	}
	for _, chart := range result.report.Charts {
		if chart.Priority != 70000 {
			t.Fatalf("runtime chart priority: got %d, want 70000: %#v", chart.Priority, chart)
		}
	}
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

func TestValidateProfileAuditsEverySampleDiscardingRelabelAction(t *testing.T) {
	dump := validDump + `
# TYPE app_drop gauge
app_drop{identity="sensitive-drop"} 1
# TYPE app_keep gauge
app_keep{identity="sensitive-keep",mode="discard"} 1
# TYPE app_dropequal gauge
app_dropequal{identity="sensitive-dropequal",left="same",right="same"} 1
# TYPE app_keepequal gauge
app_keepequal{identity="sensitive-keepequal",left="one",right="two"} 1
`
	job := `
relabeling:
  - match: app_drop
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_drop
        action: drop
  - match: app_keep
    metric_relabel_configs:
      - source_labels: [mode]
        regex: retain
        action: keep
  - match: app_dropequal
    metric_relabel_configs:
      - source_labels: [left]
        target_label: right
        action: dropequal
  - match: app_keepequal
    metric_relabel_configs:
      - source_labels: [left]
        target_label: right
        action: keepequal
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 1 {
		t.Fatalf("inverse keep filters must fail contributor validation\nreport:\n%s", result.stdout)
	}
	var closedFilters int
	var findings []finding
	for _, item := range result.report.Findings {
		if item.Code == "closed_relabel_filter" {
			closedFilters++
		}
		if item.Code == "job_relabel_discard_review" {
			findings = append(findings, item)
		}
	}
	if closedFilters != 2 {
		t.Fatalf("closed relabel filters: got %d, want 2: %#v", closedFilters, result.report.Findings)
	}
	if len(findings) != 4 {
		t.Fatalf("discard findings: got %d, want 4: %#v", len(findings), findings)
	}
	for i, item := range findings {
		if !strings.Contains(item.Path, fmt.Sprintf("relabeling[%d]", i)) ||
			!strings.Contains(item.Message, "removes 1 observed logical identities") {
			t.Fatalf("unexpected relabel discard finding %d: %#v", i, item)
		}
	}
	if strings.Contains(result.stdout, "sensitive-") {
		t.Fatalf("relabel discard review leaked observed label values:\n%s", result.stdout)
	}
}

func TestValidateProfileAuditsUnobservedSampleDiscardingRelabelAction(t *testing.T) {
	job := `
relabeling:
  - match: future_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: future_.*
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("unobserved discard review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	var found *finding
	for i := range result.report.Findings {
		if result.report.Findings[i].Code == "job_relabel_discard_review" {
			found = &result.report.Findings[i]
			break
		}
	}
	if found == nil || !strings.Contains(found.Message, "dropped no samples") {
		t.Fatalf("missing unobserved discard review: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsOpenEndedWriterSkippedInfoDeny(t *testing.T) {
	dump := validDump + `
# TYPE app_build_info gauge
app_build_info{version="test"} 1
`
	result := runValidation(t, validProfile, dump, "selector:\n  deny: ['*_info']\n")
	requireFinding(t, result, "open_ended_job_selector_deny")
	if hasFinding(result.report, "job_deny_review", "warning") {
		t.Fatalf(
			"writer-skipped info family should not be presented as lost chart surface: %#v",
			result.report.Findings,
		)
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
