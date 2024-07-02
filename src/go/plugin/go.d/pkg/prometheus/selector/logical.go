// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"github.com/prometheus/prometheus/model/labels"
)

type (
	trueSelector  struct{}
	falseSelector struct{}
	negSelector   struct{ s Selector }
	andSelector   struct{ lhs, rhs Selector }
	orSelector    struct{ lhs, rhs Selector }
)

func (trueSelector) Matches(_ labels.Labels) bool    { return true }
func (falseSelector) Matches(_ labels.Labels) bool   { return false }
func (s negSelector) Matches(lbs labels.Labels) bool { return !s.s.Matches(lbs) }
func (s andSelector) Matches(lbs labels.Labels) bool { return s.lhs.Matches(lbs) && s.rhs.Matches(lbs) }
func (s orSelector) Matches(lbs labels.Labels) bool  { return s.lhs.Matches(lbs) || s.rhs.Matches(lbs) }

// True returns a selector which always returns true
func True() Selector {
	return trueSelector{}
}

// And returns a selector which returns true only if all of it's sub-selectors return true
func And(lhs, rhs Selector, others ...Selector) Selector {
	s := andSelector{lhs: lhs, rhs: rhs}
	if len(others) == 0 {
		return s
	}
	return And(s, others[0], others[1:]...)
}

// Or returns a selector which returns true if any of it's sub-selectors return true
func Or(lhs, rhs Selector, others ...Selector) Selector {
	s := orSelector{lhs: lhs, rhs: rhs}
	if len(others) == 0 {
		return s
	}
	return Or(s, others[0], others[1:]...)
}

// Not returns a selector which returns the negation of the sub-selector's result
func Not(s Selector) Selector {
	return negSelector{s}
}
