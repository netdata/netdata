// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
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
      priority: 100
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
      priority: 100
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
	if result.report.Profile.FutureMetricCanary != "app_netdata_future_metric_0" {
		t.Fatalf("future metric canary: got %q", result.report.Profile.FutureMetricCanary)
	}
	if result.report.Counts.RawFamilies != 4 {
		t.Fatalf("raw family count: got %d, want 4", result.report.Counts.RawFamilies)
	}
	helpByFamily := make(map[string]string, len(result.report.RawFamilies))
	for _, family := range result.report.RawFamilies {
		helpByFamily[family.Name] = family.Help
	}
	if got := helpByFamily["app_temperature"]; got != "Current temperature." {
		t.Fatalf("raw family inventory lost HELP source evidence: got %q", got)
	}
	if result.report.Counts.SeriesAutogen != 0 || result.report.Counts.SeriesUnmatched != 0 {
		t.Fatalf("unexpected routing gaps: %#v", result.report.Counts)
	}
	if result.report.Counts.AuthoredCharts != 4 || result.report.Counts.CuratedCharts != 4 {
		t.Fatalf("unexpected chart counts: %#v", result.report.Counts)
	}
	if len(result.report.AuthoredMapping) != 4 {
		t.Fatalf("authored mapping count: got %d, want 4", len(result.report.AuthoredMapping))
	}
	first := result.report.AuthoredMapping[0]
	if first.DisplayedFamily != "Example" ||
		first.Title != "Temperature" ||
		first.Type != "line" ||
		first.Priority != 100 ||
		!slices.Equal(first.InstanceByLabels, []string{"instance"}) {
		t.Fatalf("unexpected first authored mapping: %#v", first)
	}
	if len(first.Dimensions) != 1 ||
		first.Dimensions[0].Selector != "app_temperature" ||
		first.Dimensions[0].Name != "temperature" {
		t.Fatalf("unexpected authored dimension mapping: %#v", first.Dimensions)
	}
	if result.report.AuthoredMapping[2].Dimensions[0].NameFromLabel != "le" {
		t.Fatalf("authored mapping lost dynamic naming mechanism: %#v", result.report.AuthoredMapping[2].Dimensions)
	}
	if result.report.AuthoredMapping[1].Priority != 110 ||
		result.report.AuthoredMapping[2].Priority != 120 ||
		result.report.AuthoredMapping[3].Priority != 130 {
		t.Fatalf("authored mapping lost source order: %#v", result.report.AuthoredMapping)
	}
	if !hasFinding(result.report, "default_validation_job", "warning") {
		t.Fatalf("missing warning that the deployable job policy was not validated: %#v", result.report.Findings)
	}
	for _, chart := range result.report.Charts {
		if chart.IDFingerprint == "" {
			t.Fatalf("materialized chart ID fingerprint is empty: %#v", chart)
		}
	}
}

func TestStockProfileProofsPass(t *testing.T) {
	goRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		fixture string
	}{
		"ceph": {
			fixture: "prometheus/profiles/ceph/fixtures/ceph_all_metrics.prom",
		},
		"litellm": {
			fixture: "prometheus/profiles/litellm/fixtures/litellm_all_metrics.prom",
		},
		"vllm": {
			fixture: "prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom",
		},
		"vllm_ray": {
			fixture: "prometheus/profiles/vllm_ray/fixtures/vllm_ray_all_metrics.prom",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			profilePath := filepath.Join(goRoot, "plugin/go.d/config/go.d/prometheus.profiles/default", name+".yaml")
			jobPath := filepath.Join(goRoot, "plugin/go.d/collector/prometheus/profile-proofs", name, "VALIDATION-JOB.yaml")
			dumpPath := promtestdata.Require(t, test.fixture)
			result := runValidationFiles(t, profilePath, dumpPath, jobPath)
			if result.exitCode != 0 || result.report.Verdict != verdictPass {
				t.Fatalf("stock profile proof failed\nexit code: %d\nstderr:\n%s\nreport:\n%s",
					result.exitCode, result.stderr, result.stdout)
			}
		})
	}
}

func TestFingerprintIDUsesStableRedactedReportFormat(t *testing.T) {
	const raw = "label-derived-sensitive-value"
	got := fingerprintID(raw)
	if got != fingerprintID(raw) {
		t.Fatalf("fingerprint is not deterministic: %q", got)
	}
	if got == fingerprintID(raw+"-different") {
		t.Fatalf("distinct inputs produced the same test fingerprint: %q", got)
	}
	if strings.Contains(got, raw) {
		t.Fatalf("fingerprint leaked its raw input: %q", got)
	}
	const prefix = "sha256:"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("fingerprint prefix: got %q, want %q", got, prefix)
	}
	digest := strings.TrimPrefix(got, prefix)
	if len(digest) != 16 {
		t.Fatalf("fingerprint digest length: got %d, want 16", len(digest))
	}
	if _, err := hex.DecodeString(digest); err != nil {
		t.Fatalf("fingerprint digest is not hexadecimal: %q: %v", digest, err)
	}
}

func TestValidateProfileReportsComposedDisplayedFamily(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"    - title: Temperature\n",
		"    - title: Temperature\n      family: Environment\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	if got := result.report.AuthoredMapping[0].DisplayedFamily; got != "Example/Environment" {
		t.Fatalf("displayed family: got %q, want %q", got, "Example/Environment")
	}
}

func TestValidateProfileRejectsUnsafeJobFields(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "url: http://example.invalid/metrics\n")
	requireFinding(t, result, "job_policy")
}

func TestValidateProfileAppliesRelabelAndFallbackPolicy(t *testing.T) {
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
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, job)
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

func TestValidateProfileRejectsClosedFallbackWithoutObservedCoverageGap(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    allow: [app_temperature]\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "closed_profile_fallback")
	requireFinding(t, result, "future_metric_blocked_by_profile")
	if hasFinding(result.report, "unexpected_autogen", "error") {
		t.Fatalf("the current-source fixture has no coverage gap: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsExplicitFallbackSuppression(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: [app_extra]\n",
		1,
	)
	dump := validDump + "\n# TYPE app_extra gauge\napp_extra{instance=\"node-a\"} 1\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("explicitly suppressed fallback should pass\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
	if !hasFinding(result.report, "profile_suppressed_series", "warning") {
		t.Fatalf("missing profile_suppressed_series warning in %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsExactFallbackSuppressionWithoutFixtureEvidence(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: [app_absent]\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "unproven_profile_fallback_deny")
}

func TestValidateProfileRejectsLabelConstrainedFallbackDeny(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: ['app_temperature{environment=\"future\"}']\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "open_ended_profile_fallback_deny")
}

func TestValidateProfileRejectsOpenEndedFallbackDenyWithoutObservedCoverageGap(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: ['app_*']\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "open_ended_profile_fallback_deny")
}

func TestValidateProfileRejectsJobSelectorClosedToFutureFamily(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  allow: [app_temperature, app_requests_total, app_latency_seconds, app_size_bytes]\n")
	requireFinding(t, result, "future_metric_blocked_by_job_selector")
}

func TestValidateProfileRejectsJobSelectorThatOnlyAdmitsPredictableCanary(t *testing.T) {
	job := "selector:\n  allow: [app_temperature, app_requests_total, app_latency_seconds, app_size_bytes, 'app_netdata_future_metric_*']\n"
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_metric_blocked_by_job_selector")
}

func TestValidateProfileRejectsJobSelectorThatAdmitsEveryPublicCanary(t *testing.T) {
	job := `
selector:
  allow:
    - app_temperature
    - app_requests_total
    - app_latency_seconds
    - app_size_bytes
    - 'app_netdata_future_metric_*'
    - 'app_upstream_added_metric_*'
    - 'app_exporter_new_signal_*'
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "closed_job_selector_allow")
	if hasFinding(result.report, "future_metric_blocked_by_job_selector", "error") {
		t.Fatalf("all finite canaries passed; structural namespace coverage must catch the closed allowlist: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsJobSelectorOpenToFutureFamily(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  allow: ['app_*']\n")
	if result.exitCode != 0 {
		t.Fatalf("namespace-wide job selector should admit future family\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

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

func TestValidateProfileRejectsOpenEndedJobSelectorDenyWithoutObservedCoverageGap(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  deny: ['app_*_created']\n")
	requireFinding(t, result, "open_ended_job_selector_deny")
}

func TestValidateProfileRejectsExactJobSelectorDenyWithoutFixtureEvidence(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  deny: [app_absent]\n")
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
	requireFinding(t, result, "future_relabel_canary_unavailable")
	if hasFinding(result.report, "future_metric_blocked_by_job_relabel", "error") {
		t.Fatalf("the block excludes every public canary; the unavailable-probe check must catch it: %#v", result.report.Findings)
	}
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

func TestSimpleGlobPatternsIntersectOnMetricName(t *testing.T) {
	tests := map[string]struct {
		left  string
		right string
		want  bool
	}{
		"disjoint character class": {left: "app_a*", right: "app_[b]*"},
		"overlapping character class": {
			left: "app_[ab]*", right: "app_b*", want: true,
		},
		"negated character class": {
			left: "app_?", right: "app_[^a]", want: true,
		},
		"stars can match empty": {
			left: "app_*", right: "app_", want: true,
		},
		"invalid metric initial character": {left: "1*", right: "1*"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := simpleGlobPatternsIntersectOnMetricName(tc.left, tc.right)
			if !ok {
				t.Fatal("valid simple globs were not parsed")
			}
			if got != tc.want {
				t.Fatalf("intersection: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSimplePatternScopesMayOverlapHonorsOrderedNegatives(t *testing.T) {
	tests := map[string]struct {
		left  string
		right string
		want  bool
	}{
		"character class intersection excluded": {
			left: `!app_[ab]* app_*`, right: `app_a*`, want: false,
		},
		"union of earlier negatives excludes intersection": {
			left: `!app_a* !app_b* app_*`, right: `app_[ab]*`, want: false,
		},
		"one character class branch remains": {
			left: `!app_a* app_*`, right: `app_[ab]*`, want: true,
		},
		"earlier positive wins over later negative": {
			left: `app_a* !app_a* app_*`, right: `app_a*`, want: true,
		},
		"earlier negative wins over later positive": {
			left: `!app_a* app_a*`, right: `app_a*`, want: false,
		},
		"negatives from both operands exclude intersection": {
			left: `!app_a* app_*`, right: `!app_b* app_[ab]*`, want: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := simplePatternScopesMayOverlap(tc.left, tc.right); got != tc.want {
				t.Fatalf("scope overlap: got %v, want %v", got, tc.want)
			}
		})
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
	requireFinding(t, result, "future_relabel_canary_unavailable")
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

func TestRelabelRuleMayWriteMetricName(t *testing.T) {
	tests := map[string]struct {
		action      relabel.Action
		regex       string
		target      string
		replacement string
		want        bool
	}{
		"static labelmap destination": {
			action: relabel.LabelMap, regex: "metric_name", replacement: "__name__", want: true,
		},
		"finite safe labelmap captures": {
			action: relabel.LabelMap, regex: "(instance|family)", replacement: "$1",
		},
		"finite reachable labelmap captures": {
			action: relabel.LabelMap, regex: "(name|instance)", replacement: "__${1}__", want: true,
		},
		"identity labelmap excludes metric name input": {
			action: relabel.LabelMap, regex: "(.*)", replacement: "$1",
		},
		"incompatible replace target prefix": {
			action: relabel.Replace, regex: "node-(a)", target: "app_$1",
		},
		"finite safe replace target": {
			action: relabel.Replace, regex: "(instance|family)", target: "__${1}__",
		},
		"finite reachable replace target": {
			action: relabel.Replace, regex: "(name|instance)", target: "__${1}__", want: true,
		},
		"literal safe label": {
			action: relabel.Replace, regex: "(.*)", target: "instance",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rule := relabel.Config{
				Regex:       relabel.MustNewRegexp(tc.regex),
				TargetLabel: tc.target,
				Replacement: tc.replacement,
				Action:      tc.action,
			}
			if got := relabelRuleMayWriteMetricName(rule, tc.action); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRelabelLabelMapMayOverwriteProtectedLabel(t *testing.T) {
	tests := map[string]struct {
		regex       string
		replacement string
		want        bool
	}{
		"finite disjoint destination": {
			regex: "(instance)", replacement: "copy_${1}",
		},
		"dynamic disjoint prefix": {
			regex: "(.+)", replacement: "copy_${1}",
		},
		"dynamic disjoint suffix": {
			regex: "(.+)", replacement: "${1}_copy",
		},
		"finite self-map": {
			regex: "(worker)", replacement: "${1}",
		},
		"infinite identity map": {
			regex: "(.*)", replacement: "${1}",
		},
		"static overwrite from another label": {
			regex: "(instance)", replacement: "worker", want: true,
		},
		"finite branch can overwrite": {
			regex: "(worker|instance)", replacement: "worker", want: true,
		},
		"unresolved dynamic destination": {
			regex: "(.+)", replacement: "${1}er", want: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rule := relabel.Config{
				Regex:       relabel.MustNewRegexp(tc.regex),
				Replacement: tc.replacement,
				Action:      relabel.LabelMap,
			}
			if got := relabelTemplateMayExpandToLabelName(
				rule, relabel.LabelMap, rule.Replacement, "worker",
			); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
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

func TestFiniteRegexpReplacementOutputs(t *testing.T) {
	tests := map[string]struct {
		expr        string
		replacement string
		want        []string
		finite      bool
	}{
		"finite suffix capture": {
			expr:        `app_worker_(.+)_(temperature|requests_total)`,
			replacement: `app_${2}`,
			want:        []string{"app_requests_total", "app_temperature"},
			finite:      true,
		},
		"nested finite capture": {
			expr:        `app_worker_(.+(temperature|requests_total))`,
			replacement: `app_${2}`,
			finite:      false,
		},
		"dynamic capture": {
			expr:        `app_worker_(.+)_old`,
			replacement: `${1}`,
			finite:      false,
		},
		"constant": {
			expr:        `app_temperature_(.+)`,
			replacement: `app_temperature`,
			want:        []string{"app_temperature"},
			finite:      true,
		},
		"capture absent on one branch": {
			expr:        `app_(foo|(bar))_(.+)`,
			replacement: `app_${2}`,
			want:        []string{"app_", "app_bar"},
			finite:      true,
		},
		"ambiguous named capture": {
			expr:        `app_(?P<kind>temperature)|app_(?P<kind>requests_total)`,
			replacement: `app_${kind}`,
			finite:      false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, finite := finiteRegexpReplacementOutputs(tc.expr, tc.replacement, 256)
			if finite != tc.finite || !slices.Equal(got, tc.want) {
				t.Fatalf("outputs=%q finite=%v, want outputs=%q finite=%v", got, finite, tc.want, tc.finite)
			}
		})
	}
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

func TestSyntheticFutureMetricSupportsCompleteSimplePatternGlobSyntax(t *testing.T) {
	canaries, wildcard := syntheticFutureMetrics(`!app_a* app_[bc]?*`)
	if !wildcard || len(canaries) != len(futureMetricStems) {
		t.Fatalf("expected varied future canaries, got canaries=%v wildcard=%v", canaries, wildcard)
	}
	scope, err := matcher.NewSimplePatternsMatcher(`!app_a* app_[bc]?*`)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range canaries {
		if !prometheusMetricNamePattern.MatchString(canary) {
			t.Fatalf("canary %q is not a valid Prometheus metric name", canary)
		}
		if !scope.MatchString(canary) {
			t.Fatalf("canary %q does not match its source scope", canary)
		}
	}

	if canaries, wildcard := syntheticFutureMetrics(`app_\*`); len(canaries) != 0 || wildcard {
		t.Fatalf("escaped glob metacharacter must not create a canary: canaries=%v wildcard=%v", canaries, wildcard)
	}

	canaries, wildcard = syntheticFutureMetrics(`app_* service_*`)
	if !wildcard || len(canaries) != 2*len(futureMetricStems) {
		t.Fatalf("every positive wildcard namespace needs a canary: canaries=%v wildcard=%v", canaries, wildcard)
	}
}

func TestValidateProfileRejectsWildcardScopeWithoutValidFutureCanary(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: '[0-9]*'", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "future_metric_canary_unavailable")
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
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "by_labels: [instance]", "by_labels: [node]")
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value{instance=\"a\"} 1\n", "")
	requireFinding(t, result, "dead_chart")
	if len(result.report.DeadCharts) != 1 {
		t.Fatalf("missing explicit identity label should make the chart unroutable: %#v", result.report.DeadCharts)
	}
}

func TestValidateProfileFindsObservedSameTemplateInstanceIDCollision(t *testing.T) {
	dump := "# TYPE app_value gauge\napp_value{instance=\"a.b\"} 1\napp_value{instance=\"a_b\"} 2\n"
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, "")
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
	dump := "# TYPE app_value gauge\napp_value{state=\"a\"} 1\napp_value{state=\"'a\"} 2\n"
	result := runValidation(t, singleDynamicValueGaugeProfile, dump, "")
	requireFinding(t, result, "dimension_id_collision_observed")
	if len(result.report.DimensionCollisions) != 1 {
		t.Fatalf("expected one emitted dimension ID collision: %#v", result.report.DimensionCollisions)
	}
	if strings.Contains(result.stdout, "\"'a\"") {
		t.Fatalf("report leaked label-derived dimension value:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsDimensionLostAtWireEmission(t *testing.T) {
	result := runValidation(t, singleDynamicValueGaugeProfile, "# TYPE app_value gauge\napp_value{state=\"'\"} 1\n", "")
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
	result := runValidation(t, profile, twoValueGaugesDump, "")
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
	result := runValidation(t, profile, twoValueGaugesDump, "")
	requireFinding(t, result, "context_wire_collision_observed")
	if len(result.report.ContextCollisions) != 1 ||
		len(result.report.ContextCollisions[0].RawContextFingerprints) != 2 {
		t.Fatalf("expected two distinct raw contexts to collapse: %#v", result.report.ContextCollisions)
	}
}

func TestInspectEmittedPlanAssociatesContextsInEmitterOrder(t *testing.T) {
	plan := chartengine.Plan{Actions: []chartengine.EngineAction{
		chartengine.CreateChartAction{ChartID: "z", Meta: chartengine.ChartMeta{Context: "'shared"}},
		chartengine.CreateChartAction{ChartID: "a", Meta: chartengine.ChartMeta{Context: "shared"}},
		chartengine.CreateChartAction{ChartID: "m", Meta: chartengine.ChartMeta{Context: "distinct"}},
	}}

	result, err := inspectEmittedPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.contextCollisions) != 1 {
		t.Fatalf("expected one context collision: %#v", result.contextCollisions)
	}
	want := []string{fingerprintID("'shared"), fingerprintID("shared")}
	slices.Sort(want)
	if !slices.Equal(want, result.contextCollisions[0].RawContextFingerprints) {
		t.Fatalf("wrong raw contexts associated with emitted collision: got %v, want %v",
			result.contextCollisions[0].RawContextFingerprints, want)
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
	result := runValidation(t, profile, twoValueGaugesDump, "")
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

func TestInspectEmittedPlanHandlesLargeChartLine(t *testing.T) {
	action := chartengine.CreateChartAction{ChartID: "value"}
	action.Meta.Title = strings.Repeat("x", 70*1024)
	result, err := inspectEmittedPlan(chartengine.Plan{Actions: []chartengine.EngineAction{action}})
	if err != nil {
		t.Fatal(err)
	}
	if result.emittedCharts != 1 {
		t.Fatalf("expected one emitted chart: %#v", result)
	}
}

func TestValidateProfileDoesNotLeakChartIDFromEmitterError(t *testing.T) {
	const sentinel = "sensitive-instance-value-"
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "  context_namespace: app\n", "")
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
	result := runValidation(t, profile, twoValueGaugesDump, "")
	requireFinding(t, result, "rendered_id_collision")
	if len(result.report.Collisions) != 1 {
		t.Fatalf("collisions: got %#v", result.report.Collisions)
	}
	if result.report.Collisions[0].RenderedIDFingerprint == "" {
		t.Fatalf("collision ID fingerprint is empty: %#v", result.report.Collisions[0])
	}
}

func TestValidateProfileRequiresExplicitPositivePriority(t *testing.T) {
	profile := strings.Replace(validProfile, "      priority: 110\n", "", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "priority_missing")
}

func TestValidateProfileKeepsLastValidPriorityAfterMissingPriority(t *testing.T) {
	profile := replaceOnce(t, validProfile, "      priority: 110\n", "")
	profile = replaceOnce(t, profile, "      priority: 120\n", "      priority: 90\n")
	result := runValidation(t, profile, validDump, "")

	requireFinding(t, result, "priority_missing")
	requireFinding(t, result, "priority_source_order")
	for _, finding := range result.report.Findings {
		if finding.Code == "priority_source_order" && !strings.Contains(finding.Message, "does not follow 100") {
			t.Fatalf("descending priority did not stay anchored to the last valid priority: %#v", finding)
		}
	}
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

func TestValidateProfileDoesNotTreatGaugeBucketSuffixAsHistogram(t *testing.T) {
	profile := `
match: app_capacity_bucket
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_capacity_bucket]
  charts:
    - title: Capacity
      context: capacity
      units: items
      algorithm: absolute
      type: line
      priority: 100
      dimensions:
        - selector: app_capacity_bucket
          name: capacity
`
	dump := `
# TYPE app_capacity_bucket gauge
app_capacity_bucket 7
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("a gauge name ending in _bucket is not a histogram bucket\nreport:\n%s", result.stdout)
	}
	for _, item := range result.report.Findings {
		switch item.Code {
		case "histogram_type_runtime_override", "histogram_bucket_units", "histogram_bucket_algorithm":
			t.Fatalf("unexpected histogram presentation finding %q: %#v", item.Code, result.report.Findings)
		}
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
	result := runValidation(t, profile, currentCapacityGaugesDump, "")
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
	result := runValidation(t, profile, currentCapacityGaugesDump, "")
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
	result := runValidation(t, profile, currentCapacityGaugesDump, "")
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

func TestValidateProfileWarnsWhenIncrementalUnitsOmitRateSemantics(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"      units: requests/s\n      algorithm: incremental\n",
		"      units: requests\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("incremental unit review must preserve author judgment\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "incremental_units_review", "warning") {
		t.Fatalf("missing incremental unit review: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsCompoundIncrementalRateUnits(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"      units: requests/s\n      algorithm: incremental\n",
		"      units: seconds/item/s\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("compound rate units should pass\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "incremental_units_review", "warning") {
		t.Fatalf("compound observed units retain a per-second denominator: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsDerivedIncrementalRateUnits(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"      units: requests/s\n      algorithm: incremental\n",
		"      units: in-flight\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("derived rate-equivalent units should pass\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "incremental_units_review", "warning") {
		t.Fatalf("a duration-total rate can truthfully express concurrent work: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsAboutObservedLabelsWithoutAnAuthoredRole(t *testing.T) {
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "selector: app_value", `selector: 'app_value{mode="sync"}'`)
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
	result := runValidation(t, profile, twoValueGaugesDump, "")
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
	dump := "# TYPE app_value gauge\napp_value{instance=\"finite\"} 1\napp_value{instance=\"nan\"} NaN\n"
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, "")
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

func TestValidateProfileReportsPartialRelabeledWriterMaterialization(t *testing.T) {
	dump := "# TYPE app_raw_value gauge\napp_raw_value{instance=\"finite\"} 1\napp_raw_value{instance=\"nan\"} NaN\n"
	job := `
relabeling:
  - match: app_raw_value
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_raw_value
        target_label: __name__
        replacement: app_value
        action: replace
`
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("one rejected renamed source series should remain transparent\nreport:\n%s", result.stdout)
	}
	if len(result.report.PipelineExcluded) != 1 {
		t.Fatalf("missing partial renamed-family exclusion: %#v", result.report.PipelineExcluded)
	}
	excluded := result.report.PipelineExcluded[0]
	if excluded.Name != "app_raw_value" ||
		excluded.Category != "partially_not_materialized_after_job_policy_or_writer" ||
		excluded.RawLogicalSeries != 2 ||
		excluded.WriterSourceSeries != 1 {
		t.Fatalf("unexpected partial renamed-family exclusion: %#v", excluded)
	}
	if len(result.report.PipelineRenamed) != 1 {
		t.Fatalf("missing partial rename lineage: %#v", result.report.PipelineRenamed)
	}
	renamed := result.report.PipelineRenamed[0]
	if renamed.RawName != "app_raw_value" ||
		!slices.Equal(renamed.FinalNames, []string{"app_value"}) ||
		renamed.RawLogicalSeries != 2 ||
		renamed.MaterializedLogicalSeries != 1 {
		t.Fatalf("unexpected partial rename lineage: %#v", renamed)
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

func TestWriteTextReportIncludesAuthoredMapping(t *testing.T) {
	r := newReport()
	r.AuthoredMapping = []authoredChartMappingReport{{
		Path:             "groups[0](Service).charts[0]",
		DisplayedFamily:  "Service/Requests",
		Title:            "Requests",
		Context:          "requests",
		Units:            "requests/s",
		Priority:         100,
		Type:             "line",
		InstanceByLabels: []string{"instance"},
		Dimensions: []authoredDimensionMappingReport{{
			Selector: "service_requests_total",
			Name:     "requests",
		}},
	}}
	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Authored selector-to-display mapping (source order):",
		`family="Service/Requests"`,
		`selector="service_requests_total"`,
		"algorithm=\"<inferred>\"",
		"identity=[instance]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
	}
}

func TestWriteTextReportSeparatesPipelineLossFromRenames(t *testing.T) {
	r := newReport()
	r.Counts.PipelineExcluded = 1
	r.Counts.PipelineRenamed = 1
	r.PipelineExcluded = []pipelineExcludedReport{{
		Name:               "app_dropped",
		Type:               "gauge",
		Category:           "not_materialized_after_job_policy_or_writer",
		RawLogicalSeries:   1,
		WriterSourceSeries: 0,
	}}
	r.PipelineRenamed = []pipelineRenamedReport{{
		RawName:                   "app_worker_alpha_temperature",
		FinalNames:                []string{"app_temperature"},
		RawLogicalSeries:          1,
		MaterializedLogicalSeries: 1,
	}}

	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Pipeline: excluded_families=1, renamed_families=1",
		"app_dropped: not_materialized_after_job_policy_or_writer",
		"app_worker_alpha_temperature -> app_temperature",
		"normalized_and_materialized=1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
	}
}

func TestWriteTextReportIncludesDetailedDiagnostics(t *testing.T) {
	r := newReport()
	r.DeadCharts = []deadChartReport{{
		Path:     "template.charts[0]",
		Title:    "Dead chart",
		Context:  "dead",
		Priority: 100,
	}}
	r.DeadDimensions = []deadDimensionReport{{
		Path:     "template.charts[1].dimensions[0]",
		Selector: "app_missing",
		Name:     "missing",
	}}
	r.DimensionLosses = []dimensionMaterializationLossReport{{
		Path:               "template.charts[2]",
		ObservedDimensions: 3,
		PlannedDimensions:  2,
		Cause:              "dimension lifecycle cap",
	}}
	r.Collisions = []collisionReport{{
		RenderedIDFingerprint: "sha256:rendered",
		Charts:                []string{"template.charts[3]", "template.charts[4]"},
	}}
	r.InstanceLosses = []instanceMaterializationLossReport{{
		Path:               "template.charts[5]",
		ObservedIdentities: 2,
		RenderedIDs:        1,
		Cause:              "rendered IDs collapsed",
	}}
	r.ChartWireCollisions = []wireChartCollisionReport{{
		WireIDFingerprint: "sha256:wire-chart",
		Occurrences:       2,
	}}
	r.ContextCollisions = []wireContextCollisionReport{{
		WireContextFingerprint: "sha256:wire-context",
		RawContextFingerprints: []string{"sha256:raw-a", "sha256:raw-b"},
	}}
	r.DimensionCollisions = []dimensionCollisionReport{{
		ChartIDFingerprint:     "sha256:chart",
		DimensionIDFingerprint: "sha256:dimension",
		Occurrences:            2,
	}}

	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Dead authored charts:",
		`template.charts[0] title="Dead chart"`,
		"Dead authored dimensions:",
		`selector="app_missing" name="missing"`,
		"Dimension materialization losses:",
		`observed=3 planned=2 cause="dimension lifecycle cap"`,
		"Rendered chart ID collisions:",
		"id=sha256:rendered charts=template.charts[3],template.charts[4]",
		"Chart instance materialization losses:",
		`observed=2 rendered=1 cause="rendered IDs collapsed"`,
		"Public wire chart ID collisions:",
		"id=sha256:wire-chart occurrences=2",
		"Public wire context collisions:",
		"context=sha256:wire-context raw_contexts=sha256:raw-a,sha256:raw-b",
		"Public wire dimension ID collisions:",
		"chart=sha256:chart dimension=sha256:dimension occurrences=2",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
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

func replaceOnce(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if count := strings.Count(source, old); count != 1 {
		t.Fatalf("fixture replacement target occurs %d times, want exactly 1: %q", count, old)
	}
	return strings.Replace(source, old, replacement, 1)
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

	jobPath := ""
	if job != "" {
		jobPath = filepath.Join(dir, "job.yaml")
		if err := os.WriteFile(jobPath, []byte(job), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return runValidationFiles(t, profilePath, dumpPath, jobPath)
}

func runValidationFiles(t *testing.T, profilePath, dumpPath, jobPath string) validationResult {
	t.Helper()

	args := []string{
		"-test.run=^TestValidatorHelperProcess$",
		"--",
		"--profile", profilePath,
		"--dump", dumpPath,
		"--output", "json",
	}
	if jobPath != "" {
		args = append(args, "--job", jobPath)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(
		withoutEnvironmentKeys(
			os.Environ(),
			"NETDATA_CYGWIN_BASE_PATH",
			"NETDATA_USER_CONFIG_DIR",
			"NETDATA_STOCK_CONFIG_DIR",
		),
		"NETDATA_PROFILE_VALIDATOR_HELPER=1",
		"NETDATA_CYGWIN_BASE_PATH=/hostile/ambient/cygwin",
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

func hasFinding(r report, code, severity string) bool {
	for _, item := range r.Findings {
		if item.Code == code && item.Severity == severity {
			return true
		}
	}
	return false
}
