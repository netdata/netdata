// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

type compiledEnvironment struct {
	axes     map[string]EnvironmentAxis
	policies map[string]compiledEnvironmentCondition
}

type compiledEnvironmentCondition struct {
	clauses [][]EnvironmentPredicate
}

type environmentLiteral struct {
	predicate EnvironmentPredicate
	negated   bool
}

func compileEnvironment(document SourceSemanticsDocument) compiledEnvironment {
	compiled := compiledEnvironment{
		axes:     document.Environment.Axes,
		policies: make(map[string]compiledEnvironmentCondition, len(document.Environment.Policies)),
	}
	for id, policy := range document.Environment.Policies {
		compiled.policies[id] = compileEnvironmentCondition(policy.When)
	}
	return compiled
}

func compileEnvironmentCondition(condition EnvironmentCondition) compiledEnvironmentCondition {
	compiled := compiledEnvironmentCondition{clauses: make([][]EnvironmentPredicate, 0, len(condition.Any))}
	for _, clause := range condition.Any {
		compiled.clauses = append(compiled.clauses, slices.Clone(clause.All))
	}
	return compiled
}

func (e compiledEnvironment) resolve(use ConditionUse) (compiledEnvironmentCondition, error) {
	if use.IsZero() {
		return unconditionalEnvironmentCondition(), nil
	}
	if use.Policy != "" {
		condition, ok := e.policies[use.Policy]
		if !ok {
			return compiledEnvironmentCondition{}, fmt.Errorf("unknown environment policy %q", use.Policy)
		}
		return condition, nil
	}
	if err := validateEnvironmentCondition("condition", *use.Inline, e.axes); err != nil {
		return compiledEnvironmentCondition{}, err
	}
	return compileEnvironmentCondition(*use.Inline), nil
}

func unconditionalEnvironmentCondition() compiledEnvironmentCondition {
	return compiledEnvironmentCondition{clauses: [][]EnvironmentPredicate{{}}}
}

func impossibleEnvironmentCondition() compiledEnvironmentCondition {
	return compiledEnvironmentCondition{}
}

func (c compiledEnvironmentCondition) and(other compiledEnvironmentCondition, axes map[string]EnvironmentAxis) compiledEnvironmentCondition {
	if len(c.clauses) == 0 || len(other.clauses) == 0 {
		return impossibleEnvironmentCondition()
	}
	result := compiledEnvironmentCondition{
		clauses: make([][]EnvironmentPredicate, 0, len(c.clauses)*len(other.clauses)),
	}
	for _, left := range c.clauses {
		for _, right := range other.clauses {
			combined := append(slices.Clone(left), right...)
			if environmentLiteralsSatisfiable(axes, predicatesAsLiterals(combined)) {
				result.clauses = append(result.clauses, combined)
			}
		}
	}
	return result
}

func (c compiledEnvironmentCondition) overlaps(other compiledEnvironmentCondition, axes map[string]EnvironmentAxis) bool {
	for _, left := range c.clauses {
		for _, right := range other.clauses {
			literals := predicatesAsLiterals(left)
			literals = append(literals, predicatesAsLiterals(right)...)
			if environmentLiteralsSatisfiable(axes, literals) {
				return true
			}
		}
	}
	return false
}

func (c compiledEnvironmentCondition) coveredBy(
	axes map[string]EnvironmentAxis,
	covering ...compiledEnvironmentCondition,
) bool {
	for _, baseClause := range c.clauses {
		blockers := make([][]environmentLiteral, 0)
		for _, condition := range covering {
			for _, coverClause := range condition.clauses {
				alternatives := make([]environmentLiteral, 0, len(coverClause))
				for _, predicate := range coverClause {
					alternatives = append(alternatives, environmentLiteral{predicate: predicate, negated: true})
				}
				blockers = append(blockers, alternatives)
			}
		}
		slices.SortFunc(blockers, func(left, right []environmentLiteral) int {
			return len(left) - len(right)
		})
		if environmentUncoveredWitnessExists(axes, predicatesAsLiterals(baseClause), blockers, 0) {
			return false
		}
	}
	return true
}

func environmentUncoveredWitnessExists(
	axes map[string]EnvironmentAxis,
	literals []environmentLiteral,
	blockers [][]environmentLiteral,
	index int,
) bool {
	if !environmentLiteralsSatisfiable(axes, literals) {
		return false
	}
	if index == len(blockers) {
		return true
	}
	for _, alternative := range blockers[index] {
		next := append(slices.Clone(literals), alternative)
		if environmentUncoveredWitnessExists(axes, next, blockers, index+1) {
			return true
		}
	}
	return false
}

func predicatesAsLiterals(predicates []EnvironmentPredicate) []environmentLiteral {
	literals := make([]environmentLiteral, 0, len(predicates))
	for _, predicate := range predicates {
		literals = append(literals, environmentLiteral{predicate: predicate})
	}
	return literals
}

func environmentLiteralsSatisfiable(
	axes map[string]EnvironmentAxis,
	literals []environmentLiteral,
) bool {
	byAxis := make(map[string][]environmentLiteral)
	for _, literal := range literals {
		byAxis[literal.predicate.Axis] = append(byAxis[literal.predicate.Axis], literal)
	}
	for axisID, axisLiterals := range byAxis {
		axis, ok := axes[axisID]
		if !ok || !axisLiteralsSatisfiable(axis, axisLiterals) {
			return false
		}
	}
	return true
}

func axisLiteralsSatisfiable(axis EnvironmentAxis, literals []environmentLiteral) bool {
	switch axis.Kind {
	case "enum", "ordered_enum":
		for _, value := range axis.Values {
			if axisStringValueSatisfies(axis, value, literals) {
				return true
			}
		}
		return false
	case "integer":
		minimum, maximum := *axis.Min, *axis.Max
		excluded := make(map[int]struct{})
		var finite map[int]struct{}
		for _, literal := range literals {
			predicate := literal.predicate
			switch predicate.Op {
			case "min", "max":
				value := *predicate.Value.Integer
				if literal.negated {
					if predicate.Op == "min" {
						if value == axisMinimumInt() {
							return false
						}
						maximum = min(maximum, value-1)
					} else {
						if value == axisMaximumInt() {
							return false
						}
						minimum = max(minimum, value+1)
					}
				} else if predicate.Op == "min" {
					minimum = max(minimum, value)
				} else {
					maximum = min(maximum, value)
				}
			case "eq", "in":
				values := integerPredicateValues(predicate)
				if literal.negated {
					for _, value := range values {
						excluded[value] = struct{}{}
					}
				} else {
					finite = intersectIntSets(finite, intSliceSet(values))
				}
			}
		}
		if minimum > maximum {
			return false
		}
		if finite != nil {
			for value := range finite {
				if value >= minimum && value <= maximum {
					if _, blocked := excluded[value]; !blocked {
						return true
					}
				}
			}
			return false
		}
		excludedCount := len(excludedValuesInRange(excluded, minimum, maximum))
		span := maximum - minimum
		if minimum < 0 && maximum >= 0 && span < 0 {
			return true
		}
		return span >= excludedCount
	case "enum_set":
		return enumSetLiteralsSatisfiable(axis.Values, literals)
	default:
		return false
	}
}

func axisMinimumInt() int {
	return -int(^uint(0)>>1) - 1
}

func axisMaximumInt() int {
	return int(^uint(0) >> 1)
}

func axisStringValueSatisfies(axis EnvironmentAxis, value string, literals []environmentLiteral) bool {
	for _, literal := range literals {
		matches := stringValueMatchesPredicate(axis, value, literal.predicate)
		if literal.negated == matches {
			return false
		}
	}
	return true
}

func stringValueMatchesPredicate(axis EnvironmentAxis, value string, predicate EnvironmentPredicate) bool {
	switch predicate.Op {
	case "eq":
		return value == *predicate.Value.String
	case "in":
		return slices.ContainsFunc(predicate.Values, func(item AxisValue) bool {
			return value == *item.String
		})
	case "at_least":
		return slices.Index(axis.Values, value) >= slices.Index(axis.Values, *predicate.Value.String)
	case "at_most":
		return slices.Index(axis.Values, value) <= slices.Index(axis.Values, *predicate.Value.String)
	default:
		return false
	}
}

func integerPredicateValues(predicate EnvironmentPredicate) []int {
	if predicate.Value != nil {
		return []int{*predicate.Value.Integer}
	}
	values := make([]int, 0, len(predicate.Values))
	for _, value := range predicate.Values {
		values = append(values, *value.Integer)
	}
	return values
}

func intSliceSet(values []int) map[int]struct{} {
	result := make(map[int]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func excludedValuesInRange(values map[int]struct{}, minimum, maximum int) map[int]struct{} {
	result := make(map[int]struct{})
	for value := range values {
		if value >= minimum && value <= maximum {
			result[value] = struct{}{}
		}
	}
	return result
}

func enumSetLiteralsSatisfiable(values []string, literals []environmentLiteral) bool {
	index := make(map[string]int, len(values))
	for position, value := range values {
		index[value] = position
	}
	clauses := make([]setClause, 0, len(literals))
	for _, literal := range literals {
		clauses = append(clauses, enumSetLiteralClauses(index, literal)...)
	}
	assignment := make([]int8, len(values))
	return solveSetClauses(clauses, assignment)
}

type setClause struct {
	positive bool
	values   []int
}

func enumSetLiteralClauses(index map[string]int, literal environmentLiteral) []setClause {
	predicate := literal.predicate
	values := predicateStringValues(predicate)
	switch predicate.Op {
	case "contains":
		return []setClause{{positive: !literal.negated, values: []int{index[values[0]]}}}
	case "excludes":
		return []setClause{{positive: literal.negated, values: []int{index[values[0]]}}}
	case "contains_all":
		if literal.negated {
			return []setClause{{positive: false, values: stringIndexes(index, values)}}
		}
		clauses := make([]setClause, 0, len(values))
		for _, value := range values {
			clauses = append(clauses, setClause{positive: true, values: []int{index[value]}})
		}
		return clauses
	case "contains_any":
		if literal.negated {
			clauses := make([]setClause, 0, len(values))
			for _, value := range values {
				clauses = append(clauses, setClause{positive: false, values: []int{index[value]}})
			}
			return clauses
		}
		return []setClause{{positive: true, values: stringIndexes(index, values)}}
	default:
		return nil
	}
}

func predicateStringValues(predicate EnvironmentPredicate) []string {
	if predicate.Value != nil {
		return []string{*predicate.Value.String}
	}
	values := make([]string, 0, len(predicate.Values))
	for _, value := range predicate.Values {
		values = append(values, *value.String)
	}
	return values
}

func stringIndexes(index map[string]int, values []string) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, index[value])
	}
	return result
}

func solveSetClauses(clauses []setClause, assignment []int8) bool {
	for {
		changed := false
		for _, clause := range clauses {
			satisfied, undecided, last := evaluateSetClause(clause, assignment)
			if satisfied {
				continue
			}
			if undecided == 0 {
				return false
			}
			if undecided == 1 {
				want := int8(-1)
				if clause.positive {
					want = 1
				}
				if assignment[last] != 0 && assignment[last] != want {
					return false
				}
				if assignment[last] == 0 {
					assignment[last] = want
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	allSatisfied := true
	branch := -1
	for _, clause := range clauses {
		satisfied, undecided, last := evaluateSetClause(clause, assignment)
		if satisfied {
			continue
		}
		allSatisfied = false
		if undecided > 0 {
			branch = last
			break
		}
	}
	if allSatisfied {
		return true
	}
	if branch < 0 {
		return false
	}
	for _, value := range []int8{1, -1} {
		next := slices.Clone(assignment)
		next[branch] = value
		if solveSetClauses(clauses, next) {
			return true
		}
	}
	return false
}

func evaluateSetClause(clause setClause, assignment []int8) (satisfied bool, undecided int, last int) {
	for _, index := range clause.values {
		value := assignment[index]
		if clause.positive && value == 1 || !clause.positive && value == -1 {
			return true, 0, -1
		}
		if value == 0 {
			undecided++
			last = index
		}
	}
	return false, undecided, last
}

func (c compiledEnvironmentCondition) evaluate(
	axes map[string]EnvironmentAxis,
	assignment map[string]AxisValue,
) bool {
	for _, clause := range c.clauses {
		matched := true
		for _, predicate := range clause {
			if !environmentPredicateMatches(axes[predicate.Axis], assignment[predicate.Axis], predicate) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func environmentPredicateMatches(axis EnvironmentAxis, value AxisValue, predicate EnvironmentPredicate) bool {
	switch predicate.Op {
	case "eq":
		return axisValueKey(value) == axisValueKey(*predicate.Value)
	case "in":
		return slices.ContainsFunc(predicate.Values, func(item AxisValue) bool {
			return axisValueKey(value) == axisValueKey(item)
		})
	case "at_least", "at_most":
		actual := slices.Index(axis.Values, *value.String)
		boundary := slices.Index(axis.Values, *predicate.Value.String)
		if predicate.Op == "at_least" {
			return actual >= boundary
		}
		return actual <= boundary
	case "min":
		return *value.Integer >= *predicate.Value.Integer
	case "max":
		return *value.Integer <= *predicate.Value.Integer
	case "contains":
		return slices.Contains(value.Strings, *predicate.Value.String)
	case "contains_all":
		for _, item := range predicate.Values {
			if !slices.Contains(value.Strings, *item.String) {
				return false
			}
		}
		return true
	case "contains_any":
		return slices.ContainsFunc(predicate.Values, func(item AxisValue) bool {
			return slices.Contains(value.Strings, *item.String)
		})
	case "excludes":
		return !slices.Contains(value.Strings, *predicate.Value.String)
	default:
		return false
	}
}
