// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"errors"
	"fmt"
	"strings"
)

type (
	simplePatternTerm struct {
		matcher  Matcher
		positive bool
	}

	// simplePatternsMatcher patterns.
	simplePatternsMatcher []simplePatternTerm
)

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
