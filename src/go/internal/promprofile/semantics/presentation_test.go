// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"
)

func TestCompileSemanticContractAuthorizesStackedClosedLabelPartition(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    presentation:
      type: stacked
      relationship: requests_by_status
      reason: Response statuses are an additive request partition.
    labels:
      dimensions:
        status: {render: label_value}
      promote: []
      omit: {}`, 1)
	program := compileTestSemanticContract(t, design, sourceWithStatusPartitionV1())
	if got := program.views["requests"].presentation; got != "stacked" {
		t.Fatalf("presentation = %q, want stacked", got)
	}

	nondisjoint := strings.Replace(sourceWithStatusPartitionV1(), "disjoint: true", "disjoint: false", 1)
	contract := loadTestSemanticContract(t, design, nondisjoint, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "must be a disjoint exhaustive partition") {
		t.Fatalf("CompileSemanticContract() error = %v, want partition failure", err)
	}
}

func TestCompileSemanticContractAuthorizesStackedInputPartition(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, `    inputs:
      requests:
        signal: requests
        components: [total]
    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    inputs:
      successful:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: "200"}]}]}
      throttled:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: "429"}]}]}
      invalid:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: invalid}]}]}
    labels:
      dimensions:
        status: {render: input_role}
      promote: []
      omit: {}
    presentation:
      type: stacked
      relationship: requests_by_status
      reason: Response statuses are an additive request partition.`, 1)
	program := compileTestSemanticContract(t, design, sourceWithStatusPartitionV1())
	if got := program.views["requests"].presentation; got != "stacked" {
		t.Fatalf("presentation = %q, want stacked", got)
	}
}

func TestCompileSemanticContractAcceptsDeliberateAreaWithoutUnitHeuristics(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "    labels:\n", `    presentation:
      type: area
      reason: Filled magnitude is the deliberate operator presentation.
    labels:
`, 1)
	program := compileTestSemanticContract(t, design, validSourceSemanticsV1)
	if got := program.views["requests"].presentation; got != "area" {
		t.Fatalf("presentation = %q, want area", got)
	}
}
