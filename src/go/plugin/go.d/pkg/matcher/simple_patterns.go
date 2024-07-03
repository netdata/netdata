// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
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

	for _, pattern := range strings.Fields(expr) {
		if err := ps.add(pattern); err != nil {
			return nil, err
		}
	}
	if len(ps) == 0 {
		return FALSE(), nil
	}
	return ps, nil
}

func (m *simplePatternsMatcher) add(term string) error {
	p := simplePatternTerm{}
	if term[0] == '!' {
		p.positive = false
		term = term[1:]
	} else {
		p.positive = true
	}
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
