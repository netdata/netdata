// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"
)

func TestCompileSemanticContractRequiresExactDynamicCumulativeLimitation(t *testing.T) {
	design := dynamicContributorReductionDesignV1(true)
	program := compileTestSemanticContract(t, design, sourceWithDynamicContributorsV1())
	if program.limitations["requests#requests"] == nil {
		t.Fatal("compiled limitation is missing")
	}

	missing := dynamicContributorReductionDesignV1(false)
	contract := loadTestSemanticContract(t, missing, sourceWithDynamicContributorsV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "requires an exact limitation") {
		t.Fatalf("CompileSemanticContract() error = %v, want exact-limitation failure", err)
	}
}

func TestCompileSemanticContractRejectsUnnecessaryCumulativeLimitation(t *testing.T) {
	source := strings.Replace(sourceWithDynamicContributorsV1(), "membership: {stability: dynamic}",
		"membership: {stability: restart_stable}", 1)
	source = strings.Replace(source, "reset: {scope: per_contributor}", "reset: {scope: shared}", 1)
	contract := loadTestSemanticContract(t, dynamicContributorReductionDesignV1(true), source, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "is not dynamic") {
		t.Fatalf("CompileSemanticContract() error = %v, want unnecessary-limitation failure", err)
	}
}

func dynamicContributorReductionDesignV1(withLimitation bool) string {
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    reduction:
      reducer: sum
      lost_comparison: Per-contributor comparison is intentionally collapsed.
    labels:
      dimensions: {}
      promote: []
      omit:
        pid: Per-process detail is intentionally collapsed.`, 1)
	if !withLimitation {
		return design
	}
	return strings.Replace(design, "limitations: {}", `limitations:
  requests#requests:
    contributor_variant: workers
    evidence: [worker_lifecycle]
    proof_sequence: worker_expiry
    effect: aggregate_drop_may_create_one_reset_interpreted_rate_gap`, 1)
}

func sourceWithDynamicContributorsV1() string {
	source := sourceWithStatusLabelV1()
	source = strings.Replace(source, "status_label:", "pid_label:", 1)
	source = strings.Replace(source, "HTTP response status", "Exporter worker process identity", 2)
	source = strings.Replace(source, "status:", "pid:", 1)
	source = strings.Replace(source,
		`domain: {kind: closed, values: ["200", "429", invalid]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [status_label]`,
		`domain: {kind: open}
        endpoint_cardinality: {kind: operational_population}
        stability: dynamic
        evidence: [pid_label]`, 1)
	source = strings.Replace(source, "environment:\n", `  worker_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:18]
    claim: Worker processes can coexist.
  worker_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:19]
    claim: Worker processes can disappear independently.
  worker_values:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:20]
    claim: Worker counters form an additive population.
environment:
`, 1)
	return strings.Replace(source, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        workers:
          identity: [pid]
          cardinality: {kind: operational_population}
          concurrency: may_coexist
          value_model: {total: additive}
          membership: {stability: dynamic}
          reset: {scope: per_contributor}
          join: {new_contributor_baseline: zero}
          evidence:
            population: [worker_population]
            lifecycle: [worker_lifecycle]
            relationship: [worker_values]`, 1)
}
