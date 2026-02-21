// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

type (
	trueSelector  struct{}
	falseSelector struct{}
	negSelector   struct{ s Selector }
	andSelector   struct{ lhs, rhs Selector }
	orSelector    struct{ lhs, rhs Selector }
)

func (trueSelector) Matches(_ string, _ Labels) bool  { return true }
func (falseSelector) Matches(_ string, _ Labels) bool { return false }

func (s negSelector) Matches(metricName string, labels Labels) bool {
	return !s.s.Matches(metricName, labels)
}

func (s andSelector) Matches(metricName string, labels Labels) bool {
	return s.lhs.Matches(metricName, labels) && s.rhs.Matches(metricName, labels)
}

func (s orSelector) Matches(metricName string, labels Labels) bool {
	return s.lhs.Matches(metricName, labels) || s.rhs.Matches(metricName, labels)
}

// True returns a selector which always matches.
func True() Selector {
	return trueSelector{}
}

// And returns a selector that matches only when all sub-selectors match.
func And(lhs, rhs Selector, others ...Selector) Selector {
	s := andSelector{lhs: lhs, rhs: rhs}
	if len(others) == 0 {
		return s
	}
	return And(s, others[0], others[1:]...)
}

// Or returns a selector that matches when any sub-selector matches.
func Or(lhs, rhs Selector, others ...Selector) Selector {
	s := orSelector{lhs: lhs, rhs: rhs}
	if len(others) == 0 {
		return s
	}
	return Or(s, others[0], others[1:]...)
}

// Not returns a selector that negates the wrapped selector.
func Not(s Selector) Selector {
	return negSelector{s: s}
}
