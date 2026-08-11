// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"slices"
	"strings"
	"testing"
)

func TestUncoveredWildcardProfileTerms(t *testing.T) {
	tests := map[string]struct {
		profile string
		allows  []string
		want    []string
	}{
		"no allowlist is open": {
			profile: `app_*`,
		},
		"matching term": {
			profile: `app_*`,
			allows:  []string{`app_*`},
		},
		"universal term": {
			profile: `app_* service_*`,
			allows:  []string{`*`},
		},
		"one allow expression can carry every positive term": {
			profile: `app_* service_*`,
			allows:  []string{`app_* service_*`},
		},
		"exact profile expression preserves its exclusions": {
			profile: `!app_debug_* app_*`,
			allows:  []string{`!app_debug_* app_*`},
		},
		"positive superset preserves an excluded profile namespace": {
			profile: `!app_debug_* app_*`,
			allows:  []string{`app_*`},
		},
		"label constraint is not namespace coverage": {
			profile: `app_*`,
			allows:  []string{`app_*{environment="production"}`},
			want:    []string{`app_*`},
		},
		"negative allow term closes a subnamespace": {
			profile: `app_*`,
			allows:  []string{`!app_private_* app_*`},
			want:    []string{`app_*`},
		},
		"finite public canaries are not structural coverage": {
			profile: `app_*`,
			allows: []string{
				`app_netdata_future_metric_*`,
				`app_upstream_added_metric_*`,
				`app_exporter_new_signal_*`,
			},
			want: []string{`app_*`},
		},
		"exact profile has no future namespace": {
			profile: `app_temperature`,
			allows:  []string{`app_temperature`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := uncoveredWildcardProfileTerms(tc.profile, tc.allows); !slices.Equal(got, tc.want) {
				t.Fatalf("uncovered terms: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateProfileFutureRunPreservesNaNSummaryQuantile(t *testing.T) {
	dump := strings.Replace(
		validDump,
		`app_size_bytes{instance="node-a",quantile="0.5"} 100`,
		`app_size_bytes{instance="node-a",quantile="0.5"} NaN`,
		1,
	)
	result := runValidation(t, validProfile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("an unchanged NaN summary quantile must survive the future run\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "future_run_changed_current_evidence", "error") {
		t.Fatalf("unchanged NaN evidence was reported as changed: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsOpenEndedJobSelectorDenyWithoutObservedCoverageGap(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  deny: ['app_*_created']\n")
	requireFinding(t, result, "open_ended_job_selector_deny")
}

func TestValidateProfileRejectsExactJobSelectorDenyWithoutFixtureEvidence(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  deny: [app_absent]\n")
	requireFinding(t, result, "unproven_job_selector_deny")
}

func TestValidateProfileAggregateReplayStillRequiresJobSelectorEvidence(t *testing.T) {
	result := runValidationWithAggregateEvidence(t, validProfile, validDump, "selector:\n  deny: [app_absent]\n", true)
	requireFinding(t, result, "unproven_job_selector_deny")
}

func TestValidateProfileRejectsLabelConstrainedJobSelectorDeny(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  deny: ['app_temperature{environment=\"future\"}']\n")
	requireFinding(t, result, "open_ended_job_selector_deny")
}

func TestValidateProfileRejectsRelabelDropOfFutureFamilyWithoutObservedCoverageGap(t *testing.T) {
	job := `
relabeling:
  - match: app_netdata_future_metric_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_netdata_future_metric_.*
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_blocked_by_job_relabel")
}

func TestValidateProfileRejectsRelabelDropOfFutureSubnamespace(t *testing.T) {
	job := `
relabeling:
  - match: app_internal_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_internal_.*
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_blocked_by_job_relabel")
}

func TestValidateProfileRejectsFutureAffectingRelabelBlockWithoutCanary(t *testing.T) {
	job := `
relabeling:
  - match: '!app_netdata_future_metric_* !app_upstream_added_metric_* !app_exporter_new_signal_* app_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "open_ended_relabel_name_discard")
}

func TestValidateProfileAcceptsUnprobedLabelOnlyRelabelBlock(t *testing.T) {
	job := `
relabeling:
  - match: '!app_netdata_future_metric_* !app_upstream_added_metric_* !app_exporter_new_signal_* app_*'
    metric_relabel_configs:
      - regex: internal_.+
        action: labeldrop
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("label-only relabeling cannot hide a future metric family\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsUnprobedFutureAffectingDisjointRelabelBlock(t *testing.T) {
	job := `
relabeling:
  - match: '!other_netdata_future_metric_* !other_upstream_added_metric_* !other_exporter_new_signal_* other_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: other_.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("a provably disjoint relabel namespace cannot hide an app family\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsRelabelScopeExcludedByProfile(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: '!app_debug_* app_*'", 1)
	job := `
relabeling:
  - match: app_debug_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_debug_.+
        action: drop
`
	result := runValidation(t, profile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("profile exclusions make the relabel scope provably disjoint\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsRelabelScopeExcludedByOrderedCharacterClassNegative(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: '!app_[ab]* app_*'", 1)
	job := `
relabeling:
  - match: app_a*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_a.+
        action: drop
`
	result := runValidation(t, profile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("ordered negative character-class scope makes the relabel block disjoint\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsDisjointCharacterClassRelabelScope(t *testing.T) {
	profile := strings.ReplaceAll(validProfile, "app_", "app_a")
	dump := strings.ReplaceAll(validDump, "app_", "app_a")
	job := `
relabeling:
  - match: app_[b]*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_b.+
        action: drop
`
	result := runValidation(t, profile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("character classes prove the two wildcard scopes disjoint\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsRelabelTermWithoutCanaryCoverage(t *testing.T) {
	job := `
relabeling:
  - match: '!app_hidden_netdata_future_metric_* !app_hidden_upstream_added_metric_* !app_hidden_exporter_new_signal_* app_safe_* app_hidden_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_hidden_.+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_blocked_by_job_relabel")
}

func TestValidateProfileRejectsLabelDependentDiscardUnderWildcardRelabelScope(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [kind]
        regex: .+
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unbounded_relabel_discard")
	if hasFinding(result.report, "future_metric_blocked_by_job_relabel", "error") {
		t.Fatalf("empty-label canaries do not exercise this policy; the structural check must catch it: %#v", result.report.Findings)
	}
}

func TestValidateProfileDefersUnboundedRelabelDiscardOnlyForSemanticCoverage(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [kind]
        regex: .+
        action: drop
`, 1)

	ordinary := runValidation(t, profile, validDump, "")
	if !hasFinding(ordinary.report, "unbounded_relabel_discard", "error") {
		t.Fatalf("ordinary validation must reject an unbounded label discard: %#v", ordinary.report.Findings)
	}

	semantic := runValidationWithSemanticCoverage(t, profile, validDump, "")
	if semantic.exitCode != 0 || !hasFinding(semantic.report, "unbounded_relabel_discard", "warning") {
		t.Fatalf("semantic coverage must defer the fixture-local discard finding\nreport:\n%s", semantic.stdout)
	}
}

func TestValidateProfileAcceptsSourceProvenLabelDependentDiscardUnderExactRelabelScope(t *testing.T) {
	dump := validDump + `
# TYPE app_private gauge
app_private{kind="private"} 1
`
	job := `
relabeling:
  - match: app_private
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("an exact known metric block with exercised source evidence bounds label-dependent discard\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsExactRelabelScopeWithoutFixtureEvidence(t *testing.T) {
	job := `
relabeling:
  - match: app_absent
    metric_relabel_configs:
      - regex: internal_.+
        action: labeldrop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unproven_exact_relabel_scope")
}

func TestValidateProfileDefersExactProfileRelabelScopeEvidenceForAggregateReplay(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_absent
    metric_relabel_configs:
      - regex: internal_.+
        action: labeldrop
`, 1)
	result := runValidationWithAggregateEvidence(t, profile, validDump, "", true)
	if result.exitCode != 0 {
		t.Fatalf("aggregate semantic replay defers per-case exact profile scope presence\nreport:\n%s", result.stdout)
	}
}

func TestValidateProfileRejectsExactRelabelDiscardWithoutMatchingFixtureEvidence(t *testing.T) {
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        action: drop
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unproven_exact_relabel_discard")
}

func TestValidateProfileDefersExactProfileRelabelDiscardEvidenceForAggregateReplay(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        action: drop
`, 1)
	result := runValidationWithAggregateEvidence(t, profile, validDump, "", true)
	if result.exitCode != 0 {
		t.Fatalf("aggregate semantic replay defers per-case exact profile discard presence\nreport:\n%s", result.stdout)
	}
}

func TestValidateProfileRejectsLabelDerivedMetricNameRewriteUnderWildcardScope(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [kind]
        regex: private
        target_label: __name__
        replacement: ''
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unbounded_metric_name_rewrite")
	if hasFinding(result.report, "future_metric_blocked_by_job_relabel", "error") {
		t.Fatalf("empty-label canaries do not take the conditional invalid-name path: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsLabelDerivedMetricNameRewriteUnderExactScope(t *testing.T) {
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [kind]
        regex: (.+)
        target_label: __name__
        replacement: app_temperature_${1}
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("an exact known metric block bounds label-derived name rewriting\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsLabelMapMetricNameRewriteUnderWildcardScope(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: metric_name
        replacement: __name__
        action: labelmap
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unbounded_metric_name_rewrite")
}

func TestValidateProfileAcceptsLabelMapMetricNameRewriteUnderExactScope(t *testing.T) {
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - regex: metric_name
        replacement: __name__
        action: labelmap
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("an exact original metric block bounds labelmap name rewriting\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsSafeFiniteMetricNameTemplates(t *testing.T) {
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - regex: (instance|family)
        replacement: $1
        action: labelmap
      - source_labels: [__name__]
        regex: node-(a)
        target_label: app_$1
        replacement: normalized
        action: replace
`
	result := runValidation(t, validProfile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("finite regex outputs cannot create __name__\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileRejectsFutureMetricRoutedToAuthoredMetric(t *testing.T) {
	job := `
future_inputs:
  - name: app_future_signal
    labels: {instance: node-future}
relabeling:
  - match: '!app_temperature !app_requests_total !app_latency_seconds_* !app_size_bytes* app_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_.+
        target_label: __name__
        replacement: app_temperature
        action: replace
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_routed_to_authored_metric")
}

func TestValidateProfileRejectsFutureMetricIdentityCollapse(t *testing.T) {
	job := `
future_inputs:
  - name: app_future_one
  - name: app_future_two
relabeling:
  - match: '!app_temperature !app_requests_total !app_latency_seconds_* !app_size_bytes* app_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_.+
        target_label: __name__
        replacement: app_generic
        action: replace
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_identity_collapse")
}

func TestValidateProfileChecksFallbackAfterRelabeling(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: [app_suppressed]\n",
		1,
	)
	job := `
future_inputs:
  - name: app_future_signal
relabeling:
  - match: '!app_temperature !app_requests_total !app_latency_seconds_* !app_size_bytes* app_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_.+
        target_label: __name__
        replacement: app_suppressed
        action: replace
`
	result := runValidation(t, profile, validDump, job)
	requireFinding(t, result, "future_metric_blocked_by_profile")
}
