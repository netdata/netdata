// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDocuments(t *testing.T) {
	directory := t.TempDir()
	designPath := filepath.Join(directory, ProfileDesignFilename)
	sourcePath := filepath.Join(directory, SourceFilename)
	writeTextFile(t, designPath, validProfileDesignV1)
	writeTextFile(t, sourcePath, validSourceSemanticsV1)

	design, err := LoadProfileDesign(designPath)
	if err != nil {
		t.Fatalf("LoadProfileDesign() error = %v", err)
	}
	if design.Profile != "example" || len(design.Views) != 1 {
		t.Fatalf("LoadProfileDesign() = %#v", design)
	}

	source, err := LoadSourceSemantics(sourcePath)
	if err != nil {
		t.Fatalf("LoadSourceSemantics() error = %v", err)
	}
	if source.Profile != "example" || len(source.Signals) != 1 {
		t.Fatalf("LoadSourceSemantics() = %#v", source)
	}
}

func TestLoadRejectsStrictShapeAndVersionErrors(t *testing.T) {
	tests := map[string]struct {
		filename string
		content  string
		load     func(string) error
		wantErr  string
	}{
		"design unknown nested field": {
			filename: ProfileDesignFilename,
			content:  strings.Replace(validProfileDesignV1, "grain: service", "grain: service\n    unknown: true", 1),
			load: func(path string) error {
				_, err := LoadProfileDesign(path)
				return err
			},
			wantErr: "field unknown not found",
		},
		"design missing required entities": {
			filename: ProfileDesignFilename,
			content:  strings.Replace(validProfileDesignV1, "entities:\n", "missing_entities:\n", 1),
			load: func(path string) error {
				_, err := LoadProfileDesign(path)
				return err
			},
			wantErr: "required field entities is missing",
		},
		"design wrong version": {
			filename: ProfileDesignFilename,
			content:  strings.Replace(validProfileDesignV1, "version: v1", "version: 1", 1),
			load: func(path string) error {
				_, err := LoadProfileDesign(path)
				return err
			},
			wantErr: "version",
		},
		"source wrong version": {
			filename: SourceFilename,
			content:  strings.Replace(validSourceSemanticsV1, "version: v1", "version: v2", 1),
			load: func(path string) error {
				_, err := LoadSourceSemantics(path)
				return err
			},
			wantErr: "version",
		},
		"source duplicate nested key": {
			filename: SourceFilename,
			content: strings.Replace(
				validSourceSemanticsV1,
				"repository: owner/exporter",
				"repository: owner/exporter\n    repository: owner/other",
				1,
			),
			load: func(path string) error {
				_, err := LoadSourceSemantics(path)
				return err
			},
			wantErr: "duplicate mapping key",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.filename)
			writeTextFile(t, path, tc.content)
			err := tc.load(path)
			if err == nil {
				t.Fatal("load error = nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("load error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestSourceSemanticsV1AcceptsReviewedSourceContracts(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1, "kind: cumulative", "kind: constant", 1)
	content = strings.Replace(content, "labels: {}", "labels:\n      position:\n        meaning: Speculative token position.\n        presence: required\n        domain: {kind: open}\n        endpoint_cardinality: {kind: bounded_configuration}\n        stability: stable\n        evidence: [position_label]", 1)
	content = strings.Replace(content, "  request_unit:\n", "  position_label:\n    kind: label\n    upstream: exporter\n    locations: [metrics.go:14]\n    claim: One finite label value is registered per configured speculative token.\n  request_unit:\n", 1)

	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	source, err := LoadSourceSemantics(path)
	if err != nil {
		t.Fatalf("LoadSourceSemantics() error = %v", err)
	}
	component := source.Signals["requests"].Components["total"]
	if component.Lifecycle.Kind != "constant" {
		t.Fatalf("lifecycle = %q, want constant", component.Lifecycle.Kind)
	}
	cardinality := source.Signals["requests"].Labels["position"].EndpointCardinality
	if cardinality.Kind != "bounded_configuration" || cardinality.Max != nil {
		t.Fatalf("cardinality = %#v, want bounded_configuration without max", cardinality)
	}
}

func TestProfileDesignAcceptsDottedRelativeContext(t *testing.T) {
	content := strings.Replace(validProfileDesignV1, "views:\n  requests:", "views:\n  traffic.requests:", 1)
	path := filepath.Join(t.TempDir(), ProfileDesignFilename)
	writeTextFile(t, path, content)
	if _, err := LoadProfileDesign(path); err != nil {
		t.Fatalf("LoadProfileDesign() error = %v", err)
	}
}

func TestProfileDesignRejectsUnknownInlineConditionField(t *testing.T) {
	content := strings.Replace(validProfileDesignV1, "supports: {}", `supports:
    runtime:
      when:
        any:
          - all:
              - axis: mode
                op: eq
                value: enabled
                unexpected: true`, 1)
	path := filepath.Join(t.TempDir(), ProfileDesignFilename)
	writeTextFile(t, path, content)
	if _, err := LoadProfileDesign(path); err == nil || !strings.Contains(err.Error(), "field unexpected not found") {
		t.Fatalf("LoadProfileDesign() error = %v, want strict nested-field failure", err)
	}
}

func TestProfileDesignCategoryRangesAreUnsignedSortedAndExactFirst(t *testing.T) {
	validCategory := `normalizations:
  status_class:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: status
    target_label: status_class
    exact: {"429": throttled}
    ranges:
      - {min: 400, max: 499, value: client_error}
      - {min: 500, max: 599, value: server_error}
    missing: {leave_absent: true}
    malformed: {set: malformed}
    unknown: {set: other}
    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
    evidence: [status_meaning]`

	tests := map[string]struct {
		normalizations string
		wantErr        string
	}{
		"exact override accepted": {normalizations: validCategory},
		"unsorted ranges": {
			normalizations: strings.Replace(validCategory,
				"- {min: 400, max: 499, value: client_error}\n      - {min: 500, max: 599, value: server_error}",
				"- {min: 500, max: 599, value: server_error}\n      - {min: 400, max: 499, value: client_error}", 1),
			wantErr: "unsorted or overlaps",
		},
		"negative range": {
			normalizations: strings.Replace(validCategory, "min: 400", "min: -1", 1),
			wantErr:        "cannot unmarshal !!int `-1` into uint64",
		},
		"missing output": {
			normalizations: strings.Replace(validCategory, `    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
`, "", 1),
			wantErr: ".output must be present",
		},
		"derived cardinality repeated": {
			normalizations: strings.Replace(validCategory,
				"      meaning: HTTP response-status class.",
				"      meaning: HTTP response-status class.\n      endpoint_cardinality: {kind: closed_domain}", 1),
			wantErr: "endpoint_cardinality is derived for kind category",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validProfileDesignV1, "normalizations: {}", tc.normalizations, 1)
			path := filepath.Join(t.TempDir(), ProfileDesignFilename)
			writeTextFile(t, path, content)
			_, err := LoadProfileDesign(path)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("LoadProfileDesign() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("LoadProfileDesign() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestProfileDesignEmbeddedIdentityExtractRequiresNonDerivedOutputSchema(t *testing.T) {
	valid := `normalizations:
  objecter_identity:
    kind: embedded_identity_extract
    registry_grammar: objecter_operation_family
    target_label: objecter_address
    retain: {family: canonical_branch, captured_identity: true}
    output:
      meaning: Ceph objecter client address.
      endpoint_cardinality: {kind: operational_population}
      stability: restart_stable
      evidence: [objecter_address_label]
    evidence: [objecter_identity_encoding]`

	tests := map[string]struct {
		normalizations string
		wantErr        string
	}{
		"complete output accepted": {normalizations: valid},
		"signal scope rejected": {
			normalizations: strings.Replace(valid, "    registry_grammar:",
				"    applies_to: {signal: requests, components: [total]}\n    registry_grammar:", 1),
			wantErr: "applies_to is not allowed",
		},
		"missing output": {
			normalizations: strings.Replace(valid, `    output:
      meaning: Ceph objecter client address.
      endpoint_cardinality: {kind: operational_population}
      stability: restart_stable
      evidence: [objecter_address_label]
`, "", 1),
			wantErr: ".output must be present",
		},
		"missing stability": {
			normalizations: strings.Replace(valid, "      stability: restart_stable\n", "", 1),
			wantErr:        ".output.stability must be present",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validProfileDesignV1, "normalizations: {}", tc.normalizations, 1)
			path := filepath.Join(t.TempDir(), ProfileDesignFilename)
			writeTextFile(t, path, content)
			_, err := LoadProfileDesign(path)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("LoadProfileDesign() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("LoadProfileDesign() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestProfileDesignEmbeddedIdentityRepairRequiresConstructedIdentityOutput(t *testing.T) {
	normalization := `normalizations:
  rgw_identity:
    kind: embedded_identity_repair
    registry_grammar: rgw_family
    source_identity_label: source_zone
    canonical: {family_prefix: example_zone_, identity_label: source_zone}
    embedded: {family_prefix: example_, capture: source_zone_fragment}
    identity:
      operands: [source_zone_fragment, source_zone]
      separator: _
      blank: omit_operand_and_separator
      sanitizer: prometheus_label_value
    duplicate_exclusion:
      when_identity_label: absent
      outcome: drop_before_writer
      evidence: [raw_duplicate]
    output:
      meaning: Canonical source-zone identity reconstructed from the source family and label.
      endpoint_cardinality: {kind: bounded_configuration}
      stability: stable
      evidence: [source_zone_label]
    evidence: [identity_encoding]`
	content := strings.Replace(validProfileDesignV1, "normalizations: {}", normalization, 1)
	path := filepath.Join(t.TempDir(), ProfileDesignFilename)
	writeTextFile(t, path, content)
	if _, err := LoadProfileDesign(path); err != nil {
		t.Fatalf("LoadProfileDesign() error = %v", err)
	}

	withoutOutput := strings.Replace(normalization, `    output:
      meaning: Canonical source-zone identity reconstructed from the source family and label.
      endpoint_cardinality: {kind: bounded_configuration}
      stability: stable
      evidence: [source_zone_label]
`, "", 1)
	content = strings.Replace(validProfileDesignV1, "normalizations: {}", withoutOutput, 1)
	path = filepath.Join(t.TempDir(), ProfileDesignFilename)
	writeTextFile(t, path, content)
	if _, err := LoadProfileDesign(path); err == nil || !strings.Contains(err.Error(), "output must be present") {
		t.Fatalf("LoadProfileDesign() error = %v, want missing-output failure", err)
	}

	withSignalScope := strings.Replace(withoutOutput, "    registry_grammar:",
		"    applies_to: {signal: requests, components: [total]}\n    registry_grammar:", 1)
	content = strings.Replace(validProfileDesignV1, "normalizations: {}", withSignalScope, 1)
	path = filepath.Join(t.TempDir(), ProfileDesignFilename)
	writeTextFile(t, path, content)
	if _, err := LoadProfileDesign(path); err == nil || !strings.Contains(err.Error(), "applies_to is not allowed") {
		t.Fatalf("LoadProfileDesign() error = %v, want signal-scope failure", err)
	}
}

func TestSourceSemanticsV1UntypedClassificationAndQuantileFreeSummary(t *testing.T) {
	t.Run("untyped requires classification", func(t *testing.T) {
		content := strings.Replace(validSourceSemanticsV1,
			"prometheus: {type: counter, shape: scalar}",
			"prometheus: {type: untyped, shape: scalar}", 1)
		path := filepath.Join(t.TempDir(), SourceFilename)
		writeTextFile(t, path, content)
		if _, err := LoadSourceSemantics(path); err == nil || !strings.Contains(err.Error(), "classification") {
			t.Fatalf("LoadSourceSemantics() error = %v, want classification failure", err)
		}

		content = strings.Replace(content,
			"prometheus: {type: untyped, shape: scalar}",
			"prometheus: {type: untyped, shape: scalar, classification: counter}", 1)
		writeTextFile(t, path, content)
		if _, err := LoadSourceSemantics(path); err != nil {
			t.Fatalf("LoadSourceSemantics(classified) error = %v", err)
		}
	})

	t.Run("summary count and sum without quantile", func(t *testing.T) {
		content := strings.Replace(validSourceSemanticsV1,
			"prometheus: {type: counter, shape: scalar}",
			"prometheus: {type: summary, shape: summary}", 1)
		content = strings.Replace(content, validScalarComponentV1, validQuantileFreeSummaryComponentsV1, 1)
		path := filepath.Join(t.TempDir(), SourceFilename)
		writeTextFile(t, path, content)
		document, err := LoadSourceSemantics(path)
		if err != nil {
			t.Fatalf("LoadSourceSemantics() error = %v", err)
		}
		components := document.Signals["requests"].Components
		if len(components) != 2 || components["count"].WireRole != "summary_count" ||
			components["sum"].WireRole != "summary_sum" {
			t.Fatalf("summary components = %#v, want count+sum without quantile", components)
		}
	})
}

func TestSourceSemanticsV1RejectsIncompatibleEvidenceInAConsumer(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1,
		"evidence: [request_lifecycle]",
		"evidence: [request_lifecycle, request_unit]", 1)
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceSemantics(path); err == nil || !strings.Contains(err.Error(), "incompatible kind") {
		t.Fatalf("LoadSourceSemantics() error = %v, want incompatible-evidence failure", err)
	}
}

func TestSourceSemanticsV1RejectsContradictoryAndMalformedConditions(t *testing.T) {
	tests := map[string]struct {
		environment string
		wantErr     string
	}{
		"contradictory enum": {
			environment: `environment:
  axes:
    mode:
      kind: enum
      values: [single, multi]
      meaning: Exporter process mode.
      evidence: [mode_availability]
  policies:
    impossible:
      when:
        any:
          - all:
              - {axis: mode, op: eq, value: single}
              - {axis: mode, op: eq, value: multi}
      evidence: [mode_availability]`,
			wantErr: "contradictory predicates",
		},
		"enum set scalar operation rejects a sequence": {
			environment: `environment:
  axes:
    features:
      kind: enum_set
      values: [a, b]
      meaning: Enabled exporter features.
      evidence: [mode_availability]
  policies:
    malformed:
      when:
        any:
          - all:
              - {axis: features, op: contains, value: [a, b]}
      evidence: [mode_availability]`,
			wantErr: "individual strings",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validSourceSemanticsV1, "environment:\n  axes: {}\n  policies: {}", tc.environment, 1)
			content = strings.Replace(content, "evidence:\n", `evidence:
  mode_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:9]
    claim: The source defines the exporter mode.
`, 1)
			path := filepath.Join(t.TempDir(), SourceFilename)
			writeTextFile(t, path, content)
			if _, err := LoadSourceSemantics(path); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadSourceSemantics() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSourceSemanticsV1AcceptsClosedDomainContributors(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  pid_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: The source uses one label value for each configured worker.
  worker_topology:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:9]
    claim: Worker contributions are additive.
`, 1)
	content = strings.Replace(content, "labels: {}", `labels:
      pid:
        meaning: Configured worker identity.
        presence: required
        domain: {kind: closed, values: [one, two]}
        endpoint_cardinality: {kind: closed_domain}
        stability: restart_stable
        evidence: [pid_label]`, 1)
	content = strings.Replace(content, "    functional_dependencies: {}", `    functional_dependencies: {}
    contributors:
      variants:
        workers:
          identity: [pid]
          cardinality: {kind: closed_domain}
          concurrency: may_coexist
          value_model: {total: additive}
          membership: {stability: restart_stable}
          reset: {scope: per_contributor}
          join: {new_contributor_baseline: unknown}
          evidence:
            population: [request_population]
            lifecycle: [request_lifecycle]
            relationship: [worker_topology]`, 1)
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceSemantics(path); err != nil {
		t.Fatalf("LoadSourceSemantics() error = %v", err)
	}
}

func TestSourceSemanticsV1AcceptsEmptyPrometheusLabelValueInClosedDomain(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  result_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: The exporter uses an empty result label when no classification applies.
`, 1)
	content = strings.Replace(content, "labels: {}", `labels:
      result:
        meaning: Optional source classification.
        presence: optional
        domain: {kind: closed, values: [success, '']}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [result_label]`, 1)
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceSemantics(path); err != nil {
		t.Fatalf("LoadSourceSemantics() error = %v", err)
	}
}

func TestSourceSemanticsV1AcceptsPresentLabelPresence(t *testing.T) {
	content := sourceSemanticsWithResultLabel("present")
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	source, err := LoadSourceSemantics(path)
	if err != nil {
		t.Fatalf("LoadSourceSemantics() error = %v", err)
	}
	if got := source.Signals["requests"].Labels["result"].Presence.Kind; got != "present" {
		t.Fatalf("presence = %q, want present", got)
	}
}

func sourceSemanticsWithResultLabel(presence string) string {
	content := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  result_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: The exporter always includes the result key and uses an empty value when no classification applies.
`, 1)
	return strings.Replace(content, "labels: {}", `labels:
      result:
        meaning: Source classification.
        presence: `+presence+`
        domain: {kind: closed, values: [success, '']}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [result_label]`, 1)
}

func TestSourceSemanticsV1StateEncodingRequiresCurrentStateComponent(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  state_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: The source emits a closed operational state.
  state_encoding:
    kind: state_encoding
    upstream: exporter
    locations: [metrics.go:9]
    claim: Exactly one state is active.
`, 1)
	content = strings.Replace(content, "quantity: count", "quantity: state", 1)
	content = strings.Replace(content, "labels: {}", `labels:
      state:
        meaning: Operational state.
        presence: required
        domain: {kind: closed, values: [up, down]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [state_label]`, 1)
	content = strings.Replace(content, "state_encodings: {}", `state_encodings:
  operational:
    signal: requests
    component: total
    label: state
    states: [up, down]
    encoding: one_hot_exactly_one
    evidence: [state_encoding]`, 1)
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceSemantics(path); err == nil || !strings.Contains(err.Error(), "current state component") {
		t.Fatalf("LoadSourceSemantics() error = %v, want state-component failure", err)
	}

	content = strings.Replace(content, "prometheus: {type: counter, shape: scalar}", "prometheus: {type: gauge, shape: scalar}", 1)
	content = strings.Replace(content, "kind: cumulative", "kind: current", 1)
	writeTextFile(t, path, content)
	if _, err := LoadSourceSemantics(path); err != nil {
		t.Fatalf("LoadSourceSemantics(current state) error = %v", err)
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const validProfileDesignV1 = `
version: v1
profile: example
match: example_*
namespace: example
composition:
  supports: {}
entities:
  service:
    grain: service
    identity:
      required: []
      optional: []
label_policies: {}
reduction_policies: {}
normalizations: {}
exclusions: {}
limitations: {}
views:
  requests:
    family: Traffic/Requests
    question: How many requests complete?
    entity: service
    inputs:
      requests:
        signal: requests
        components: [total]
    labels:
      dimensions: {}
      promote: []
      omit: {}
`

const validScalarComponentV1 = `    components:
      total:
        wire_role: scalar
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed
          evidence: [request_unit]`

const validQuantileFreeSummaryComponentsV1 = `    components:
      count:
        wire_role: summary_count
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
        wire_role: summary_sum
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: duration
          base: second
          rate: none
          object: request_time
          aspect: completed
          evidence: [request_unit]`

const validSourceSemanticsV1 = `
version: v1
profile: example
upstreams:
  exporter:
    repository: owner/exporter
    commit: 0123456789abcdef0123456789abcdef01234567
evidence:
  request_registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:10]
    claim: The source registers one request counter.
  request_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:11]
    claim: One increment represents one completed request.
  request_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:12]
    claim: The counter resets only with process state.
  request_unit:
    kind: unit
    upstream: exporter
    locations: [metrics.go:13]
    claim: The counter measures completed requests.
environment:
  axes: {}
  policies: {}
component_policies: {}
label_policies: {}
signals:
  requests:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [request_population]
    components:
      total:
        wire_role: scalar
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
    labels: {}
    functional_dependencies: {}
relationships: {}
state_encodings: {}
source_exclusions: {}
`
