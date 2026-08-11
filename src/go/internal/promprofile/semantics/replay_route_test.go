// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionRoutesMatchesOccurrenceQualifiedSemanticEdge(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)

	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
	if len(reconciled.Edges) != 1 {
		t.Fatalf("edges = %#v, want one", reconciled.Edges)
	}
	got := reconciled.Edges[0]
	if got.SourceIndex != 0 || got.SourceProfile != "example" || got.Signal != "requests" ||
		got.Component != "total" || got.OccurrenceID != "occurrence" ||
		got.DestinationProfile != "example" || got.Context != "requests" ||
		got.Input != "requests" || got.RenderedRole != "requests" ||
		got.ChartID != "requests" || got.DimensionName != "requests" {
		t.Fatalf("edge = %#v", got)
	}
}

func TestReconcileProductionRoutesRejectsSemanticDriftAndMultiplicity(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		mutate func(*promreplay.SemanticSnapshot)
		want   string
	}{
		"missing edge": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.Sources[0].Routes = nil },
			want:   "no exact production route",
		},
		"duplicate edge": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.Sources[0].Routes = append(snapshot.Sources[0].Routes, snapshot.Sources[0].Routes[0])
			},
			want: "unexpected production route",
		},
		"context": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.Sources[0].Routes[0].Context = "prometheus.example.responses"
			},
			want: "context",
		},
		"algorithm": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.Sources[0].Routes[0].Algorithm = "absolute"
			},
			want: "algorithm",
		},
		"contributor count": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.Sources[0].Routes[0].ContributorCount = 2
			},
			want: "contributor count",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionRouteSnapshot()
			tc.mutate(snapshot)
			reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
			err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReconcileProductionRoutes() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReconcileProductionRoutesDerivesLabelDimensionWithAutomaticPromotion(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    labels:
      dimensions:
        status: {render: label_value}
      promote: []
      omit: {}`, 1)
	program := compileTestSemanticContract(t, design, sourceWithStatusLabelV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.Sources[0].Labels = semanticTestLabels("status", "429")
	snapshot.Sources[0].FinalLabels = semanticTestLabels("status", "429")
	route := &snapshot.Sources[0].Routes[0]
	route.DimensionName = "429"
	route.DimensionKeyLabel = "status"

	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
}

func TestReconcileProductionRoutesPreservesOrderedIdentityAndLabelValues(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "required: []", "required: [status]", 1)
	program := compileTestSemanticContract(t, design, sourceWithStatusLabelV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.Sources[0].Labels = semanticTestLabels("status", "200")
	snapshot.Sources[0].FinalLabels = semanticTestLabels("status", "200")
	route := &snapshot.Sources[0].Routes[0]
	route.IdentityLabels = []string{"status"}
	route.ChartLabels = []string{"status"}
	route.ChartLabelValues = semanticTestLabels("status", "200")

	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
}

func TestReconcileProductionRoutesOmitsAbsentOptionalPromotedLabel(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "promote: []", "promote: [region]", 1)
	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  request_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: Region is stable endpoint metadata when the exporter supplies it.
`, 1)
	source = strings.Replace(source, "    labels: {}", `    labels:
      region:
        meaning: Deployment region.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [request_label]`, 1)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	route := &snapshot.Sources[0].Routes[0]
	route.PromotionMode = "allowlist"
	route.PromotedLabels = []string{"region"}

	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
}

func TestReconcileProductionRoutesAcceptsExplicitIdentityOnlyWithoutNonidentityLabels(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.Sources[0].Routes[0].PromotionMode = "identity_only"

	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
}

func TestReconcileProductionRoutesCountsReducedOccurrenceMultiset(t *testing.T) {
	program := compileTestSemanticContract(t, workerAverageReductionDesignV1(), sourceWithWorkerContributorsV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.Sources[0].OccurrenceID = "worker-a"
	snapshot.Sources[0].Labels = semanticTestLabels("pid", "100")
	snapshot.Sources[0].FinalLabels = semanticTestLabels("pid", "100")
	snapshot.Sources[0].PrometheusType = "gauge"
	route := &snapshot.Sources[0].Routes[0]
	route.PromotionMode = "identity_only"
	route.Algorithm = "absolute"
	route.SeriesKind = "gauge"
	route.Aggregation = "avg"
	route.Units = "requests"
	route.ContributorCount = 2
	second := snapshot.Sources[0]
	second.OccurrenceID = "worker-b"
	second.Labels = semanticTestLabels("pid", "200")
	second.FinalLabels = semanticTestLabels("pid", "200")
	second.Routes = slices.Clone(second.Routes)
	snapshot.Sources = append(snapshot.Sources, second)

	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}
	if len(reconciled.Edges) != 2 {
		t.Fatalf("edges = %#v, want two occurrence-qualified contributors", reconciled.Edges)
	}
}

func TestReconcileProductionRoutesRequiresExactDesignExclusionOutcome(t *testing.T) {
	tests := map[string]struct {
		outcome string
		mutate  func(*promreplay.SemanticSource)
	}{
		"drop before writer": {
			outcome: "drop_before_writer",
			mutate: func(source *promreplay.SemanticSource) {
				rule := semanticTestRelabel(
					"relabeling[0].metric_relabel_configs[0]", "drop",
					source.MetricName, nil, source.MetricName, nil,
				)
				rule.Dropped = true
				source.RelabelRules = []promreplay.SemanticRelabelOccurrence{rule}
				source.Terminal = &promreplay.SemanticTerminal{
					Disposition: "profile_excluded", Profile: "example", RuntimePath: rule.RuntimePath,
				}
			},
		},
		"retain writable unrendered": {
			outcome: "retain_writable_unrendered",
			mutate: func(source *promreplay.SemanticSource) {
				source.WriterSeries = 1
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			design := strings.Replace(
				designWithProcessStartExclusionV1(), "outcome: drop_before_writer", "outcome: "+tc.outcome, 1,
			)
			program := compileTestSemanticContract(t, design, sourceWithProcessStartTimestampV1())
			semanticCase, err := program.EvaluateCaseEnvironment(
				context.Background(), map[string]map[string]AxisValue{"example": {}},
			)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := validProductionSourceSnapshot()
			source := &snapshot.Sources[0]
			source.MetricName = "process_start_time_seconds"
			source.FinalMetricName = source.MetricName
			source.PrometheusType = "gauge"
			tc.mutate(source)
			if tc.outcome == "retain_writable_unrendered" {
				snapshot.Profiles[0].AutogenSelectorDeny = []string{"process_start_time_seconds"}
				source.AutogenSuppressions = []promreplay.SemanticAutogenSuppression{{
					Profile: "example", Family: "process_start_time_seconds",
				}}
			}

			reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
			if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
				t.Fatalf("ReconcileProductionRoutes() error = %v", err)
			}
			if len(reconciled.Exclusions) != 1 || reconciled.Exclusions[0].Outcome != tc.outcome {
				t.Fatalf("exclusions = %#v, want %q", reconciled.Exclusions, tc.outcome)
			}
		})
	}
}

func TestReconcileProductionRoutesRequiresUnitMetadataCarrierValue(t *testing.T) {
	design, sourceContract := metadataOnlyScalarContract()
	program := compileTestSemanticContract(t, design, sourceContract)
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}

	buildSnapshot := func(value float64) *promreplay.SemanticSnapshot {
		snapshot := validProductionSourceSnapshot()
		snapshot.Profiles[0].AutogenSelectorDeny = []string{"example_runtime_metadata"}
		source := &snapshot.Sources[0]
		source.MetricName = "example_runtime_metadata"
		source.FinalMetricName = source.MetricName
		source.PrometheusType = "gauge"
		source.Value = value
		source.Labels = semanticTestLabels("version", "1.2.3")
		source.FinalLabels = semanticTestLabels("version", "1.2.3")
		source.WriterSeries = 1
		source.AutogenSuppressions = []promreplay.SemanticAutogenSuppression{{
			Profile: "example", Family: "example_runtime_metadata",
		}}
		return snapshot
	}

	snapshot := buildSnapshot(1)
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}

	snapshot = buildSnapshot(2)
	reconciled = reconcileTestProductionCase(t, semanticCase, snapshot)
	err = semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), "metadata carrier value is 2, want 1") {
		t.Fatalf("ReconcileProductionRoutes() error = %v, want metadata-value failure", err)
	}
}

func TestReconcileProductionRoutesBindsRetainedExclusionToExactAutogenDeny(t *testing.T) {
	design := strings.Replace(
		designWithProcessStartExclusionV1(),
		"outcome: drop_before_writer",
		"outcome: retain_writable_unrendered",
		1,
	)
	program := compileTestSemanticContract(t, design, sourceWithProcessStartTimestampV1())
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		deny         []string
		suppressions []promreplay.SemanticAutogenSuppression
		want         string
	}{
		"exact": {
			deny: []string{"process_start_time_seconds"},
			suppressions: []promreplay.SemanticAutogenSuppression{{
				Profile: "example", Family: "process_start_time_seconds",
			}},
		},
		"missing deny": {
			suppressions: []promreplay.SemanticAutogenSuppression{{
				Profile: "example", Family: "process_start_time_seconds",
			}},
			want: "has no exact autogen deny",
		},
		"missing suppression": {
			deny: []string{"process_start_time_seconds"},
			want: "is not suppressed",
		},
		"wrong owner": {
			deny: []string{"process_start_time_seconds"},
			suppressions: []promreplay.SemanticAutogenSuppression{{
				Profile: "support", Family: "process_start_time_seconds",
			}},
			want: "suppressed by profile",
		},
		"wrong family": {
			deny: []string{"process_start_time_seconds"},
			suppressions: []promreplay.SemanticAutogenSuppression{{
				Profile: "example", Family: "process_other_seconds",
			}},
			want: "suppresses family",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			snapshot.Profiles[0].AutogenSelectorDeny = slices.Clone(tc.deny)
			source := &snapshot.Sources[0]
			source.MetricName = "process_start_time_seconds"
			source.FinalMetricName = source.MetricName
			source.PrometheusType = "gauge"
			source.WriterSeries = 1
			source.AutogenSuppressions = slices.Clone(tc.suppressions)

			reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
			err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ReconcileProductionRoutes() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReconcileProductionRoutes() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReconcileProductionRoutesRejectsUncoveredWriterOccurrence(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, sourceWithProcessStartTimestampV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	source := &snapshot.Sources[0]
	source.MetricName = "process_start_time_seconds"
	source.FinalMetricName = source.MetricName
	source.PrometheusType = "gauge"
	source.WriterSeries = 1
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)

	err = semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), "no expected authored destination or intentional terminal exclusion") {
		t.Fatalf("ReconcileProductionRoutes() error = %v, want uncovered occurrence failure", err)
	}
}

func TestReconcileProductionRoutesAcceptsSourceDeclaredInfoFamilyRejection(t *testing.T) {
	source := strings.Replace(validSourceSemanticsV1, "relationships: {}", `  runtime_info:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: example_runtime_info}
            prometheus: {type: gauge, shape: info}
            evidence: [request_registration]
    population:
      id: runtime_metadata
      meaning: Runtime metadata for this exporter process.
      evidence: [request_population]
    components:
      value:
        wire_role: scalar
        lifecycle: {kind: constant, evidence: [request_lifecycle]}
        unit:
          quantity: count
          base: one
          rate: none
          object: runtime_metadata
          aspect: present
          evidence: [request_unit]
    labels: {}
    functional_dependencies: {}
relationships: {}`, 1)
	program := compileTestSemanticContract(t, validProfileDesignV1, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{{
		OccurrenceID: "info", MetricName: "example_runtime_info", FinalMetricName: "example_runtime_info",
		Component: "scalar", PrometheusType: "gauge",
		Terminal: &promreplay.SemanticTerminal{
			Disposition: "writer_ineligible", WriterReason: "info_family",
		},
	}}
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionRoutes() error = %v", err)
	}

	snapshot.Sources[0].Terminal = nil
	reconciled = reconcileTestProductionCase(t, semanticCase, snapshot)
	err = semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), "info family") {
		t.Fatalf("ReconcileProductionRoutes() error = %v, want missing writer rejection", err)
	}
}

func validProductionRouteSnapshot() *promreplay.SemanticSnapshot {
	snapshot := validProductionSourceSnapshot()
	snapshot.ContextRoot = "prometheus.example"
	snapshot.Profiles[0].Charts = []promreplay.SemanticChartPolicy{{
		RuntimePath: "template.charts[0]",
		TemplateID:  "template-0",
		Dimensions:  []promreplay.SemanticDimensionPolicy{{Index: 0}},
	}}
	snapshot.Sources[0].WriterSeries = 1
	snapshot.Sources[0].Routes = []promreplay.SemanticRoute{{
		Profile:          "example",
		TemplatePath:     "template.charts[0]",
		MetricName:       "example_requests_total",
		ChartID:          "requests",
		Context:          "prometheus.example.requests",
		DisplayedFamily:  "Traffic/Requests",
		DimensionName:    "requests",
		PromotionMode:    "automatic",
		Algorithm:        "incremental",
		SeriesKind:       "counter",
		Aggregation:      "sum",
		Units:            "requests/s",
		Multiplier:       1,
		Divisor:          1,
		Presentation:     "line",
		ContributorCount: 1,
	}}
	return snapshot
}

func reconcileTestProductionCase(
	t *testing.T,
	semanticCase *CompiledSemanticCase,
	snapshot *promreplay.SemanticSnapshot,
) *ReconciledSemanticCase {
	t.Helper()
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	if err := semanticCase.ReconcileProductionNormalizations(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionNormalizations() error = %v", err)
	}
	return reconciled
}
