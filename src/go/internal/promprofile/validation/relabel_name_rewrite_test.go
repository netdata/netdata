// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateProfileRejectsOpenEndedMetricNameRewriteDespiteObservedMatch(t *testing.T) {
	dump := validDump + `
# TYPE app_secret_signal gauge
app_secret_signal{instance="node-b"} 1
`
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_secret_.+
        target_label: __name__
        replacement: app_temperature
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "open_ended_relabel_name_rewrite")
}

func TestValidateProfileDefersOpenEndedMetricNameRewriteOnlyForSemanticCoverage(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_secret_.+
        target_label: __name__
        replacement: app_temperature
        action: replace
`, 1)
	job := `
future_inputs:
  - name: app_secret_future
`

	ordinary := runValidation(t, profile, validDump, job)
	if !hasFinding(ordinary.report, "open_ended_relabel_name_rewrite", "error") {
		t.Fatalf("ordinary validation must reject an open-ended name rewrite: %#v", ordinary.report.Findings)
	}

	semantic := runValidationWithSemanticCoverage(t, profile, validDump, job)
	if semantic.exitCode != 0 || !hasFinding(semantic.report, "open_ended_relabel_name_rewrite", "warning") {
		t.Fatalf("semantic coverage must defer the fixture-local rewrite finding\nreport:\n%s", semantic.stdout)
	}
}

func TestValidateProfileDefersComposedDynamicNameIdentityOnlyForSemanticCoverage(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [worker, instance]
        regex: (.+);(.+)
        target_label: instance
        replacement: ${1}_${2}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
      - regex: worker
        action: labeldrop
`, 1)
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
`
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels: {instance: node-future}
`

	ordinary := runValidation(t, profile, dump, job)
	if !hasFinding(ordinary.report, "unpreserved_relabel_name_identity", "error") {
		t.Fatalf("ordinary validation must reject composed dynamic identity: %#v", ordinary.report.Findings)
	}

	semantic := runValidationWithSemanticCoverage(t, profile, dump, job)
	if semantic.exitCode != 0 || !hasFinding(semantic.report, "unpreserved_relabel_name_identity", "warning") {
		t.Fatalf("semantic coverage must defer composed dynamic identity to exact normalization replay\nreport:\n%s", semantic.stdout)
	}
}

func TestValidateProfileRejectsBoundedMetricNameRewriteWithoutFixtureEvidence(t *testing.T) {
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "unproven_relabel_name_rewrite")
}

func TestValidateProfileDefersBoundedMetricNameRewriteEvidenceForAggregateReplay(t *testing.T) {
	profile := strings.Replace(validProfile, "app: app\n", `app: app
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: __name__
        replacement: app_${2}
        action: replace
`, 1)
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels: {instance: node-future}
`
	result := runValidationWithAggregateEvidence(t, profile, validDump, job, true)
	if result.exitCode != 0 {
		t.Fatalf("aggregate semantic replay defers per-case bounded profile rewrite presence\nreport:\n%s", result.stdout)
	}
}

func TestValidateProfileRequiresEveryBoundedMetricNameRewriteBranchInFixture(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unproven_relabel_name_rewrite")
}

func TestValidateProfileAcceptsBoundedMetricNameRewriteWithCompleteFixtureEvidence(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_requests_total counter
app_worker_beta_requests_total{instance="node-b"} 11
`
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels: {instance: node-future}
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("bounded source-evidenced metric-name rewrites remain valid\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
	if len(result.report.PipelineExcluded) != 0 {
		t.Fatalf("successfully renamed source families were reported as excluded: %#v", result.report.PipelineExcluded)
	}
	if got, want := result.report.Counts.PipelineRenamed, 2; got != want {
		t.Fatalf("renamed source families=%d, want %d: %#v", got, want, result.report.PipelineRenamed)
	}
	for _, want := range []pipelineRenamedReport{
		{
			RawName:                   "app_worker_alpha_temperature",
			FinalNames:                []string{"app_temperature"},
			RawLogicalSeries:          1,
			MaterializedLogicalSeries: 1,
		},
		{
			RawName:                   "app_worker_beta_requests_total",
			FinalNames:                []string{"app_requests_total"},
			RawLogicalSeries:          1,
			MaterializedLogicalSeries: 1,
		},
	} {
		if !slices.ContainsFunc(result.report.PipelineRenamed, func(got pipelineRenamedReport) bool {
			return got.RawName == want.RawName &&
				slices.Equal(got.FinalNames, want.FinalNames) &&
				got.RawLogicalSeries == want.RawLogicalSeries &&
				got.MaterializedLogicalSeries == want.MaterializedLogicalSeries
		}) {
			t.Fatalf("missing rename lineage %#v: %#v", want, result.report.PipelineRenamed)
		}
	}
}

func TestValidateProfileReportsTypedFamilyRenameAsMaterialized(t *testing.T) {
	profile := strings.ReplaceAll(validProfile, "app_latency_seconds", "app_normalized_latency_seconds")
	job := `
relabeling:
  - match: app_latency_seconds_bucket app_latency_seconds_sum app_latency_seconds_count
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_latency_seconds(_bucket|_sum|_count)
        target_label: __name__
        replacement: app_normalized_latency_seconds${1}
        action: replace
`
	result := runValidation(t, profile, validDump, job)
	if result.exitCode != 0 {
		t.Fatalf("typed-family rename should pass\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
	if len(result.report.PipelineExcluded) != 0 {
		t.Fatalf("successfully renamed typed family was reported as excluded: %#v", result.report.PipelineExcluded)
	}
	want := pipelineRenamedReport{
		RawName:                   "app_latency_seconds",
		FinalNames:                []string{"app_normalized_latency_seconds"},
		RawLogicalSeries:          1,
		MaterializedLogicalSeries: 1,
	}
	if !slices.ContainsFunc(result.report.PipelineRenamed, func(got pipelineRenamedReport) bool {
		return got.RawName == want.RawName &&
			slices.Equal(got.FinalNames, want.FinalNames) &&
			got.RawLogicalSeries == want.RawLogicalSeries &&
			got.MaterializedLogicalSeries == want.MaterializedLogicalSeries
	}) {
		t.Fatalf("missing typed-family rename lineage %#v: %#v", want, result.report.PipelineRenamed)
	}
}

func TestValidateProfileRejectsObservedFiniteWildcardRewriteIdentityCollapse(t *testing.T) {
	dump := validDump + `
# TYPE app_alias_a gauge
app_alias_a{instance="node-b"} 43
# TYPE app_alias_b gauge
app_alias_b{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_alias_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_alias_(a|b)
        target_label: __name__
        replacement: app_temperature
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "observed_relabel_identity_collapse")
}

func TestValidateProfileRejectsObservedExactScopeRewriteIdentityCollapse(t *testing.T) {
	dump := validDump + `
# TYPE app_alias_a gauge
app_alias_a{instance="node-b"} 43
# TYPE app_alias_b gauge
app_alias_b{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_alias_a app_alias_b
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_alias_(a|b)
        target_label: __name__
        replacement: app_temperature
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "observed_relabel_identity_collapse")
}

func TestValidateProfileRejectsObservedRelabelLabelIdentityCollapse(t *testing.T) {
	dump := validDump + `
app_temperature{instance="node-b",worker="a"} 43
app_temperature{instance="node-b",worker="b"} 44
`
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - regex: worker
        action: labeldrop
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "observed_relabel_identity_collapse")
}

func TestValidateProfileIgnoresWriterRejectedScalarInRelabelIdentityCollapse(t *testing.T) {
	dump := validDump + `
app_temperature{instance="node-b",worker="a"} NaN
app_temperature{instance="node-b",worker="b"} 44
`
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - regex: worker
        action: labeldrop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("a writer-rejected scalar cannot participate in a writer identity collision\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "observed_relabel_identity_collapse", "error") {
		t.Fatalf("writer-rejected NaN was treated as a materialized identity: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsImplicitInvalidMetricNameDiscard(t *testing.T) {
	dump := validDump + `
app_temperature{instance="node-b",canonical_name="app_temperature"} 44
`
	job := `
relabeling:
  - match: app_temperature
    metric_relabel_configs:
      - source_labels: [canonical_name]
        regex: (.*)
        target_label: __name__
        replacement: ${1}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "invalid_relabel_metric_name_discard")
}

func TestValidateProfilePreservesReplacementSuffixInMutationReachability(t *testing.T) {
	dump := strings.ReplaceAll(validDump, "app_temperature", "app_source")
	dump = strings.Replace(
		dump,
		`app_source{instance="node-a"}`,
		`app_source{instance="node-a",target_prefix="app"}`,
		1,
	) + `
# TYPE app_bad gauge
app_bad{instance="node-b"} 1
`
	job := `
relabeling:
  - match: app_source
    metric_relabel_configs:
      - source_labels: [target_prefix]
        regex: (.+)
        target_label: __name__
        replacement: ${1}_temperature
        action: replace
  - match: app_bad
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_bad
        action: drop
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("literal replacement suffix proves the earlier rewrite cannot reach app_bad\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "tainted_relabel_name_discard", "error") {
		t.Fatalf("disjoint replacement suffix was discarded from reachability: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsUnreachableDynamicIdentityLabelWrite(t *testing.T) {
	dump := validDump + `
# TYPE app_sensor_alpha_temperature gauge
app_sensor_alpha_temperature{instance="node-b"} 43
`
	job := `
future_inputs:
  - name: app_sensor_future_temperature
    labels: {instance: node-future}
relabeling:
  - match: app_sensor_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_sensor_(.+)_(temperature)
        target_label: sensor
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_sensor_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
      - source_labels: [__name__]
        regex: app_other
        target_label: sensor
        replacement: overwritten
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("a name-disjoint later rule cannot overwrite the extracted identity\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "unpreserved_relabel_name_identity", "error") {
		t.Fatalf("name-disjoint label write was treated as reachable: %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsBoundedRewriteToNonAuthoredMetrics(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_requests_total counter
app_worker_beta_requests_total{instance="node-b"} 11
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature|requests_total)
        target_label: __name__
        replacement: renamed_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "open_ended_relabel_name_rewrite")
}

func TestValidateProfileRejectsBoundedRewriteOutputDerivedFromDynamicIdentity(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_app_temperature_old gauge
app_worker_app_temperature_old{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_old
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_old
        target_label: __name__
        replacement: ${1}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "open_ended_relabel_name_rewrite")
}

func TestValidateProfileRejectsDynamicRewriteWithoutIdentityExtraction(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_temperature gauge
app_worker_beta_temperature{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileAcceptsDynamicRewriteWithIdentityExtraction(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_temperature gauge
app_worker_beta_temperature{instance="node-b"} 44
`
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels: {instance: node-future}
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	baseline := runValidation(t, validProfile, validDump, "")
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("identity-preserving dynamic rewrite should pass\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
	if got, want := result.report.Counts.WriterSeries, baseline.report.Counts.WriterSeries+2; got != want {
		t.Fatalf("writer series=%d, want %d after preserving two dynamic identities", got, want)
	}
}

func TestValidateProfileRejectsIncompleteNestedDynamicIdentityExtraction(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_barX_temperature gauge
app_worker_barX_temperature{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(foo|baz|bar(.+))_(temperature)
        target_label: worker
        replacement: ${2}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(foo|baz|bar(.+))_(temperature)
        target_label: __name__
        replacement: app_${3}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileAcceptsCompleteNestedDynamicIdentityExtraction(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_barX_temperature gauge
app_worker_barX_temperature{instance="node-b"} 43
`
	job := `
future_inputs:
  - name: app_worker_barFuture_temperature
    labels: {instance: node-future}
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(foo|baz|bar(.+))_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(foo|baz|bar(.+))_(temperature)
        target_label: __name__
        replacement: app_${3}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("the capture enclosing the complete dynamic grammar region should preserve identity\nstderr:\n%s\nreport:\n%s",
			result.stderr, result.stdout)
	}
}

func TestValidateProfileAcceptsDynamicRewriteWithDisjointLabelMap(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
`
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels: {instance: node-future}
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
      - regex: (instance)
        replacement: copy_${1}
        action: labelmap
`
	baseline := runValidation(t, validProfile, validDump, "")
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("a labelmap with destinations disjoint from the extracted identity label should pass\nstderr:\n%s\nreport:\n%s",
			result.stderr, result.stdout)
	}
	if got, want := result.report.Counts.WriterSeries, baseline.report.Counts.WriterSeries+1; got != want {
		t.Fatalf("writer series=%d, want %d after preserving the dynamic identity", got, want)
	}
}

func TestValidateProfileRejectsDynamicRewriteWithOverwritingLabelMap(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
      - regex: (instance)
        replacement: worker
        action: labelmap
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileRejectsDynamicRewriteAfterIdentityLabelRemoval(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - regex: worker
        action: labeldrop
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileRejectsDynamicRewriteBeforeIdentityLabelRemoval(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_temperature gauge
app_worker_beta_temperature{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
      - regex: worker
        action: labeldrop
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileRejectsDynamicIdentityRemovalInLaterBlock(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_alpha_temperature gauge
app_worker_alpha_temperature{instance="node-b"} 43
# TYPE app_worker_beta_temperature gauge
app_worker_beta_temperature{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
  - match: app_temperature
    metric_relabel_configs:
      - regex: worker
        action: labeldrop
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileRejectsDynamicRewriteThatErasesFiniteBranch(t *testing.T) {
	dump := validDump + `
# TYPE app_a_x_temperature gauge
app_a_x_temperature{instance="node-b"} 43
# TYPE app_b_x_temperature gauge
app_b_x_temperature{instance="node-b"} 44
`
	job := `
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_(a|b)_(.+)_(temperature)
        target_label: worker
        replacement: ${2}
        action: replace
      - source_labels: [__name__]
        regex: app_(a|b)_(.+)_(temperature)
        target_label: __name__
        replacement: app_${3}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileRejectsDynamicRewriteThatOverwritesSourceLabel(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_x_temperature gauge
app_worker_x_temperature{instance="node-b",worker="original-a"} 43
app_worker_x_temperature{instance="node-b",worker="original-b"} 44
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "unpreserved_relabel_name_identity")
}

func TestValidateProfileAcceptsSourceProvenCanonicalDynamicTailRewrite(t *testing.T) {
	dump := validDump + `
# TYPE app_temperature_sensor_a gauge
app_temperature_sensor_a{instance="node-b"} 43
`
	job := `
future_inputs:
  - name: app_temperature_sensor_future
    labels: {instance: node-future}
relabeling:
  - match: app_temperature_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_temperature_(.+)
        target_label: sensor
        replacement: ${1}
        action: replace
      - source_labels: [__name__]
        regex: app_temperature_(.+)
        target_label: __name__
        replacement: app_temperature
        action: replace
`
	result := runValidation(t, validProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("a source-evidenced canonical dynamic tail may normalize to its fixed metric prefix\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}
