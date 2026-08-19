// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCompileSemanticContractBuildsNormalizedLabelCatalog(t *testing.T) {
	source := sourceWithStatusNormalizationV1()
	design := designWithNormalizationsV1(`  copy_status:
    kind: label_rename
    source_label: status
    target_label: copied_status
    retain_source: true
  status_class:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: copied_status
    target_label: status_class
    exact: {"429": throttled}
    ranges: [{min: 400, max: 499, value: client_error}]
    missing: {leave_absent: true}
    malformed: {set: malformed}
    unknown: {set: other}
    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
    evidence: [status_normalization]`)
	design = strings.Replace(design, "required: []", "required: [status]", 1)
	design = strings.Replace(design, "      promote: []", "      promote: [copied_status, status_class]", 1)

	program := compileTestSemanticContract(t, design, source)
	if len(program.normalizations) != 2 || program.normalizations[0].id != "copy_status" ||
		program.normalizations[1].id != "status_class" {
		t.Fatalf("normalization order = %#v", program.normalizations)
	}
	occurrence := onlyCompiledOccurrence(t, program)
	if _, ok := occurrence.sourceLabels["status"]; !ok || len(occurrence.sourceLabels) != 1 {
		t.Fatalf("raw source catalog = %#v, want only status", occurrence.sourceLabels)
	}
	for _, label := range []string{"status", "copied_status", "status_class"} {
		if _, ok := occurrence.labels[label]; !ok {
			t.Fatalf("normalized catalog lacks %q: %#v", label, occurrence.labels)
		}
	}
	class := occurrence.labels["status_class"]
	if class.Presence.Kind != "required" || class.Domain.Kind != "closed" ||
		!sameStrings(class.Domain.Values, []string{"client_error", "malformed", "other", "throttled"}) ||
		class.EndpointCardinality.Kind != "closed_domain" || class.Stability != "stable" {
		t.Fatalf("derived category schema = %#v", class)
	}
}

func TestCategoryNormalizationTreatsPresentBlankAsAValueNotMissing(t *testing.T) {
	other := "other"
	leaveAbsent := true
	definition := Normalization{
		Exact:     map[string]string{"": "unclassified", "success": "successful"},
		Missing:   &CategoryAction{LeaveAbsent: &leaveAbsent},
		Malformed: &CategoryAction{Set: &other},
		Unknown:   &CategoryAction{Set: &other},
	}
	source := SourceLabel{
		Presence: LabelPresence{Kind: "present"},
		Domain:   LabelDomain{Kind: "closed", Values: []string{"", "success"}},
	}
	if got := categoryCoverageBranches(definition, source); !sameStrings(got, []string{"exact:", "exact:success"}) {
		t.Fatalf("coverage branches = %v, want present values without missing", got)
	}
	if got := categoryOutputPresence(definition, source).Kind; got != "required" {
		t.Fatalf("category output presence = %q, want required", got)
	}
}

func TestCompileSemanticContractBuildsCategoryFromMetricName(t *testing.T) {
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  metric_name_normalization:
    kind: normalization
    upstream: exporter
    locations: [metrics.go:14]
    claim: The exact metric name encodes a closed request category.
  request_category_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: The derived category identifies the request metric.
environment:
`, 1)
	design := designWithNormalizationsV1(`  request_category:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: __name__
    target_label: request_category
    exact: {example_requests_total: requests}
    missing: {leave_absent: true}
    malformed: {leave_absent: true}
    unknown: {leave_absent: true}
    output:
      meaning: Request metric category.
      evidence: [request_category_label]
    evidence: [metric_name_normalization]`)
	design = strings.Replace(design, "      dimensions: {}", `      dimensions:
        request_category: {render: label_value}`, 1)

	program := compileTestSemanticContract(t, design, source)
	occurrence := onlyCompiledOccurrence(t, program)
	category, ok := occurrence.labels["request_category"]
	if !ok || category.Presence.Kind != "required" || category.Domain.Kind != "closed" ||
		!sameStrings(category.Domain.Values, []string{"requests"}) {
		t.Fatalf("metric-name category = %#v", category)
	}
	if _, leaked := occurrence.labels[semanticMetricNameField]; leaked {
		t.Fatalf("metric name leaked into chart label catalog: %#v", occurrence.labels)
	}
}

func TestCompileSemanticContractBuildsMetricNameCategoryForHistogramComponents(t *testing.T) {
	source := strings.Replace(validSourceSemanticsV1,
		"prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: histogram, shape: histogram}", 1)
	source = strings.Replace(source, validScalarComponentV1, `    components:
      bucket:
        wire_role: histogram_bucket
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: observations
          aspect: distributed
          evidence: [request_unit]
      count:
        wire_role: histogram_count
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed
          evidence: [request_unit]
      sum:
        wire_role: histogram_sum
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: duration
          base: second
          rate: none
          object: request_time
          aspect: accumulated
          evidence: [request_unit]`, 1)
	source = strings.Replace(source, "environment:\n", `  metric_name_normalization:
    kind: normalization
    upstream: exporter
    locations: [metrics.go:14]
    claim: Histogram component names encode one closed request category.
  request_category_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: The derived category identifies the request metric.
environment:
`, 1)
	design := designWithNormalizationsV1(`  request_category:
    kind: category
    applies_to: {signal: requests, components: [bucket, count, sum]}
    source_label: __name__
    target_label: request_category
    exact:
      example_requests_total_bucket: requests
      example_requests_total_count: requests
      example_requests_total_sum: requests
    missing: {leave_absent: true}
    malformed: {leave_absent: true}
    unknown: {leave_absent: true}
    output:
      meaning: Request metric category.
      evidence: [request_category_label]
    evidence: [metric_name_normalization]`)
	design = strings.Replace(design, "required: []", "required: [request_category]", 1)
	design = strings.Replace(design, "components: [total]", "components: [sum]", 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractExcludesTerminalGeneratedOccurrencesFromGlobalRenames(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency]")+"\n"+
			generatedSignalV1("registration_epochs", "", "[operation_write_latency]"),
		false,
	)
	source = strings.Replace(source, "evidence:\n", `evidence:
  generated_registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:14]
    claim: The generator registers a terminal structural family class.
  source_labels:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: Generated registrations expose the union of source labels.
`, 1)
	oldLabel := `    labels:
      old_model:
        meaning: Legacy model name.
        presence: required
        domain: {kind: closed, values: [one]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [source_labels]
    functional_dependencies: {}`
	generatedLabels := `    labels:
      old_model:
        meaning: Legacy model name.
        presence: optional
        domain: {kind: closed, values: [one]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [source_labels]
      model:
        meaning: Canonical model name.
        presence: optional
        domain: {kind: closed, values: [one]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [source_labels]
    functional_dependencies: {}`
	source = strings.Replace(source, "    labels: {}\n    functional_dependencies: {}", oldLabel, 1)
	source = strings.Replace(source, "    labels: {}\n    functional_dependencies: {}", generatedLabels, 1)

	design := designWithNormalizationsV1(`  generated_registration_epochs:
    kind: generated_component_exclusion
    source:
      namespace_prefix: example_
      terminal_suffix: _write_latency
      component: scalar
    outcome: drop_before_writer
    evidence: [generated_registration, request_lifecycle, request_unit]
  model_label:
    kind: label_rename
    source_label: old_model
    target_label: model
    retain_source: false`)
	design = strings.Replace(design, "      omit: {}", "      omit: {model: Not part of this test view.}", 1)
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
	for _, normalization := range program.normalizations {
		if normalization.id == "model_label" && len(normalization.occurrences) != 1 {
			t.Fatalf("global rename occurrences = %v, want only the nonterminal source", normalization.occurrences)
		}
	}
}

func TestCompileSemanticContractRejectsInvalidNormalizationGraph(t *testing.T) {
	source := sourceWithStatusNormalizationV1()

	t.Run("unresolved read", func(t *testing.T) {
		design := designWithNormalizationsV1(categoryNormalizationV1("classify", "missing", "class"))
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{
			Contract: loadTestSemanticContract(t, design, source, "", ""),
		})
		if err == nil || !strings.Contains(err.Error(), `reads unknown label "missing"`) {
			t.Fatalf("CompileSemanticContract() error = %v, want unresolved-read failure", err)
		}
	})

	t.Run("duplicate writer", func(t *testing.T) {
		design := designWithNormalizationsV1(
			categoryNormalizationV1("first", "status", "class") + "\n" +
				categoryNormalizationV1("second", "status", "class"),
		)
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{
			Contract: loadTestSemanticContract(t, design, source, "", ""),
		})
		if err == nil || !strings.Contains(err.Error(), `multiple writers`) {
			t.Fatalf("CompileSemanticContract() error = %v, want duplicate-writer failure", err)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		design := designWithNormalizationsV1(
			categoryNormalizationV1("first", "second_label", "first_label") + "\n" +
				categoryNormalizationV1("second", "first_label", "second_label"),
		)
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{
			Contract: loadTestSemanticContract(t, design, source, "", ""),
		})
		if err == nil || !strings.Contains(err.Error(), `dependency cycle`) {
			t.Fatalf("CompileSemanticContract() error = %v, want cycle failure", err)
		}
	})
}

func TestCompileSemanticContractDerivesEmbeddedIdentityOutputSchema(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency]")+"\n"+
			generatedSignalV1("writes", "", "[operation_write_latency]"),
		false,
	)
	source = strings.Replace(source, "evidence:\n", `evidence:
  worker_identity:
    kind: identity
    upstream: exporter
    locations: [metrics.go:14]
    claim: The worker identity is embedded in each generated family name.
  worker_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: The embedded worker identifies a restart-stable operational process.
`, 1)
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
	design = strings.Replace(design, `        components: [total]
    labels:`, `        components: [total]
      writes:
        signal: writes
        components: [total]
    labels:`, 1)
	design = strings.Replace(design, `    identity:
      required: []
      optional: []`, `    identity:
      required: []
      optional: [worker]
    high_cardinality_acceptance:
      operator_value: Preserves per-worker diagnosis.`, 1)

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
	if len(program.occurrences) != 2 {
		t.Fatalf("occurrences = %d, want 2", len(program.occurrences))
	}
	for key, occurrence := range program.occurrences {
		worker, ok := occurrence.labels["worker"]
		if !ok || worker.Presence.Kind != "optional" || worker.Domain.Kind != "open" ||
			worker.EndpointCardinality.Kind != "operational_population" || worker.Stability != "restart_stable" {
			t.Fatalf("occurrence %q worker schema = %#v", key, worker)
		}
	}

	embeddedOnlyRegistry := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}",
		"raw_branches: {embedded: {}}", 2)
	embeddedOnlyContract := loadTestSemanticContract(
		t,
		design,
		source,
		embeddedOnlyRegistry,
		validSourceRegistryGeneratorV1,
	)
	embeddedOnly, err := CompileSemanticContract(
		context.Background(), SemanticCompileInput{Contract: embeddedOnlyContract},
	)
	if err != nil {
		t.Fatalf("CompileSemanticContract(embedded-only) error = %v", err)
	}
	if got := embeddedOnly.normalizations[0].coverageBranches; !slices.Equal(got, []string{"embedded"}) {
		t.Fatalf("embedded-only normalization coverage branches = %v, want [embedded]", got)
	}

	invalidDesign := strings.Replace(design, "kind: operational_population", "kind: closed_domain", 1)
	invalidContract := loadTestSemanticContract(
		t,
		invalidDesign,
		source,
		validSourceRegistryV1,
		validSourceRegistryGeneratorV1,
	)
	_, err = CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: invalidContract})
	if err == nil || !strings.Contains(err.Error(), "closed_domain cardinality requires a closed domain") {
		t.Fatalf("CompileSemanticContract(closed identity domain) error = %v, want derived-domain failure", err)
	}
}

func TestCompileSemanticContractDerivesRegistryBackedNamespaceAlias(t *testing.T) {
	design, source, registry := namespaceAliasTestContract()
	contract := loadTestSemanticContract(
		t,
		design,
		source,
		registry,
		validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	if len(program.normalizations) != 1 {
		t.Fatalf("normalizations = %#v, want one", program.normalizations)
	}
	node := program.normalizations[0]
	if got := node.familyAliases["ray_requests"]; got != "example_requests" || len(node.familyAliases) != 1 {
		t.Fatalf("derived namespace aliases = %#v", node.familyAliases)
	}
	if got, want := node.coverageBranches, []string{"family:ray_requests"}; !sameStrings(got, want) {
		t.Fatalf("coverage branches = %v, want %v", got, want)
	}

	t.Run("missing same-signal canonical target", func(t *testing.T) {
		broken := strings.Replace(registry, "example_requests", "other_requests", 1)
		contract := loadTestSemanticContract(
			t,
			design,
			source,
			broken,
			validSourceRegistryGeneratorV1,
		)
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), `target family "example_requests" has no registration on signal "requests"`) {
			t.Fatalf("CompileSemanticContract() error = %v, want missing-target failure", err)
		}
	})
}

func compileTestSemanticContract(t *testing.T, design, source string) *CompiledSemanticContract {
	t.Helper()
	contract := loadTestSemanticContract(t, design, source, "", "")
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	return program
}

func onlyCompiledOccurrence(t *testing.T, program *CompiledSemanticContract) *compiledOccurrence {
	t.Helper()
	if len(program.occurrences) != 1 {
		t.Fatalf("occurrences = %#v, want one", program.occurrences)
	}
	for _, occurrence := range program.occurrences {
		return occurrence
	}
	panic("unreachable")
}

func sourceWithStatusLabelV1() string {
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  status_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: The source emits an HTTP response status.
environment:
`, 1)
	return strings.Replace(source, "    labels: {}", `    labels:
      status:
        meaning: HTTP response status.
        presence: required
        domain: {kind: closed, values: ["200", "429", invalid]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [status_label]`, 1)
}

func sourceWithStatusNormalizationV1() string {
	source := strings.Replace(sourceWithStatusLabelV1(), "environment:\n", `  status_class_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: The normalized label is an HTTP response-status class.
  status_normalization:
    kind: normalization
    upstream: exporter
    locations: [metrics.go:16]
    claim: Numeric status values map to finite status classes.
environment:
`, 1)
	return source
}

func designWithNormalizationsV1(normalizations string) string {
	return strings.Replace(validProfileDesignV1, "normalizations: {}", "normalizations:\n"+normalizations, 1)
}

func namespaceAliasTestContract() (design, source, registry string) {
	design = designWithNormalizationsV1(`  ray_namespace:
    kind: namespace_alias
    registry_group: ray_transport
    source_prefix: ray_
    target_prefix: example_
    evidence: [ray_namespace_equivalence]`)
	design = strings.Replace(design, "match: example_*", "match: example_* ray_*", 1)

	source = generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[native_requests, ray_requests]"),
		false,
	)
	source = strings.Replace(source, "registry_groups: [core]", "registry_groups: [native, ray_transport]", 1)
	source = strings.Replace(source, "evidence:\n", `evidence:
  ray_namespace_equivalence:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:14]
    claim: The Ray transport rewrites the native metric namespace without changing signal meaning.
`, 1)

	registry = `
version: v1
profile: example
generated: true
family_grammars: {}
groups:
  native:
    registrations:
      native_requests:
        family: {exact: example_requests}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 10}
  ray_transport:
    registrations:
      ray_requests:
        family: {exact: ray_requests}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 11}
`
	return design, source, registry
}

func categoryNormalizationV1(id, source, target string) string {
	return `  ` + id + `:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: ` + source + `
    target_label: ` + target + `
    exact: {"429": throttled}
    ranges: []
    missing: {leave_absent: true}
    malformed: {set: malformed}
    unknown: {set: other}
    output:
      meaning: Derived status class.
      evidence: [status_class_label]
    evidence: [status_normalization]`
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestCompileNormalizationDerivesOnlyRealizableCoverageBranches(t *testing.T) {
	design := designWithNormalizationsV1(categoryNormalizationV1("status_class", "status", "status_class"))
	design = strings.Replace(design, "required: []", "required: [status]", 1)
	design = strings.Replace(design, "dimensions: {}", "dimensions: {status_class: {render: label_value}}", 1)
	program := compileTestSemanticContract(t, design, sourceWithStatusNormalizationV1())
	if got, want := program.normalizations[0].coverageBranches,
		[]string{"exact:429", "malformed", "unknown"}; !sameStrings(got, want) {
		t.Fatalf("coverage branches = %v, want %v", got, want)
	}
}
