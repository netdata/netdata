// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestProductionCoverageAcceptsCompleteMinimalCase(t *testing.T) {
	semanticCase, snapshot, reconciled := validProductionPlanCase(t)
	coverage, err := NewProductionCoverage(semanticCase.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := coverage.ObserveCase(context.Background(), semanticCase, snapshot, reconciled); err != nil {
		t.Fatalf("ObserveCase() error = %v", err)
	}
	if err := coverage.Verify(context.Background()); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestProductionCoverageRejectsMissingDeclarations(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	coverage, err := NewProductionCoverage(program)
	if err != nil {
		t.Fatal(err)
	}
	err = coverage.Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "registration inline/requests/canonical") ||
		!strings.Contains(err.Error(), "view requests input requests") {
		t.Fatalf("Verify() error = %v, want missing registration and view-input coverage", err)
	}
}

func TestProductionCoverageRequiresEachRawGrammarBranch(t *testing.T) {
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
	coverage, err := NewProductionCoverage(program)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"registration generated/operation_latency raw branch canonical",
		"registration generated/operation_latency raw branch embedded",
		"registration generated/operation_write_latency raw branch canonical",
		"registration generated/operation_write_latency raw branch embedded",
	} {
		if _, ok := coverage.required[key]; !ok {
			t.Fatalf("coverage does not require %q", key)
		}
	}
}

func TestProductionCoverageOmitsUnreachableConditionalLabelAbsence(t *testing.T) {
	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  transport_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:9]
    claim: Ray metadata exists only on the Ray transport.
  transport_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: Ray identifies the emitting component.
`, 1)
	source = strings.Replace(source, "  axes: {}\n  policies: {}", `  axes:
    transport:
      kind: enum
      values: [native, ray]
      meaning: Exporter metric transport.
      evidence: [transport_availability]
  policies:
    ray_transport:
      when: {any: [{all: [{axis: transport, op: eq, value: ray}]}]}
      evidence: [transport_availability]`, 1)
	source = strings.Replace(source, "  requests:\n    source:", "  requests:\n    availability: ray_transport\n    source:", 1)
	source = strings.Replace(source,
		"            prometheus: {type: counter, shape: scalar}\n            evidence: [request_registration]",
		"            prometheus: {type: counter, shape: scalar}\n            when: ray_transport\n            evidence: [request_registration]", 1)
	source = strings.Replace(source, "    labels: {}", `    labels:
      Component:
        meaning: Ray component metadata.
        presence: {when: ray_transport}
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [transport_label]`, 1)
	design := strings.Replace(validProfileDesignV1, "promote: []", "promote: [Component]", 1)
	program := compileTestSemanticContract(t, design, source)
	coverage, err := NewProductionCoverage(program)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "label requests/total/Component"
	if _, ok := coverage.required[prefix+" present"]; !ok {
		t.Fatalf("coverage does not require reachable %q branch", prefix+" present")
	}
	if _, ok := coverage.required[prefix+" absent"]; ok {
		t.Fatalf("coverage requires unreachable %q branch", prefix+" absent")
	}
}

func TestProductionCoveragePresentLabelRequiresNoAbsentWitnessAndObservesBlankValue(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "optional: []", "optional: [result]", 1)
	program := compileTestSemanticContract(t, design, sourceSemanticsWithResultLabel("present"))
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := NewProductionCoverage(program)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "label requests/total/result"
	if _, ok := coverage.required[prefix+" present"]; !ok {
		t.Fatalf("coverage does not require %q", prefix+" present")
	}
	if _, ok := coverage.required[prefix+" absent"]; ok {
		t.Fatalf("coverage incorrectly requires %q", prefix+" absent")
	}

	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].Labels = semanticTestLabels("result", "")
	snapshot.Sources[0].FinalLabels = semanticTestLabels("result", "")
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := coverage.ObserveCase(context.Background(), semanticCase, snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{prefix + " present", prefix + " value "} {
		if _, ok := coverage.seen[key]; !ok {
			t.Fatalf("coverage did not observe %q", key)
		}
	}
	if _, ok := coverage.seen[prefix+" absent"]; ok {
		t.Fatalf("coverage incorrectly observed %q", prefix+" absent")
	}
}

func TestProductionCoverageRejectsUnexercisedProductionChartPolicy(t *testing.T) {
	semanticCase, snapshot, reconciled := validProductionPlanCase(t)
	snapshot.Profiles[0].Charts = append(snapshot.Profiles[0].Charts,
		promreplay.SemanticChartPolicy{
			RuntimePath: "template.charts[1]", TemplateID: "dead-template",
			Dimensions: []promreplay.SemanticDimensionPolicy{{Index: 0}},
		})
	coverage, err := NewProductionCoverage(semanticCase.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := coverage.ObserveCase(context.Background(), semanticCase, snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	err = coverage.Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "production chart template.charts[1]") ||
		!strings.Contains(err.Error(), "production dimension template.charts[1] index 0") {
		t.Fatalf("Verify() error = %v, want dead production chart/dimension coverage", err)
	}
}

func TestProductionCoverageRejectsUnexercisedExactAutogenDeny(t *testing.T) {
	semanticCase, snapshot, reconciled := validProductionPlanCase(t)
	snapshot.Profiles[0].AutogenSelectorDeny = []string{"example_unused"}
	coverage, err := NewProductionCoverage(semanticCase.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := coverage.ObserveCase(context.Background(), semanticCase, snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	err = coverage.Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "autogen deny example_unused") {
		t.Fatalf("Verify() error = %v, want unexercised exact deny", err)
	}
}
