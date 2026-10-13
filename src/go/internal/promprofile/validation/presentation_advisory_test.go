// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"strings"
	"testing"
)

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

func TestValidateProfileAcceptsOmittedRuntimeDerivedHistogramType(t *testing.T) {
	profile := strings.Replace(validProfile, "      type: heatmap\n", "", 1)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("omitted derived heatmap type must pass\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "histogram_type_runtime_override", "warning") {
		t.Fatalf("omitted derived heatmap type must not warn: %#v", result.report.Findings)
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

func TestValidateProfileReviewsExplicitFilledPresentationWithoutInferringIntentFromUnits(t *testing.T) {
	for _, chartType := range []string{"area", "stacked"} {
		t.Run(chartType, func(t *testing.T) {
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
      type: ` + chartType + `
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
			if result.exitCode != 0 {
				t.Fatalf("presentation intent cannot be rejected from units alone\nreport:\n%s", result.stdout)
			}
			if !hasFinding(result.report, chartType+"_presentation_review", "warning") {
				t.Fatalf("missing %s intent review: %#v", chartType, result.report.Findings)
			}
		})
	}
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
	if !hasFinding(result.report, "area_presentation_review", "warning") {
		t.Fatalf("missing physical-rate fill review prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileReviewsFilledPhysicalStorageUnit(t *testing.T) {
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
	if !hasFinding(result.report, "area_presentation_review", "warning") {
		t.Fatalf("explicit area still requires authored intent: %#v", result.report.Findings)
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

func TestValidateProfileLeavesIncrementalEquivalentUnitsToSemanticReview(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"      units: requests/s\n      algorithm: incremental\n",
		"      units: in-flight\n",
		1,
	)
	result := runValidation(t, profile, validDump, "")
	if result.exitCode != 0 {
		t.Fatalf("rate-equivalent units remain a semantic-review concern\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "incremental_units_review", "warning") {
		t.Fatalf("unit syntax cannot establish or reject integrated population semantics: %#v", result.report.Findings)
	}
}
