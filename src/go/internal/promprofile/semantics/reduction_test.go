// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"
)

func TestCompileSemanticContractRequiresSourceAuthorizedReduction(t *testing.T) {
	source := sourceWithStatusPartitionV1()
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    labels:
      dimensions: {}
      promote: []
      omit:
        status: Per-status comparison is intentionally collapsed.`, 1)

	t.Run("missing reduction", func(t *testing.T) {
		contract := loadTestSemanticContract(t, design, source, "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "requires reduction") {
			t.Fatalf("CompileSemanticContract() error = %v, want required-reduction failure", err)
		}
	})

	withSum := strings.Replace(design, "    labels:\n", `    reduction:
      reducer: sum
      lost_comparison: Per-status request comparison is intentionally collapsed.
    labels:
`, 1)
	t.Run("partition authorizes sum", func(t *testing.T) {
		compileTestSemanticContract(t, withSum, source)
	})

	t.Run("partition does not authorize average", func(t *testing.T) {
		withAverage := strings.Replace(withSum, "reducer: sum", "reducer: avg", 1)
		contract := loadTestSemanticContract(t, withAverage, source, "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), `reducer "avg" is not source-authorized`) {
			t.Fatalf("CompileSemanticContract() error = %v, want reducer-authorization failure", err)
		}
	})
}

func TestCompileSemanticContractRejectsUnnecessaryReduction(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "    labels:\n", `    reduction:
      reducer: sum
      lost_comparison: No comparison is actually lost.
    labels:
`, 1)
	contract := loadTestSemanticContract(t, design, validSourceSemanticsV1, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "reduction is unnecessary") {
		t.Fatalf("CompileSemanticContract() error = %v, want unnecessary-reduction failure", err)
	}
}

func TestCompileSemanticContractTreatsDisjointInputPredicatesAsCoexistingContributors(t *testing.T) {
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
        render_as: responses
      throttled:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: "429"}]}]}
        render_as: responses
      invalid:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: invalid}]}]}
        render_as: responses
    reduction:
      reducer: sum
      lost_comparison: Per-status comparison is intentionally collapsed.
    labels:
      dimensions:
        status: {render: input_role}
      promote: []
      omit: {}`, 1)
	compileTestSemanticContract(t, design, sourceWithStatusPartitionV1())

	withoutReduction := strings.Replace(design, `    reduction:
      reducer: sum
      lost_comparison: Per-status comparison is intentionally collapsed.
`, "", 1)
	contract := loadTestSemanticContract(t, withoutReduction, sourceWithStatusPartitionV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "requires reduction") {
		t.Fatalf("CompileSemanticContract() error = %v, want coexisting-input reduction failure", err)
	}
}

func TestCompileSemanticContractUsesContributorValueModelForAverage(t *testing.T) {
	source := sourceWithWorkerContributorsV1()
	design := workerAverageReductionDesignV1()
	compileTestSemanticContract(t, design, source)

	wrong := strings.Replace(design, "reducer: avg", "reducer: sum", 1)
	contract := loadTestSemanticContract(t, wrong, source, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `reducer "sum" is not source-authorized`) {
		t.Fatalf("CompileSemanticContract() error = %v, want contributor-model failure", err)
	}
}

func TestCompileSemanticContractAuthorizesReducerForDerivedContributorCategory(t *testing.T) {
	source := strings.Replace(sourceWithStatusNormalizationV1(), "environment:\n", `  status_children:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:17]
    claim: Per-status request children are disjoint additive contributors.
environment:
`, 1)
	source = strings.Replace(source, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        status_children:
          identity: [status]
          cardinality: {kind: closed_domain}
          concurrency: may_coexist
          value_model: {total: additive}
          membership: {stability: stable}
          reset: {scope: shared}
          join: {new_contributor_baseline: zero}
          evidence:
            population: [request_population]
            lifecycle: [request_lifecycle]
            relationship: [status_children]`, 1)
	design := designWithNormalizationsV1(categoryNormalizationV1("status_class", "status", "status_class"))
	design = strings.Replace(design, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    reduction:
      reducer: sum
      lost_comparison: Per-status request comparison is intentionally collapsed.
    labels:
      dimensions: {}
      promote: []
      omit:
        status: Exact status comparison is intentionally collapsed.
        status_class: Status-class comparison is intentionally collapsed.`, 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAuthorizesReducerForRenamedContributorIdentity(t *testing.T) {
	source := strings.Replace(sourceWithStatusLabelV1(), "environment:\n", `  status_children:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:17]
    claim: Per-status request children are disjoint additive contributors.
environment:
`, 1)
	source = strings.Replace(source, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        status_children:
          identity: [status]
          cardinality: {kind: closed_domain}
          concurrency: may_coexist
          value_model: {total: additive}
          membership: {stability: stable}
          reset: {scope: shared}
          join: {new_contributor_baseline: zero}
          evidence:
            population: [request_population]
            lifecycle: [request_lifecycle]
            relationship: [status_children]`, 1)
	design := designWithNormalizationsV1(`  status_label:
    kind: label_rename
    source_label: status
    target_label: status_class
    retain_source: false`)
	design = strings.Replace(design, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    reduction:
      reducer: sum
      lost_comparison: Per-status request comparison is intentionally collapsed.
    labels:
      dimensions: {}
      promote: []
      omit:
        status_class: Status-class comparison is intentionally collapsed.`, 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAuthorizesReducerForEmbeddedContributorIdentity(t *testing.T) {
	source := sourceWithEmbeddedWorkerContributorsV1()
	design := embeddedWorkerReductionDesignV1()

	contract := loadTestSemanticContract(
		t,
		design,
		source,
		registryWithSingleOperationV1(),
		validSourceRegistryGeneratorV1,
	)
	if _, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract}); err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
}

func TestCompileSemanticContractRejectsUnresolvedContributorIdentity(t *testing.T) {
	tests := map[string]struct {
		design string
		source string
		want   string
	}{
		"missing normalizer": {
			design: validProfileDesignV1,
			source: sourceWithEmbeddedWorkerContributorsV1(),
			want:   `identity "worker" is absent from post-normalization occurrence`,
		},
		"misspelled identity": {
			design: embeddedWorkerReductionDesignV1(),
			source: strings.Replace(sourceWithEmbeddedWorkerContributorsV1(),
				"identity: [worker]", "identity: [worker_typo]", 1),
			want: `identity "worker_typo" is absent from post-normalization occurrence`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contract := loadTestSemanticContract(
				t,
				tc.design,
				tc.source,
				registryWithSingleOperationV1(),
				validSourceRegistryGeneratorV1,
			)
			_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CompileSemanticContract() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func embeddedWorkerReductionDesignV1() string {
	design := designWithNormalizationsV1(`  worker_identity:
    kind: embedded_identity_extract
    registry_grammar: operation_family
    target_label: worker
    retain: {family: canonical_branch, captured_identity: true}
    output:
      meaning: Exporter worker identity.
      endpoint_cardinality: {kind: operational_population}
      stability: restart_stable
      evidence: [worker_label]
    evidence: [worker_identity]`)
	return strings.Replace(design, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    reduction:
      reducer: sum
      lost_comparison: Per-worker comparison is intentionally collapsed.
    labels:
      dimensions: {}
      promote: []
      omit:
        worker: Per-worker comparison is intentionally collapsed.`, 1)
}

func registryWithSingleOperationV1() string {
	registry := strings.Replace(validSourceRegistryV1, "    interpretation: longest_known_suffix\n", "", 1)
	registry = strings.Replace(registry, `      write_latency:
        canonical: {prefix: example_, suffix: write_latency}
        embedded:
          prefix: example_
          suffix: write_latency
          separator: _
          identity_slot: {name: worker, nonempty: true}
`, "", 1)
	return strings.Replace(registry, `      operation_write_latency:
        family: {grammar: operation_family, form: write_latency}
        raw_branches: {canonical: {}, embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, range: {start: 11, end: 12}}
`, "", 1)
}

func TestCompileSemanticContractRequiresContributorVariantsToPartitionAvailability(t *testing.T) {
	t.Run("uncovered", func(t *testing.T) {
		contract := loadTestSemanticContract(t, workerAverageReductionDesignV1(),
			sourceWithConditionalWorkerVariantsV1(""), "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "do not cover every active environment") {
			t.Fatalf("CompileSemanticContract() error = %v, want uncovered-variant failure", err)
		}
	})

	t.Run("overlap", func(t *testing.T) {
		contract := loadTestSemanticContract(t, workerAverageReductionDesignV1(),
			sourceWithConditionalWorkerVariantsV1("single"), "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), `variants "alternate" and "workers" overlap`) {
			t.Fatalf("CompileSemanticContract() error = %v, want overlapping-variant failure", err)
		}
	})

	t.Run("disjoint exhaustive", func(t *testing.T) {
		compileTestSemanticContract(t, workerAverageReductionDesignV1(),
			sourceWithConditionalWorkerVariantsV1("multi"))
	})
}

func workerAverageReductionDesignV1() string {
	return strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    reduction:
      reducer: avg
      lost_comparison: Per-worker comparison is intentionally collapsed.
    labels:
      dimensions: {}
      promote: []
      omit:
        pid: Per-worker comparison is available through a worker view.`, 1)
}

func sourceWithStatusPartitionV1() string {
	source := sourceWithStatusLabelV1()
	source = strings.Replace(source, "environment:\n", `  status_partition:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:17]
    claim: Status values are disjoint and exhaustive request populations.
environment:
`, 1)
	return strings.Replace(source, "relationships: {}", `relationships:
  requests_by_status:
    kind: partition
    whole: {signal: requests, components: [total]}
    parts:
      - signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: "200"}]}]}
      - signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: "429"}]}]}
      - signal: requests
        components: [total]
        where: {any: [{all: [{label: status, op: eq, value: invalid}]}]}
    disjoint: true
    exhaustive: true
    evidence: [status_partition]`, 1)
}

func sourceWithWorkerContributorsV1() string {
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
        stability: restart_stable
        evidence: [pid_label]`, 1)
	source = strings.Replace(source, "prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: gauge, shape: scalar}", 1)
	source = strings.Replace(source, "kind: cumulative", "kind: current", 1)
	source = strings.Replace(source, "environment:\n", `  worker_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:18]
    claim: Workers coexist within one exporter endpoint.
  worker_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:19]
    claim: Worker membership remains stable until service restart.
  worker_values:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:20]
    claim: Worker point values are directly comparable.
environment:
`, 1)
	return strings.Replace(source, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        workers:
          identity: [pid]
          cardinality: {kind: operational_population}
          concurrency: may_coexist
          value_model: {total: comparable_point}
          membership: {stability: restart_stable}
          reset: {scope: per_contributor}
          join: {new_contributor_baseline: current_total}
          evidence:
            population: [worker_population]
            lifecycle: [worker_lifecycle]
            relationship: [worker_values]`, 1)
}

func sourceWithEmbeddedWorkerContributorsV1() string {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency]"),
		false,
	)
	source = strings.Replace(source, "environment:\n", `  worker_identity:
    kind: identity
    upstream: exporter
    locations: [metrics.go:14]
    claim: The generated family name embeds exporter worker identity.
  worker_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: Embedded worker identity is restart-stable and operationally bounded.
  worker_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:16]
    claim: Workers coexist within one exporter endpoint.
  worker_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:17]
    claim: Worker membership remains stable until service restart.
  worker_values:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:18]
    claim: Worker point values are additive.
environment:
`, 1)
	return strings.Replace(source, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        workers:
          identity: [worker]
          cardinality: {kind: operational_population}
          concurrency: may_coexist
          value_model: {total: additive}
          membership: {stability: restart_stable}
          reset: {scope: per_contributor}
          join: {new_contributor_baseline: current_total}
          evidence:
            population: [worker_population]
            lifecycle: [worker_lifecycle]
            relationship: [worker_values]`, 1)
}

func sourceWithConditionalWorkerVariantsV1(alternateMode string) string {
	source := sourceWithWorkerContributorsV1()
	source = strings.Replace(source, "environment:\n", `  worker_mode:
    kind: availability
    upstream: exporter
    locations: [metrics.go:21]
    claim: The exporter selects one worker representation.
environment:
`, 1)
	source = strings.Replace(source, "  axes: {}", `  axes:
    mode:
      kind: enum
      values: [single, multi]
      meaning: Active worker representation.
      evidence: [worker_mode]`, 1)
	source = strings.Replace(source, `        workers:
          identity: [pid]`, `        workers:
          when: {any: [{all: [{axis: mode, op: eq, value: single}]}]}
          identity: [pid]`, 1)
	if alternateMode == "" {
		return source
	}
	alternate := `        alternate:
          when: {any: [{all: [{axis: mode, op: eq, value: ` + alternateMode + `}]}]}
          identity: [pid]
          cardinality: {kind: operational_population}
          concurrency: may_coexist
          value_model: {total: comparable_point}
          membership: {stability: restart_stable}
          reset: {scope: per_contributor}
          join: {new_contributor_baseline: current_total}
          evidence:
            population: [worker_population]
            lifecycle: [worker_lifecycle]
            relationship: [worker_values]
`
	return strings.Replace(source, "relationships: {}", alternate+"relationships: {}", 1)
}
