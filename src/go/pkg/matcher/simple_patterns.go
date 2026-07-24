// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type (
	simplePatternTerm struct {
		matcher  Matcher
		positive bool
	}

	// simplePatternsMatcher patterns.
	simplePatternsMatcher []simplePatternTerm

	// PositivePatternList is an immutable, canonical list of positive glob
	// patterns compiled for OR matching.
	PositivePatternList struct {
		patterns []string
		matchers []Matcher
	}
)

// CompilePositivePatternList validates, canonicalizes, and compiles positive
// glob patterns. Empty input is valid and matches nothing.
func CompilePositivePatternList(patterns []string) (PositivePatternList, error) {
	if len(patterns) == 0 {
		return PositivePatternList{}, nil
	}

	unique := make(map[string]struct{}, len(patterns))
	for i, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		switch {
		case pattern == "":
			return PositivePatternList{}, fmt.Errorf("pattern[%d]: must not be empty", i)
		case strings.HasPrefix(pattern, "!"):
			return PositivePatternList{}, fmt.Errorf("pattern[%d]: negative patterns are not allowed", i)
		}
		if _, err := NewGlobMatcher(pattern); err != nil {
			return PositivePatternList{}, fmt.Errorf("pattern[%d]: invalid pattern %q: %w", i, pattern, err)
		}
		unique[pattern] = struct{}{}
	}

	if _, ok := unique["*"]; ok {
		return PositivePatternList{
			patterns: []string{"*"},
			matchers: []Matcher{TRUE()},
		}, nil
	}

	canonical := make([]string, 0, len(unique))
	for pattern := range unique {
		canonical = append(canonical, pattern)
	}
	sort.Strings(canonical)

	compiled := PositivePatternList{
		patterns: canonical,
		matchers: make([]Matcher, 0, len(canonical)),
	}
	for _, pattern := range canonical {
		m, err := NewGlobMatcher(pattern)
		if err != nil {
			return PositivePatternList{}, fmt.Errorf("compile canonical pattern %q: %w", pattern, err)
		}
		compiled.matchers = append(compiled.matchers, m)
	}
	return compiled, nil
}

// Patterns returns a copy of the canonical patterns.
func (m PositivePatternList) Patterns() []string {
	return slices.Clone(m.patterns)
}

// Match matches when any positive pattern matches.
func (m PositivePatternList) Match(b []byte) bool {
	return m.MatchString(string(b))
}

// MatchString matches when any positive pattern matches.
func (m PositivePatternList) MatchString(value string) bool {
	for _, pattern := range m.matchers {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

// NewSimplePatternsMatcher creates new simple patterns. It returns error in case one of patterns has bad syntax.
func NewSimplePatternsMatcher(expr string) (Matcher, error) {
	ps := simplePatternsMatcher{}

	for pattern := range strings.FieldsSeq(expr) {
		positive := true
		if strings.HasPrefix(pattern, "!") {
			positive = false
			pattern = strings.TrimPrefix(pattern, "!")
		}
		if err := ps.add(pattern, positive); err != nil {
			return nil, err
		}
	}
	if len(ps) == 0 {
		return FALSE(), nil
	}
	return ps, nil
}

// NewSimplePatternListMatcher creates a simple-patterns matcher from a pre-split
// list of glob patterns. Use it when individual patterns may contain whitespace.
func NewSimplePatternListMatcher(patterns []string) (Matcher, error) {
	ps := simplePatternsMatcher{}
	hasPositive := false

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		negative := strings.HasPrefix(pattern, "!")
		if negative {
			pattern = strings.TrimSpace(strings.TrimPrefix(pattern, "!"))
		}
		if pattern == "" {
			return nil, errors.New("invalid empty negative pattern")
		}
		hasPositive = hasPositive || !negative
		if err := ps.add(pattern, !negative); err != nil {
			return nil, fmt.Errorf("invalid pattern: %w", err)
		}
	}
	if len(ps) == 0 {
		return FALSE(), nil
	}
	if !hasPositive {
		return nil, errors.New("must include at least one positive pattern")
	}
	return ps, nil
}

func (m *simplePatternsMatcher) add(term string, positive bool) error {
	p := simplePatternTerm{positive: positive}
	matcher, err := NewGlobMatcher(term)
	if err != nil {
		return err
	}

	p.matcher = matcher
	*m = append(*m, p)

	return nil
}

func (m simplePatternsMatcher) Match(b []byte) bool {
	return m.MatchString(string(b))
}

// MatchString matches.
func (m simplePatternsMatcher) MatchString(line string) bool {
	for _, p := range m {
		if p.matcher.MatchString(line) {
			return p.positive
		}
	}
	return false
}
