// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func TestValidateProfilePassesThroughRealPipeline(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS (exit 0), got %d\nstderr:\n%s\nreport:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	if result.report.Verdict != verdictPass {
		t.Fatalf("expected PASS report, got %#v", result.report.Findings)
	}
	if !strings.HasPrefix(result.report.Profiles.Candidate.FutureRawProbe, "app_") {
		t.Fatalf("future raw probe: got %q", result.report.Profiles.Candidate.FutureRawProbe)
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
	if result.report.Job.MaxTimeSeries != 2000 || result.report.Job.MaxSeriesPerMetric != 200 {
		t.Fatalf("collector defaults were not preserved: %#v", result.report.Job)
	}
	if len(result.report.AuthoredMapping) != 4 {
		t.Fatalf("authored mapping count: got %d, want 4", len(result.report.AuthoredMapping))
	}
	first := result.report.AuthoredMapping[0]
	if first.DisplayedFamily != "Example" ||
		first.Title != "Temperature" ||
		first.Type != "line" ||
		first.Priority != 0 ||
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
	if !hasFinding(result.report, "default_validation_job", "warning") {
		t.Fatalf("missing warning that the deployable job policy was not validated: %#v", result.report.Findings)
	}
	for _, chart := range result.report.Charts {
		if chart.IDFingerprint == "" {
			t.Fatalf("materialized chart ID fingerprint is empty: %#v", chart)
		}
	}
}

func TestStageIsolatedCatalogDoesNotMutateProcessDiscovery(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	if err := os.WriteFile(profilePath, []byte(validProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, []byte(validDump), 0o600); err != nil {
		t.Fatal(err)
	}

	wantExecutableName := executable.Name
	wantExecutableDirectory := executable.Directory
	wantUserConfigDir := buildinfo.UserConfigDir
	wantStockConfigDir := buildinfo.StockConfigDir

	staged, cleanup, err := stageValidationInputs(profilePath, nil, dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, ok := staged.catalog.Get("candidate"); !ok {
		t.Fatal("isolated catalog did not contain candidate profile")
	}

	if executable.Name != wantExecutableName ||
		executable.Directory != wantExecutableDirectory ||
		buildinfo.UserConfigDir != wantUserConfigDir ||
		buildinfo.StockConfigDir != wantStockConfigDir {
		t.Fatalf("isolated catalog staging mutated process discovery globals")
	}
}

func TestValidateProfileComposesExplicitSupportingProfiles(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	for path, content := range map[string]string{
		profilePath: `
match: app_*
app: app
template:
  family: App
  metrics: [app_value]
  charts:
    - title: App Value
      context: app_value
      units: values
      dimensions:
        - selector: app_value
          name: value
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
      dimensions:
        - selector: runtime_value
          name: value
`,
		dumpPath: "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_value gauge\nruntime_value 2\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	report := Validate(context.Background(), Options{
		ProfilePath:            profilePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
	})
	if !report.Passed() {
		t.Fatalf("explicit profile composition failed: %#v", report.Findings)
	}
	if report.Counts.AuthoredCharts != 2 || report.Counts.CuratedCharts != 2 {
		t.Fatalf("composed chart counts: got %#v, want two authored and curated charts", report.Counts)
	}
	if len(report.AuthoredMapping) != 2 {
		t.Fatalf("composed authored mapping count: got %d, want 2", len(report.AuthoredMapping))
	}
	if report.AuthoredMapping[0].DisplayedFamily != "App" || report.AuthoredMapping[1].DisplayedFamily != "Runtime" {
		t.Fatalf("supporting profile did not preserve exact composition order: %#v", report.AuthoredMapping)
	}
}

func TestValidateProfileAcceptsSupportingProfileFallbackSuppression(t *testing.T) {
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
autogen:
  selector:
    deny: [runtime_extra]
template:
  family: Runtime
  metrics: [runtime_value]
  charts:
    - title: Runtime Value
      context: runtime_value
      units: values
      dimensions: [{selector: runtime_value, name: value}]
`,
		dumpPath: "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_value gauge\nruntime_value 2\n# TYPE runtime_extra gauge\nruntime_extra 3\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result := runValidationFilesWithSupports(t, profilePath, []string{supportPath}, dumpPath, "")
	if result.exitCode != 0 {
		t.Fatalf("supporting-profile fallback suppression should pass\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "profile_suppressed_series", "warning") {
		t.Fatalf("missing profile_suppressed_series warning in %#v", result.report.Findings)
	}
}

func TestValidateProfileRejectsDuplicateSupportingProfileIdentity(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	supportDir := filepath.Join(dir, "support")
	if err := os.MkdirAll(supportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	supportPath := filepath.Join(supportDir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	for path, content := range map[string]string{
		profilePath: validProfile,
		supportPath: validProfile,
		dumpPath:    validDump,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	report := Validate(context.Background(), Options{
		ProfilePath:            profilePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
	})
	if report.Passed() || !hasFinding(report, "profile_load", "error") {
		t.Fatalf("duplicate profile identity was not rejected: %#v", report.Findings)
	}
}

func TestValidateProfileRejectsDuplicatePhysicalEvidence(t *testing.T) {
	duplicateDump := strings.Replace(
		validDump,
		`app_temperature{instance="node-a"} 42`,
		"app_temperature{instance=\"node-a\"} 42\napp_temperature{instance=\"node-a\"} 43",
		1,
	)
	result := runValidation(t, validProfile, duplicateDump, "")
	if result.exitCode == 0 {
		t.Fatalf("duplicate evidence unexpectedly passed\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "duplicate_source_sample", "error") {
		t.Fatalf("missing duplicate-source error: %#v", result.report.Findings)
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

func TestLoadJobPolicyDecodesSafeFieldsOntoCollectorDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.yaml")
	if err := os.WriteFile(path, []byte("name: custom\nselector:\n  allow: [app_*]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := loadJobPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Name != "custom" || !slices.Equal(policy.Selector.Allow, []string{"app_*"}) {
		t.Fatalf("safe fields were not decoded: %#v", policy)
	}
	if policy.MaxTS != 2000 || policy.MaxTSPerMetric != 200 {
		t.Fatalf("collector defaults were not retained: %#v", policy)
	}
	if policy.URL != "" {
		t.Fatalf("validation job policy unexpectedly gained an endpoint: %q", policy.URL)
	}
}

func TestLoadJobPolicyRejectsUnknownNestedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.yaml")
	if err := os.WriteFile(path, []byte("selector:\n  unknown: [app_*]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadJobPolicy(path)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict nested-key rejection, got %v", err)
	}
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

func TestValidateProfileRequiresCoverageForSummaryWithoutQuantiles(t *testing.T) {
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
	requireFinding(t, result, "unexpected_autogen")
	if len(result.report.PipelineExcluded) != 0 {
		t.Fatalf("writer-capable quantile-free summary was reported as pipeline-excluded: %#v", result.report.PipelineExcluded)
	}
	if result.report.Counts.SeriesAutogen != 2 {
		t.Fatalf("quantile-free summary autogen series: got %d, want 2", result.report.Counts.SeriesAutogen)
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

func TestValidateProfileDefersExactFallbackEvidenceForAggregateReplay(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"app: app\n",
		"app: app\nautogen:\n  selector:\n    deny: [app_absent]\n",
		1,
	)
	result := runValidationWithAggregateEvidence(t, profile, validDump, "", true)
	if result.exitCode != 0 {
		t.Fatalf("aggregate semantic replay defers per-case profile fallback presence\nreport:\n%s", result.stdout)
	}
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

func TestValidateProfileRejectsJobSelectorThatAdmitsOnlyLegacyFixedCanaries(t *testing.T) {
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
	requireFinding(t, result, "future_metric_blocked_by_job_selector")
}

func TestValidateProfileAcceptsJobSelectorOpenToFutureFamily(t *testing.T) {
	result := runValidation(t, validProfile, validDump, "selector:\n  allow: ['app_*']\n")
	if result.exitCode != 0 {
		t.Fatalf("namespace-wide job selector should admit future family\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
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
		result.report.PipelineExcluded[0].Category != "not_materialized_after_pipeline_policy_or_writer" {
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
		excluded.Category != "partially_not_materialized_after_pipeline_policy_or_writer" ||
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
