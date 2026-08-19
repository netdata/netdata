// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCompileSemanticContractCompilesViewLabelsIdentityAndUnits(t *testing.T) {
	source := sourceWithStatusLabelV1()
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    labels:
      dimensions:
        status: {render: label_value}
      promote: []
      omit: {}`, 1)

	program := compileTestSemanticContract(t, design, source)
	view := program.views["requests"]
	if view == nil || view.unit != "requests/s" || view.scale != newRationalScale(1, 1) || view.presentation != "line" {
		t.Fatalf("compiled view = %#v", view)
	}
	input := view.inputs["requests"]
	if input == nil || input.renderedRole != "requests" || len(input.occurrences) != 1 ||
		input.occurrences[0].algorithm != "incremental" {
		t.Fatalf("compiled input = %#v", input)
	}
}

func TestCompileSemanticContractMergesClosedLabelDomainsAcrossInputs(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("successful", "", "[operation_latency]")+"\n"+
			generatedSignalV1("failed", "", "[operation_write_latency]"),
		false,
	)
	source = strings.Replace(source, "evidence:\n", `evidence:
  outcome_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: Each signal exposes one closed request outcome.
`, 1)
	for _, outcome := range []string{"successful", "failed"} {
		labels := `    labels:
      outcome:
        meaning: Request outcome.
        presence: required
        domain: {kind: closed, values: [` + outcome + `]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [outcome_label]
    functional_dependencies: {}`
		source = strings.Replace(source, "    labels: {}\n    functional_dependencies: {}", labels, 1)
	}
	design := strings.Replace(validProfileDesignV1, `    inputs:
      requests:
        signal: requests
        components: [total]
    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    inputs:
      successful:
        signal: successful
        components: [total]
      failed:
        signal: failed
        components: [total]
    labels:
      dimensions:
        outcome: {render: label_value}
      promote: []
      omit: {}`, 1)
	contract := loadTestSemanticContract(
		t,
		design,
		source,
		validSourceRegistryV1,
		validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	labels, err := compileViewLabelUniverse("requests", program.views["requests"].inputs)
	if err != nil {
		t.Fatal(err)
	}
	if got := labels["outcome"].Domain.Values; !slices.Equal(got, []string{"failed", "successful"}) {
		t.Fatalf("merged outcome domain = %v", got)
	}
}

func TestLabelPresenceAnalysisKeepsPresentDistinct(t *testing.T) {
	base := SourceLabel{
		Meaning:             "Source classification.",
		Domain:              LabelDomain{Kind: "closed", Values: []string{"", "success"}},
		EndpointCardinality: EndpointCardinality{Kind: "closed_domain"},
		Stability:           "stable",
	}
	required := base
	required.Presence = LabelPresence{Kind: "required"}
	present := base
	present.Presence = LabelPresence{Kind: "present"}
	optional := base
	optional.Presence = LabelPresence{Kind: "optional"}

	merged, err := mergeLabelSchemas("result", required, present)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.Presence.Kind; got != "present" {
		t.Fatalf("required + present presence = %q, want present", got)
	}
	merged, err = mergeLabelSchemas("result", present, optional)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.Presence.Kind; got != "optional" {
		t.Fatalf("present + optional presence = %q, want optional", got)
	}

	blank := ""
	if labelPredicateMayMatch(LabelPredicate{Label: "result", Op: "absent"}, map[string]SourceLabel{"result": present}) {
		t.Fatal("present label incorrectly permits an absent predicate")
	}
	if !labelPredicateMayMatch(LabelPredicate{Label: "result", Op: "eq", Value: &blank}, map[string]SourceLabel{"result": present}) {
		t.Fatal("present label does not permit its declared empty value")
	}
	if labelPredicateMayMatch(LabelPredicate{Label: "result", Op: "eq", Value: &blank}, map[string]SourceLabel{"result": required}) {
		t.Fatal("required label incorrectly permits an empty value")
	}
	runtime := runtimeSourceLabelMap(map[string]SourceLabel{"result": present})["result"]
	if runtime.Presence.Kind != "optional" || !slices.Equal(runtime.Domain.Values, []string{"success"}) {
		t.Fatalf("runtime result schema = %#v, want optional nonblank values", runtime)
	}
}

func TestCompileSemanticContractRejectsIncompleteViewLabelClosure(t *testing.T) {
	contract := loadTestSemanticContract(t, validProfileDesignV1, sourceWithStatusLabelV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `unclassified label "status"`) {
		t.Fatalf("CompileSemanticContract() error = %v, want label-closure failure", err)
	}
}

func TestCompileSemanticContractAllowsStableOptionalPromotedLabel(t *testing.T) {
	design := strings.Replace(validProfileDesignV1,
		"promote: []",
		"promote: [region]", 1)
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

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractValidatesEntityIdentityRisk(t *testing.T) {
	source := sourceWithStatusLabelV1()
	design := strings.Replace(validProfileDesignV1, "required: []", "required: [status]", 1)
	program := compileTestSemanticContract(t, design, source)
	if program.views["requests"] == nil {
		t.Fatal("compiled identity view is missing")
	}

	highSource := strings.Replace(source, "kind: closed_domain", "kind: operational_population", 1)
	highSource = strings.Replace(highSource,
		"domain: {kind: closed, values: [\"200\", \"429\", invalid]}",
		"domain: {kind: open}", 1)
	highContract := loadTestSemanticContract(t, design, highSource, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: highContract})
	if err == nil || !strings.Contains(err.Error(), "requires high_cardinality_acceptance") {
		t.Fatalf("CompileSemanticContract() error = %v, want high-cardinality acceptance failure", err)
	}

	accepted := strings.Replace(design, `    identity:
      required: [status]
      optional: []`, `    identity:
      required: [status]
      optional: []
    high_cardinality_acceptance:
      operator_value: Preserves the smallest useful per-status diagnosis.`, 1)
	compileTestSemanticContract(t, accepted, highSource)
}

func TestCompileSemanticContractRejectsViewPredicateOutsideClosedDomain(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "components: [total]", `components: [total]
        where:
          any:
            - all: [{label: status, op: eq, value: "500"}]`, 1)
	design = strings.Replace(design, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    labels:
      dimensions: {}
      promote: []
      omit:
        status: The view selects the relevant status.`, 1)
	contract := loadTestSemanticContract(t, design, sourceWithStatusLabelV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `value "500" is outside label "status" closed domain`) {
		t.Fatalf("CompileSemanticContract() error = %v, want closed-domain predicate failure", err)
	}
}

func TestCompileSemanticContractRejectsRedundantAlgorithmOverride(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "components: [total]", `components: [total]
        algorithm:
          value: incremental
          reason: Preserve the source lifecycle.
          evidence: [request_lifecycle]`, 1)
	contract := loadTestSemanticContract(t, design, validSourceSemanticsV1, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "algorithm override is redundant") {
		t.Fatalf("CompileSemanticContract() error = %v, want redundant-override failure", err)
	}
}

func TestCompileSemanticContractRetainsCandidateOwnedSupportAvailability(t *testing.T) {
	supportDesign := strings.Replace(validProfileDesignV1, "profile: example", "profile: runtime", 1)
	supportDesign = strings.Replace(supportDesign, "match: example_*", "match: runtime_*", 1)
	supportDesign = strings.Replace(supportDesign, "namespace: example", "namespace: runtime", 1)
	supportSource := strings.Replace(validSourceSemanticsV1, "profile: example", "profile: runtime", 1)
	support := compileTestSemanticContract(t, supportDesign, supportSource)

	design := strings.Replace(validProfileDesignV1, "supports: {}", `supports:
    runtime:
      when: {any: [{all: [{axis: mode, op: eq, value: enabled}]}]}`, 1)
	design = strings.Replace(design, "signal: requests", "signal: runtime/requests", 1)
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  mode_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:24]
    claim: Runtime support is conditionally available.
environment:
`, 1)
	source = strings.Replace(source, "  axes: {}", `  axes:
    mode:
      kind: enum
      values: [disabled, enabled]
      meaning: Runtime support mode.
      evidence: [mode_availability]`, 1)
	contract := loadTestSemanticContract(t, design, source, "", "")
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{
		Contract: contract,
		Supports: map[string]*CompiledSemanticContract{"runtime": support},
	})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	occurrence := program.views["requests"].inputs["requests"].occurrences[0]
	if len(occurrence.destinationAvailability.clauses) != 1 ||
		len(occurrence.destinationAvailability.clauses[0]) != 1 ||
		occurrence.destinationAvailability.clauses[0][0].Axis != "mode" {
		t.Fatalf("support destination availability = %#v", occurrence.destinationAvailability)
	}
}
