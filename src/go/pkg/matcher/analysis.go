// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrAnalysisBudgetExceeded = errors.New("matcher analysis budget exceeded")

const (
	defaultAnalysisMaxStates      = 20_000
	defaultAnalysisMaxTransitions = 500_000
)

// AnalysisBudget bounds state-space analysis. A zero field uses its documented
// default; a negative field is invalid.
type AnalysisBudget struct {
	MaxStates      int
	MaxTransitions int
}

// Analyzer performs multiple intersection queries against one aggregate work
// budget. It is not goroutine-safe.
type Analyzer struct {
	ctx   context.Context
	meter *analysisMeter
}

// NewAnalyzer creates a cancelable analysis session. Its budget is shared by
// every query made through the returned Analyzer.
func NewAnalyzer(ctx context.Context, budget AnalysisBudget) (*Analyzer, error) {
	if ctx == nil {
		return nil, errors.New("nil matcher analysis context")
	}
	meter, err := newAnalysisMeter(budget)
	if err != nil {
		return nil, err
	}
	return &Analyzer{ctx: ctx, meter: meter}, nil
}

// GlobIntersectionWitness returns the shortest deterministic string matched by
// both globs and none of exclude. The query consumes the Analyzer's aggregate
// work budget.
func (a *Analyzer) GlobIntersectionWitness(
	leftPattern string,
	rightPattern string,
	exclude []string,
	requireNonEmpty bool,
) (witness string, intersects bool, err error) {
	if a == nil || a.ctx == nil || a.meter == nil {
		return "", false, errors.New("nil matcher analyzer")
	}
	return globIntersectionWitness(
		a.ctx, leftPattern, rightPattern, exclude, requireNonEmpty, a.meter,
	)
}

// SimplePatternIntersectionWitness returns the shortest deterministic string
// matched by both ordered simple-pattern expressions. The query consumes the
// Analyzer's aggregate work budget.
func (a *Analyzer) SimplePatternIntersectionWitness(
	leftExpr string,
	rightExpr string,
	requireNonEmpty bool,
) (witness string, intersects bool, err error) {
	if a == nil || a.ctx == nil || a.meter == nil {
		return "", false, errors.New("nil matcher analyzer")
	}
	left, err := parseSimplePatternAnalysisBranches(leftExpr)
	if err != nil {
		return "", false, err
	}
	right, err := parseSimplePatternAnalysisBranches(rightExpr)
	if err != nil {
		return "", false, err
	}
	var shortest string
	shortestRunes := 0
	found := false
	for _, leftBranch := range left {
		for _, rightBranch := range right {
			excluded := append(slices.Clone(leftBranch.earlierNegatives), rightBranch.earlierNegatives...)
			witness, intersects, err := globIntersectionWitness(
				a.ctx, leftBranch.pattern, rightBranch.pattern, excluded, requireNonEmpty, a.meter,
			)
			if err != nil {
				return "", false, err
			}
			if !intersects {
				continue
			}
			witnessRunes := utf8.RuneCountInString(witness)
			if !found || witnessRunes < shortestRunes {
				shortest = witness
				shortestRunes = witnessRunes
				found = true
			}
		}
	}
	return shortest, found, nil
}

type analysisMeter struct {
	budget      AnalysisBudget
	states      int
	transitions int
}

func newAnalysisMeter(budget AnalysisBudget) (*analysisMeter, error) {
	if budget.MaxStates < 0 || budget.MaxTransitions < 0 {
		return nil, fmt.Errorf("invalid negative matcher analysis budget")
	}
	if budget.MaxStates == 0 {
		budget.MaxStates = defaultAnalysisMaxStates
	}
	if budget.MaxTransitions == 0 {
		budget.MaxTransitions = defaultAnalysisMaxTransitions
	}
	return &analysisMeter{budget: budget}, nil
}

func (m *analysisMeter) addState() error {
	if m.states >= m.budget.MaxStates {
		return fmt.Errorf("%w: states reached %d", ErrAnalysisBudgetExceeded, m.budget.MaxStates)
	}
	m.states++
	return nil
}

func (m *analysisMeter) addTransition() error {
	if m.transitions >= m.budget.MaxTransitions {
		return fmt.Errorf("%w: transitions reached %d", ErrAnalysisBudgetExceeded, m.budget.MaxTransitions)
	}
	m.transitions++
	return nil
}

type simplePatternAnalysisBranch struct {
	pattern          string
	earlierNegatives []string
}

func parseSimplePatternAnalysisBranches(expr string) ([]simplePatternAnalysisBranch, error) {
	var branches []simplePatternAnalysisBranch
	var negatives []string
	for term := range strings.FieldsSeq(expr) {
		if after, ok := strings.CutPrefix(term, "!"); ok {
			if _, err := parseAnalysisGlob(after); err != nil {
				return nil, err
			}
			negatives = append(negatives, after)
			continue
		}
		if _, err := parseAnalysisGlob(term); err != nil {
			return nil, err
		}
		branches = append(branches, simplePatternAnalysisBranch{
			pattern:          term,
			earlierNegatives: slices.Clone(negatives),
		})
	}
	return branches, nil
}

type analysisGlobToken struct {
	kind    byte
	literal rune
	ranges  []analysisGlobRange
	negated bool
}

type analysisGlobRange struct {
	lo rune
	hi rune
}

const (
	analysisGlobLiteral byte = iota
	analysisGlobAny
	analysisGlobStar
	analysisGlobClass
)

type globProductState struct {
	left     []int
	right    []int
	negative [][]int
	started  bool
}

type globProductNode struct {
	state globProductState
	prev  int
	via   rune
}

func globIntersectionWitness(
	ctx context.Context,
	leftPattern string,
	rightPattern string,
	negativePatterns []string,
	requireNonEmpty bool,
	meter *analysisMeter,
) (string, bool, error) {
	left, err := parseAnalysisGlob(leftPattern)
	if err != nil {
		return "", false, err
	}
	right, err := parseAnalysisGlob(rightPattern)
	if err != nil {
		return "", false, err
	}
	negative := make([][]analysisGlobToken, 0, len(negativePatterns))
	patterns := [][]analysisGlobToken{left, right}
	for _, pattern := range negativePatterns {
		parsed, err := parseAnalysisGlob(pattern)
		if err != nil {
			return "", false, err
		}
		negative = append(negative, parsed)
		patterns = append(patterns, parsed)
	}
	alphabet := analysisGlobIntersectionAlphabet(patterns...)

	start := globProductState{
		left:     analysisGlobEpsilonClosure(left, 0),
		right:    analysisGlobEpsilonClosure(right, 0),
		negative: make([][]int, len(negative)),
	}
	for i := range negative {
		start.negative[i] = analysisGlobEpsilonClosure(negative[i], 0)
	}
	if err := meter.addState(); err != nil {
		return "", false, err
	}
	nodes := []globProductNode{{state: start, prev: -1}}
	seen := map[string]int{analysisGlobProductStateKey(start): 0}

	for currentIndex := 0; currentIndex < len(nodes); currentIndex++ {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		current := nodes[currentIndex].state
		if (!requireNonEmpty || current.started) && analysisGlobPositionSetAccepts(current.left, len(left)) &&
			analysisGlobPositionSetAccepts(current.right, len(right)) &&
			!analysisGlobAnyAccepts(current.negative, negative) {
			return reconstructGlobWitness(nodes, currentIndex), true, nil
		}

		for _, candidate := range alphabet {
			if err := meter.addTransition(); err != nil {
				return "", false, err
			}
			leftNext := analysisGlobNextPositionSet(left, current.left, candidate)
			if len(leftNext) == 0 {
				continue
			}
			rightNext := analysisGlobNextPositionSet(right, current.right, candidate)
			if len(rightNext) == 0 {
				continue
			}
			negativeNext := make([][]int, len(negative))
			for i := range negative {
				negativeNext[i] = analysisGlobNextPositionSet(negative[i], current.negative[i], candidate)
			}
			next := globProductState{left: leftNext, right: rightNext, negative: negativeNext, started: true}
			key := analysisGlobProductStateKey(next)
			if _, ok := seen[key]; ok {
				continue
			}
			if err := meter.addState(); err != nil {
				return "", false, err
			}
			seen[key] = len(nodes)
			nodes = append(nodes, globProductNode{state: next, prev: currentIndex, via: candidate})
		}
	}
	return "", false, nil
}

func reconstructGlobWitness(nodes []globProductNode, index int) string {
	var reversed []rune
	for index >= 0 && nodes[index].prev >= 0 {
		reversed = append(reversed, nodes[index].via)
		index = nodes[index].prev
	}
	slices.Reverse(reversed)
	return string(reversed)
}

func analysisGlobAnyAccepts(positions [][]int, patterns [][]analysisGlobToken) bool {
	for i := range patterns {
		if analysisGlobPositionSetAccepts(positions[i], len(patterns[i])) {
			return true
		}
	}
	return false
}

func analysisGlobNextPositionSet(pattern []analysisGlobToken, positions []int, candidate rune) []int {
	seen := make(map[int]struct{})
	for _, position := range positions {
		for _, next := range analysisGlobNextPositions(pattern, position, candidate) {
			seen[next] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func analysisGlobPositionSetAccepts(positions []int, end int) bool {
	_, ok := slices.BinarySearch(positions, end)
	return ok
}

func analysisGlobProductStateKey(state globProductState) string {
	var b strings.Builder
	if state.started {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	write := func(positions []int) {
		b.WriteByte('|')
		for _, position := range positions {
			b.WriteString(strconv.Itoa(position))
			b.WriteByte(',')
		}
	}
	write(state.left)
	write(state.right)
	for _, positions := range state.negative {
		write(positions)
	}
	return b.String()
}

func analysisGlobIntersectionAlphabet(patterns ...[]analysisGlobToken) []rune {
	preferred := []rune{'a', '_', '0', '-', '.', ':', 'é'}
	candidates := make(map[rune]struct{})
	add := func(value rune) {
		if value >= 0 && value <= 0x10ffff {
			candidates[value] = struct{}{}
		}
	}
	for _, value := range preferred {
		add(value)
	}
	add(0)
	add(0x10ffff)
	for _, pattern := range patterns {
		for _, token := range pattern {
			switch token.kind {
			case analysisGlobLiteral:
				add(token.literal)
			case analysisGlobClass:
				for _, item := range token.ranges {
					add(item.lo)
					add(item.hi)
					add(item.lo - 1)
					add(item.hi + 1)
				}
			}
		}
	}

	alphabet := make([]rune, 0, len(candidates))
	for _, value := range preferred {
		if _, ok := candidates[value]; ok {
			alphabet = append(alphabet, value)
			delete(candidates, value)
		}
	}
	alphabet = append(alphabet, slices.Sorted(maps.Keys(candidates))...)
	return alphabet
}

func analysisGlobEpsilonClosure(tokens []analysisGlobToken, position int) []int {
	positions := []int{position}
	for position < len(tokens) && tokens[position].kind == analysisGlobStar {
		position++
		positions = append(positions, position)
	}
	return positions
}

func analysisGlobNextPositions(tokens []analysisGlobToken, position int, candidate rune) []int {
	if position >= len(tokens) {
		return nil
	}
	token := tokens[position]
	if token.kind == analysisGlobStar {
		return analysisGlobEpsilonClosure(tokens, position)
	}
	if !analysisGlobTokenMatches(token, candidate) {
		return nil
	}
	return analysisGlobEpsilonClosure(tokens, position+1)
}

func analysisGlobTokenMatches(token analysisGlobToken, candidate rune) bool {
	switch token.kind {
	case analysisGlobLiteral:
		return token.literal == candidate
	case analysisGlobAny:
		return true
	case analysisGlobClass:
		matched := false
		for _, item := range token.ranges {
			if item.lo <= candidate && candidate <= item.hi {
				matched = true
				break
			}
		}
		return matched != token.negated
	default:
		return false
	}
}

func parseAnalysisGlob(pattern string) ([]analysisGlobToken, error) {
	if err := validateGlobPattern(pattern); err != nil {
		return nil, err
	}
	runes := []rune(pattern)
	var tokens []analysisGlobToken
	for index := 0; index < len(runes); index++ {
		switch runes[index] {
		case '\\':
			index++
			if index >= len(runes) {
				return nil, errBadGlobPattern
			}
			tokens = append(tokens, analysisGlobToken{kind: analysisGlobLiteral, literal: runes[index]})
		case '*':
			if len(tokens) == 0 || tokens[len(tokens)-1].kind != analysisGlobStar {
				tokens = append(tokens, analysisGlobToken{kind: analysisGlobStar})
			}
		case '?':
			tokens = append(tokens, analysisGlobToken{kind: analysisGlobAny})
		case '[':
			token, end, ok := parseAnalysisGlobClass(runes, index)
			if !ok {
				return nil, errBadGlobPattern
			}
			tokens = append(tokens, token)
			index = end
		default:
			tokens = append(tokens, analysisGlobToken{kind: analysisGlobLiteral, literal: runes[index]})
		}
	}
	return tokens, nil
}

func parseAnalysisGlobClass(pattern []rune, start int) (analysisGlobToken, int, bool) {
	token := analysisGlobToken{kind: analysisGlobClass}
	index := start + 1
	if index < len(pattern) && pattern[index] == '^' {
		token.negated = true
		index++
	}
	for index < len(pattern) && pattern[index] != ']' {
		lo, next, ok := parseAnalysisGlobClassRune(pattern, index)
		if !ok {
			return analysisGlobToken{}, 0, false
		}
		index = next
		hi := lo
		if index < len(pattern) && pattern[index] == '-' {
			hi, index, ok = parseAnalysisGlobClassRune(pattern, index+1)
			if !ok || hi < lo {
				return analysisGlobToken{}, 0, false
			}
		}
		token.ranges = append(token.ranges, analysisGlobRange{lo: lo, hi: hi})
	}
	if index >= len(pattern) || pattern[index] != ']' || len(token.ranges) == 0 {
		return analysisGlobToken{}, 0, false
	}
	return token, index, true
}

func parseAnalysisGlobClassRune(pattern []rune, index int) (rune, int, bool) {
	if index >= len(pattern) || pattern[index] == '-' || pattern[index] == ']' {
		return 0, 0, false
	}
	if pattern[index] == '\\' {
		index++
		if index >= len(pattern) {
			return 0, 0, false
		}
	}
	return pattern[index], index + 1, true
}
