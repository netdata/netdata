// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import "testing"

func TestCompiledEnvironmentConditionOverlapAndCoverage(t *testing.T) {
	axes := map[string]EnvironmentAxis{
		"mode": {Kind: "enum", Values: []string{"single", "multi"}},
	}
	single := testCompiledCondition(testStringPredicate("mode", "eq", "single"))
	multi := testCompiledCondition(testStringPredicate("mode", "eq", "multi"))
	unconditional := unconditionalEnvironmentCondition()

	if single.overlaps(multi, axes) {
		t.Fatal("single and multi overlap")
	}
	if !unconditional.coveredBy(axes, single, multi) {
		t.Fatal("single and multi do not cover the enum axis")
	}
	if unconditional.coveredBy(axes, single) {
		t.Fatal("single unexpectedly covers the enum axis")
	}
}

func TestCompiledEnvironmentConditionIntegerCoverage(t *testing.T) {
	minimum, maximum := 1, 3
	axes := map[string]EnvironmentAxis{
		"workers": {Kind: "integer", Min: &minimum, Max: &maximum},
	}
	one := testCompiledCondition(testIntegerPredicate("workers", "eq", 1))
	two := testCompiledCondition(testIntegerPredicate("workers", "eq", 2))
	three := testCompiledCondition(testIntegerPredicate("workers", "eq", 3))
	if !unconditionalEnvironmentCondition().coveredBy(axes, one, two, three) {
		t.Fatal("three exact integer conditions do not cover [1,3]")
	}
}

func TestCompiledEnvironmentConditionEnumSetIsSymbolic(t *testing.T) {
	axes := map[string]EnvironmentAxis{
		"features": {Kind: "enum_set", Values: []string{"a", "b", "c", "d", "e", "f", "g", "h"}},
	}
	containsA := testCompiledCondition(testStringPredicate("features", "contains", "a"))
	excludesA := testCompiledCondition(testStringPredicate("features", "excludes", "a"))
	if containsA.overlaps(excludesA, axes) {
		t.Fatal("contains and excludes overlap")
	}
	if !unconditionalEnvironmentCondition().coveredBy(axes, containsA, excludesA) {
		t.Fatal("contains/excludes do not cover the enum-set domain")
	}

	positive := testCompiledCondition(
		testStringValuesPredicate("features", "contains_any", "a", "b"),
		testStringValuesPredicate("features", "contains_any", "a", "c"),
		testStringValuesPredicate("features", "contains_any", "b", "c"),
	)
	negative := testCompiledCondition(
		testStringValuesPredicate("features", "contains_all", "a", "b"),
	)
	if !positive.overlaps(negative, axes) {
		t.Fatal("expected satisfiable symbolic enum-set overlap")
	}

	unsatisfiable := []environmentLiteral{
		{predicate: testStringValuesPredicate("features", "contains_any", "a", "b")},
		{predicate: testStringValuesPredicate("features", "contains_any", "a", "c")},
		{predicate: testStringValuesPredicate("features", "contains_any", "b", "c")},
		{predicate: testStringValuesPredicate("features", "contains_all", "a", "b"), negated: true},
		{predicate: testStringValuesPredicate("features", "contains_all", "a", "c"), negated: true},
		{predicate: testStringValuesPredicate("features", "contains_all", "b", "c"), negated: true},
	}
	if environmentLiteralsSatisfiable(axes, unsatisfiable) {
		t.Fatal("symbolic enum-set solver accepted an unsatisfiable non-unit formula")
	}
}

func TestCompiledEnvironmentConditionEvaluation(t *testing.T) {
	condition := testCompiledCondition(testStringPredicate("mode", "eq", "single"))
	single := "single"
	multi := "multi"
	axes := map[string]EnvironmentAxis{"mode": {Kind: "enum", Values: []string{"single", "multi"}}}
	if !condition.evaluate(axes, map[string]AxisValue{"mode": {String: &single}}) {
		t.Fatal("single assignment did not match")
	}
	if condition.evaluate(axes, map[string]AxisValue{"mode": {String: &multi}}) {
		t.Fatal("multi assignment matched")
	}
}

func testCompiledCondition(predicates ...EnvironmentPredicate) compiledEnvironmentCondition {
	return compiledEnvironmentCondition{clauses: [][]EnvironmentPredicate{predicates}}
}

func testStringPredicate(axis, operation, value string) EnvironmentPredicate {
	return EnvironmentPredicate{Axis: axis, Op: operation, Value: &AxisValue{String: &value}}
}

func testStringValuesPredicate(axis, operation string, values ...string) EnvironmentPredicate {
	items := make([]AxisValue, 0, len(values))
	for index := range values {
		items = append(items, AxisValue{String: &values[index]})
	}
	return EnvironmentPredicate{Axis: axis, Op: operation, Values: items}
}

func testIntegerPredicate(axis, operation string, value int) EnvironmentPredicate {
	return EnvironmentPredicate{Axis: axis, Op: operation, Value: &AxisValue{Integer: &value}}
}
