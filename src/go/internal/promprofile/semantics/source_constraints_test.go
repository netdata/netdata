// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbsentLabelDoesNotRequireReduction(t *testing.T) {
	source := strings.Replace(sourceSemanticsWithResultLabel("optional"), "domain: {kind: closed, values: [success, '']}", "domain: {kind: open}", 1)
	source = strings.Replace(source, "endpoint_cardinality: {kind: closed_domain}", "endpoint_cardinality: {kind: bounded_configuration}", 1)
	design := strings.Replace(validProfileDesignV1, "components: [total]", "components: [total]\n        where: {any: [{all: [{label: result, op: absent}]}]}", 1)
	design = strings.Replace(design, "omit: {}", "omit: {result: The selector admits only the absent classification.}", 1)
	contract := loadTestSemanticContract(t, design, source, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	require.NoError(t, err)

	// An alternative admitting a populated value restores possible fan-in.
	design = strings.Replace(design, "op: absent}]}]", "op: absent}]}, {all: [{label: result, op: nonblank}]}]", 1)
	contract = loadTestSemanticContract(t, design, source, "", "")
	_, err = CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	require.ErrorContains(t, err, "requires reduction")
}

func TestDimensionDeterminesNormalizedCategoryWithoutReduction(t *testing.T) {
	design := designWithNormalizationsV1(`  status_class:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: status
    target_label: status_class
    ranges: [{min: 400, max: 499, value: client_error}]
    missing: {set: other}
    malformed: {set: other}
    unknown: {set: other}
    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
    evidence: [status_normalization]`)
	design = strings.Replace(design, "dimensions: {}", "dimensions: {status: {render: label_value}}", 1)
	design = strings.Replace(design, "omit: {}", "omit: {status_class: The raw status dimension determines its category.}", 1)
	contract := loadTestSemanticContract(t, design, sourceWithStatusNormalizationV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	require.NoError(t, err)
}

func TestUnsignedIntegerLabelSourceAndPredicates(t *testing.T) {
	source := strings.Replace(sourceSemanticsWithResultLabel("required"), "domain: {kind: closed, values: [success, '']}", "domain: {kind: unsigned_integer}", 1)
	source = strings.Replace(source, "endpoint_cardinality: {kind: closed_domain}", "endpoint_cardinality: {kind: bounded_configuration}", 1)
	design := strings.Replace(validProfileDesignV1, "required: []", "required: [result]", 1)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	require.NoError(t, err)
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"0", true}, {"200", true}, {"18446744073709551615", true},
		{"", false}, {"0200", false}, {"+200", false}, {"-1", false},
		{"200.0", false}, {"bad", false}, {"18446744073709551616", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			snapshot.Sources[0].Labels = semanticTestLabels("result", tc.value)
			_, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			assert.Equal(t, tc.valid, err == nil, "source reconciliation: %v", err)
			assert.Equal(t, tc.valid, labelValueMayMatch(program.signals["requests"].labels["result"], tc.value))
		})
	}
}

func TestUnsignedIntegerCategoryCoverage(t *testing.T) {
	other, malformed := "other", "malformed"
	definition := Normalization{
		Exact:   map[string]string{"200": "ok", "bad": "invalid"},
		Missing: &CategoryAction{Set: &other}, Malformed: &CategoryAction{Set: &malformed}, Unknown: &CategoryAction{Set: &other},
		Output: &NormalizedLabelOutput{Meaning: "Status category."},
	}
	source := SourceLabel{Presence: LabelPresence{Kind: "required"}, Domain: LabelDomain{Kind: "unsigned_integer"}}
	assert.ElementsMatch(t, []string{"exact:200", "unknown"}, categoryCoverageBranches(definition, source))
	assert.ElementsMatch(t, []string{"ok", "other"}, categoryOutputSchema(definition, source).Domain.Values)
	source.Presence.Kind = "optional"
	assert.Contains(t, categoryCoverageBranches(definition, source), "missing")
	source.Domain.Kind = "open"
	assert.Contains(t, categoryCoverageBranches(definition, source), "malformed", "open strings retain malformed coverage")
}

func TestUnsignedIntegerCategoryExhaustiveCoverage(t *testing.T) {
	zero, one, two, maximum := uint64(0), uint64(1), uint64(2), ^uint64(0)
	beforeMaximum := maximum - 1
	for _, tc := range []struct {
		name     string
		exact    map[string]string
		ranges   []CategoryRange
		complete bool
	}{
		{"full range", nil, []CategoryRange{{Min: &zero, Max: &maximum, Value: "all"}}, true},
		{"exact bridges range", map[string]string{"0": "zero", "1": "one"}, []CategoryRange{{Min: &two, Max: &maximum, Value: "rest"}}, true},
		{"gap", map[string]string{"0": "zero"}, []CategoryRange{{Min: &two, Max: &maximum, Value: "rest"}}, false},
		{"maximum exact", map[string]string{"18446744073709551615": "last"}, []CategoryRange{{Min: &zero, Max: &beforeMaximum, Value: "rest"}}, true},
		{"missing maximum", nil, []CategoryRange{{Min: &zero, Max: &beforeMaximum, Value: "rest"}}, false},
		{"adjacent ranges", nil, []CategoryRange{{Min: &zero, Max: &zero, Value: "zero"}, {Min: &one, Max: &maximum, Value: "rest"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := Normalization{Exact: tc.exact, Ranges: tc.ranges, Missing: &CategoryAction{}, Malformed: &CategoryAction{}, Unknown: &CategoryAction{}, Output: &NormalizedLabelOutput{Meaning: "Category."}}
			source := SourceLabel{Presence: LabelPresence{Kind: "required"}, Domain: LabelDomain{Kind: "unsigned_integer"}}
			assert.Equal(t, tc.complete, categoryOutputPresence(definition, source).keyIsAlwaysPresent())
			assert.Equal(t, !tc.complete, slices.Contains(categoryCoverageBranches(definition, source), "unknown"))
			unreachable := "unreachable"
			definition.Unknown.Set = &unreachable
			assert.Equal(t, !tc.complete, slices.Contains(categoryOutputSchema(definition, source).Domain.Values, unreachable))
			source.Presence.Kind = "optional"
			assert.False(t, categoryOutputPresence(definition, source).keyIsAlwaysPresent())
		})
	}
}
