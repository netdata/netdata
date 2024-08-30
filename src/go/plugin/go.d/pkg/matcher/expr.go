// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"errors"
	"fmt"
)

type (
	Expr interface {
		Parse() (Matcher, error)
	}

	// SimpleExpr is a simple expression to describe the condition:
	//     (includes[0].Match(v) || includes[1].Match(v) || ...) && !(excludes[0].Match(v) || excludes[1].Match(v) || ...)
	SimpleExpr struct {
		Includes []string `yaml:"includes,omitempty" json:"includes"`
		Excludes []string `yaml:"excludes,omitempty" json:"excludes"`
	}
)

var (
	ErrEmptyExpr = errors.New("empty expression")
)

// Empty returns true if both Includes and Excludes are empty. You can't
func (s *SimpleExpr) Empty() bool {
	return len(s.Includes) == 0 && len(s.Excludes) == 0
}

// Parse parses the given matchers in Includes and Excludes
func (s *SimpleExpr) Parse() (Matcher, error) {
	if len(s.Includes) == 0 && len(s.Excludes) == 0 {
		return nil, ErrEmptyExpr
	}
	var (
		includes = FALSE()
		excludes = FALSE()
	)
	if len(s.Includes) > 0 {
		for _, item := range s.Includes {
			m, err := Parse(item)
			if err != nil {
				return nil, fmt.Errorf("parse matcher %q error: %v", item, err)
			}
			includes = Or(includes, m)
		}
	} else {
		includes = TRUE()
	}

	for _, item := range s.Excludes {
		m, err := Parse(item)
		if err != nil {
			return nil, fmt.Errorf("parse matcher %q error: %v", item, err)
		}
		excludes = Or(excludes, m)
	}

	return And(includes, Not(excludes)), nil
}
