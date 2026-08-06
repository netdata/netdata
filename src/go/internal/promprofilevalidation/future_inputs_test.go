// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	commonmodel "github.com/prometheus/common/model"
)

func TestValidateFutureInputsContract(t *testing.T) {
	valid := []futureInput{{
		Name:   "métrique.total",
		Type:   "counter",
		Labels: map[string]string{"élément": "one"},
	}}
	if err := validateFutureInputs(valid); err != nil {
		t.Fatalf("valid UTF-8 future input: %v", err)
	}

	tests := map[string]struct {
		inputs []futureInput
		want   string
	}{
		"invalid type": {
			inputs: []futureInput{{Name: "app_future", Type: "histogram"}},
			want:   "use gauge, counter, or untyped",
		},
		"metric name label": {
			inputs: []futureInput{{Name: "app_future", Labels: map[string]string{"__name__": "other"}}},
			want:   "invalid label name",
		},
		"duplicate identity": {
			inputs: []futureInput{{Name: "app_future"}, {Name: "app_future"}},
			want:   "duplicates an earlier raw metric identity",
		},
		"mixed family types": {
			inputs: []futureInput{
				{Name: "app_future", Type: "gauge", Labels: map[string]string{"instance": "a"}},
				{Name: "app_future", Type: "counter", Labels: map[string]string{"instance": "b"}},
			},
			want: "more than one type",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateFutureInputs(tc.inputs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want substring %q", err, tc.want)
			}
		})
	}

	tooMany := make([]futureInput, maxFutureInputs+1)
	if err := validateFutureInputs(tooMany); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversized input error=%v", err)
	}
}

func TestValidationJobRejectsUnknownFutureInputField(t *testing.T) {
	result := runValidation(t, validProfile, validDump, `
future_inputs:
  - name: app_future
    typo: rejected
`)
	requireFinding(t, result, "job_policy")
	if !strings.Contains(result.report.Findings[0].Message, "field typo not found") {
		t.Fatalf("unknown nested field was not reported precisely: %#v", result.report.Findings)
	}
}

func TestFutureInputEncodingRoundTripsThroughProductionParser(t *testing.T) {
	inputs := []futureInput{
		{Name: "app_counter", Type: "counter", Labels: map[string]string{"instance": "a"}},
		{Name: "app_gauge", Labels: map[string]string{"élément": "β"}},
		{Name: "app_untyped", Type: "untyped"},
	}
	combined, err := appendFutureInputs([]byte("# TYPE current gauge\ncurrent 1\n# EOF\n"), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(combined), "# EOF") {
		t.Fatalf("OpenMetrics EOF remained before appended families:\n%s", combined)
	}

	path := filepath.Join(t.TempDir(), "metrics.prom")
	if err := os.WriteFile(path, combined, 0o600); err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	batch, err := scrapeRawSamples(context.Background(), fileURL)
	if err != nil {
		t.Fatal(err)
	}
	if duplicates := prompkg.FindSampleDuplicates(batch); len(duplicates) != 0 {
		t.Fatalf("encoded future inputs contain duplicates: %#v", duplicates)
	}
	types := make(map[string]commonmodel.MetricType)
	values := make(map[float64]struct{})
	for _, sample := range batch.Samples {
		types[sample.Name] = sample.FamilyType
		if sample.Name != "current" {
			values[sample.Value] = struct{}{}
		}
	}
	if types["app_counter"] != commonmodel.MetricTypeCounter ||
		types["app_gauge"] != commonmodel.MetricTypeGauge ||
		types["app_untyped"] != commonmodel.MetricTypeUnknown {
		t.Fatalf("unexpected parsed future types: %#v", types)
	}
	if len(values) != len(inputs) {
		t.Fatalf("future probe values are not globally unique: %#v", values)
	}
}

func TestPositiveWildcardScopesPreserveOrderedSimplePatternBranches(t *testing.T) {
	scopes := positiveWildcardScopes("match", `!app_a* app_[bc]?*`, pipelineRelabelLocation{block: -1})
	if len(scopes) != 1 {
		t.Fatalf("expected one positive wildcard scope, got %#v", scopes)
	}
	if got, want := scopes[0].scopeExpr, `!app_a* app_[bc]?*`; got != want {
		t.Fatalf("scope expression: got %q, want %q", got, want)
	}
	if got := positiveWildcardScopes("match", `app_\*`, pipelineRelabelLocation{block: -1}); len(got) != 0 {
		t.Fatalf("escaped glob metacharacter created a wildcard scope: %#v", got)
	}
	if got := positiveWildcardScopes("match", `app_* service_*`, pipelineRelabelLocation{block: -1}); len(got) != 2 {
		t.Fatalf("every positive wildcard term needs a scope: %#v", got)
	}
}

func TestFutureScopeWitnessUsesSharedMatcherAndUTF8MetricGrammar(t *testing.T) {
	analyzer, err := matcher.NewAnalyzer(context.Background(), matcher.AnalysisBudget{})
	if err != nil {
		t.Fatal(err)
	}
	excluded := map[string]struct{}{}
	for range futureWitnessesPerScope {
		witness, ok, err := futureScopeWitness(analyzer, `[0-9]*`, excluded)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("expected another numeric UTF-8 metric witness after exclusions %v", excluded)
		}
		if !commonmodel.UTF8Validation.IsValidMetricName(witness) {
			t.Fatalf("witness %q is not a UTF-8-valid Prometheus metric name", witness)
		}
		excluded[witness] = struct{}{}
	}
}

func TestNamespaceChangingJobRequiresExplicitRawFutureInputs(t *testing.T) {
	dump := validDump + `
# TYPE app_worker_current_temperature gauge
app_worker_current_temperature{instance="node-b"} 43
`
	job := `
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
`
	result := runValidation(t, validProfile, dump, job)
	requireFinding(t, result, "future_inputs_required")
}

func TestNamespaceChangingProfileRequiresExplicitRawFutureInputs(t *testing.T) {
	profile := replaceOnce(t, validProfile, "app: app\n", `app: app
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
`)
	dump := validDump + "# TYPE app_worker_current_temperature gauge\napp_worker_current_temperature{instance=\"node-b\"} 43\n"

	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "future_inputs_required")
}

func TestProfileRelabelingFutureProbeTraversesTwoStageDiagnostics(t *testing.T) {
	profile := replaceOnce(t, validProfile, "app: app\n", `app: app
relabeling:
  - match: app_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
      - source_labels: [__name__]
        regex: app_worker_(.+)_(temperature)
        target_label: __name__
        replacement: app_${2}
`)
	dump := validDump + "# TYPE app_worker_current_temperature gauge\napp_worker_current_temperature{instance=\"node-b\"} 43\n"
	job := `
future_inputs:
  - name: app_worker_future_temperature
    labels:
      instance: node-c
relabeling:
  - match: app_*
    metric_relabel_configs:
      - source_labels: [instance]
        regex: (.+)
        target_label: source_instance
        replacement: ${1}
`

	result := runValidation(t, profile, dump, job)
	if result.exitCode != 0 {
		t.Fatalf("profile relabeling future proof failed\nreport:\n%s", result.stdout)
	}
	if result.report.Counts.PipelineRenamed != 1 {
		t.Fatalf("profile rename provenance was not reconciled: %#v", result.report.PipelineRenamed)
	}
}

func TestFutureInputsMustCoverEveryProfileWildcardTerm(t *testing.T) {
	profile := strings.Replace(validProfile, "match: app_*", "match: app_* service_*", 1)
	job := `
future_inputs:
  - name: legacy_future_metric
relabeling:
  - match: legacy_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: legacy_(.+)
        target_label: __name__
        replacement: app_${1}
`
	result := runValidation(t, profile, validDump, job)
	requireFinding(t, result, "future_profile_term_uncovered")
}

func TestFutureInputsMustCoverEveryReachableRelabelRule(t *testing.T) {
	job := `
future_inputs:
  - name: legacy_a_future
relabeling:
  - match: legacy_a_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: legacy_a_(.+)
        target_label: __name__
        replacement: app_${1}
  - match: legacy_b_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: legacy_b_(.+)
        target_label: __name__
        replacement: app_${1}
`
	result := runValidation(t, validProfile, validDump, job)
	requireFinding(t, result, "future_relabel_branch_uncovered")
}
