// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionSourcesMatchesExactTypedOccurrence(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()

	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	if len(reconciled.Sources) != 1 {
		t.Fatalf("sources = %#v, want one", reconciled.Sources)
	}
	got := reconciled.Sources[0]
	if got.SourceIndex != 0 || got.Profile != "example" || got.Signal != "requests" ||
		got.Component != "total" || got.Registration != "inline/requests/canonical" {
		t.Fatalf("source match = %#v", got)
	}
}

func TestReconcileProductionSourcesRejectsHeaderAndRawLabelDrift(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		mutate func(*promreplay.SemanticSnapshot)
		want   string
	}{
		"automatic selection": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.SelectedProfiles = nil },
			want:   "selected profiles",
		},
		"stock job shaping": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.Job.HasSelector = true },
			want:   "stock proof job contains profile-owned policy",
		},
		"stock job app": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.Job.HasApp = true },
			want:   "app=true",
		},
		"source type": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.Sources[0].PrometheusType = "gauge" },
			want:   "no active semantic registration",
		},
		"undeclared label": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.Sources[0].Labels = []promreplay.SemanticLabel{{Name: "extra", Value: "value"}}
			},
			want: "not declared",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			tc.mutate(snapshot)
			_, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReconcileProductionSources() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReconcileProductionSourcesDistinguishesPresentFromRequired(t *testing.T) {
	tests := map[string]struct {
		presence string
		labels   []promreplay.SemanticLabel
		wantErr  string
	}{
		"present accepts blank": {
			presence: "present",
			labels:   semanticTestLabels("result", ""),
		},
		"present rejects absent": {
			presence: "present",
			wantErr:  `present label "result" is missing`,
		},
		"required rejects blank": {
			presence: "required",
			labels:   semanticTestLabels("result", ""),
			wantErr:  `required label "result" is missing or blank`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			design := strings.Replace(validProfileDesignV1, "optional: []", "optional: [result]", 1)
			program := compileTestSemanticContract(t, design, sourceSemanticsWithResultLabel(tc.presence))
			semanticCase, err := program.EvaluateCaseEnvironment(
				context.Background(), map[string]map[string]AxisValue{"example": {}},
			)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := validProductionSourceSnapshot()
			snapshot.Sources[0].Labels = tc.labels
			_, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ReconcileProductionSources() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ReconcileProductionSources() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestReconcileProductionSourcesUsesLongestKnownSuffix(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}

	for _, family := range []string{"example_write_latency", "example_worker_write_latency"} {
		t.Run(family, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			snapshot.Sources[0].MetricName = family
			snapshot.Sources[0].PrometheusType = "gauge"
			reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("ReconcileProductionSources() error = %v", err)
			}
			if got := reconciled.Sources[0].Registration; got != "generated/operation_write_latency" {
				t.Fatalf("registration = %q, want generated/operation_write_latency", got)
			}
		})
	}
}

func TestReconcileProductionSourcesUsesOnlyDeclaredRawGrammarBranches(t *testing.T) {
	registry := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}",
		"raw_branches: {embedded: {}}", 2)
	registry = strings.ReplaceAll(registry,
		"canonical: {prefix: example_", "canonical: {prefix: canonical_")
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, registry, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("embedded source", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "example_worker_write_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		if _, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot); err != nil {
			t.Fatalf("ReconcileProductionSources(embedded) error = %v", err)
		}
	})

	t.Run("canonical target is not raw source", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "canonical_write_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		if _, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot); err == nil ||
			!strings.Contains(err.Error(), "no active semantic registration") {
			t.Fatalf("ReconcileProductionSources(canonical target) error = %v, want no registration", err)
		}
	})
}

func TestReconcileProductionSourcesAppliesRawBranchEnvironment(t *testing.T) {
	conditioned := `raw_branches:
          canonical:
            when: {any: [{all: [{axis: mode, op: eq, value: single}]}]}
          embedded:
            when: {any: [{all: [{axis: mode, op: eq, value: multi}]}]}`
	registry := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}", conditioned, 2)
	registry = strings.ReplaceAll(registry,
		"canonical: {prefix: example_", "canonical: {prefix: canonical_")
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		true,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, registry, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		mode     string
		accepted string
		rejected string
	}{
		"canonical producer": {
			mode: "single", accepted: "canonical_write_latency", rejected: "example_worker_write_latency",
		},
		"embedded producer": {
			mode: "multi", accepted: "example_worker_write_latency", rejected: "canonical_write_latency",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			semanticCase, err := program.EvaluateCaseEnvironment(
				context.Background(),
				map[string]map[string]AxisValue{"example": {"mode": {String: &tc.mode}}},
			)
			if err != nil {
				t.Fatal(err)
			}
			for family, accepted := range map[string]bool{tc.accepted: true, tc.rejected: false} {
				snapshot := validProductionSourceSnapshot()
				snapshot.Sources[0].MetricName = family
				snapshot.Sources[0].PrometheusType = "gauge"
				_, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
				if accepted && err != nil {
					t.Fatalf("ReconcileProductionSources(%s) error = %v", family, err)
				}
				if !accepted && (err == nil || !strings.Contains(err.Error(), "no active semantic registration")) {
					t.Fatalf("ReconcileProductionSources(%s) error = %v, want no registration", family, err)
				}
			}
		})
	}
}

func TestReconcileProductionSourcesPrefersCanonicalFamilyOverEmbeddedGrammar(t *testing.T) {
	registry := `
version: v1
profile: example
generated: true
family_grammars:
  operation_family:
    forms:
      write_latency:
        canonical: {prefix: example_requests, suffix: _created}
        embedded:
          prefix: example_
          suffix: created
          separator: _
          identity_slot: {name: instrument, nonempty: true}
groups:
  core:
    registrations:
      operation_write_latency:
        family: {grammar: operation_family, form: write_latency}
        raw_branches: {canonical: {}, embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 10}
`
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_write_latency]"),
		false,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, registry, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].MetricName = "example_requests_created"
	snapshot.Sources[0].PrometheusType = "gauge"
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	if got := reconciled.Sources[0].Registration; got != "generated/operation_write_latency" {
		t.Fatalf("registration = %q, want generated/operation_write_latency", got)
	}
}

func TestReconcileProductionSourcesReservesExcludedNestedEmbeddedNamespace(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[broad_latency, special_count]"),
		false,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, nestedSourceRegistryV1, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		family       string
		registration string
		wantErr      string
	}{
		"broad namespace": {
			family:       "example_worker_latency",
			registration: "generated/broad_latency",
		},
		"nested namespace": {
			family:       "example_special_worker_count",
			registration: "generated/special_count",
		},
		"unknown nested suffix cannot fall back": {
			family:  "example_special_worker_latency",
			wantErr: "no active semantic registration",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			snapshot.Sources[0].MetricName = tc.family
			snapshot.Sources[0].PrometheusType = "gauge"
			reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ReconcileProductionSources() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReconcileProductionSources() error = %v", err)
			}
			if got := reconciled.Sources[0].Registration; got != tc.registration {
				t.Fatalf("registration = %q, want %q", got, tc.registration)
			}
		})
	}
}

func TestReplaySourceFamilyUsesProductionComponentShape(t *testing.T) {
	tests := map[string]struct {
		metric    string
		component string
		want      string
	}{
		"scalar":           {metric: "requests_total", component: "scalar", want: "requests_total"},
		"histogram bucket": {metric: "request_seconds_bucket", component: "histogram_bucket", want: "request_seconds"},
		"histogram sum":    {metric: "request_seconds_sum", component: "histogram_sum", want: "request_seconds"},
		"summary count":    {metric: "request_seconds_count", component: "summary_count", want: "request_seconds"},
		"summary quantile": {metric: "request_seconds", component: "summary_quantile", want: "request_seconds"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := replaySourceFamily(tc.metric, tc.component)
			if !ok || got != tc.want {
				t.Fatalf("replaySourceFamily() = %q, %t; want %q, true", got, ok, tc.want)
			}
		})
	}
}

func validProductionSourceSnapshot() *promreplay.SemanticSnapshot {
	return &promreplay.SemanticSnapshot{
		ContextRoot:      "example",
		SelectedProfiles: []string{"example"},
		Profiles: []promreplay.SemanticProfile{{
			Name: "example", Match: "example_*", ContextNamespace: "example",
		}},
		Sources: []promreplay.SemanticSource{{
			OccurrenceID:    "occurrence",
			MetricName:      "example_requests_total",
			FinalMetricName: "example_requests_total",
			Component:       "scalar",
			PrometheusType:  "counter",
		}},
	}
}

const nestedSourceRegistryV1 = `
version: v1
profile: example
generated: true
family_grammars:
  broad_family:
    forms:
      latency:
        canonical: {prefix: example_, suffix: latency}
        embedded:
          prefix: example_
          excluded_prefixes: [example_special_]
          suffix: latency
          separator: _
          identity_slot: {name: worker, nonempty: true}
  special_family:
    forms:
      count:
        canonical: {prefix: example_special_, suffix: count}
        embedded:
          prefix: example_special_
          suffix: count
          separator: _
          identity_slot: {name: worker, nonempty: true}
groups:
  core:
    registrations:
      broad_latency:
        family: {grammar: broad_family, form: latency}
        raw_branches: {embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 10}
      special_count:
        family: {grammar: special_family, form: count}
        raw_branches: {embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 11}
`
