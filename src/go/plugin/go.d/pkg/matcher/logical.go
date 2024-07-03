// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

type (
	trueMatcher  struct{}
	falseMatcher struct{}
	andMatcher   struct{ lhs, rhs Matcher }
	orMatcher    struct{ lhs, rhs Matcher }
	negMatcher   struct{ Matcher }
)

var (
	matcherT trueMatcher
	matcherF falseMatcher
)

// TRUE returns a matcher which always returns true
func TRUE() Matcher {
	return matcherT
}

// FALSE returns a matcher which always returns false
func FALSE() Matcher {
	return matcherF
}

// Not returns a matcher which positive the sub-matcher's result
func Not(m Matcher) Matcher {
	switch m {
	case TRUE():
		return FALSE()
	case FALSE():
		return TRUE()
	default:
		return negMatcher{m}
	}
}

// And returns a matcher which returns true only if all of it's sub-matcher return true
func And(lhs, rhs Matcher, others ...Matcher) Matcher {
	var matcher Matcher
	switch lhs {
	case TRUE():
		matcher = rhs
	case FALSE():
		matcher = FALSE()
	default:
		switch rhs {
		case TRUE():
			matcher = lhs
		case FALSE():
			matcher = FALSE()
		default:
			matcher = andMatcher{lhs, rhs}
		}
	}
	if len(others) > 0 {
		return And(matcher, others[0], others[1:]...)
	}
	return matcher
}

// Or returns a matcher which returns true if any of it's sub-matcher return true
func Or(lhs, rhs Matcher, others ...Matcher) Matcher {
	var matcher Matcher
	switch lhs {
	case TRUE():
		matcher = TRUE()
	case FALSE():
		matcher = rhs
	default:
		switch rhs {
		case TRUE():
			matcher = TRUE()
		case FALSE():
			matcher = lhs
		default:
			matcher = orMatcher{lhs, rhs}
		}
	}
	if len(others) > 0 {
		return Or(matcher, others[0], others[1:]...)
	}
	return matcher
}

func (trueMatcher) Match(_ []byte) bool       { return true }
func (trueMatcher) MatchString(_ string) bool { return true }

func (falseMatcher) Match(_ []byte) bool       { return false }
func (falseMatcher) MatchString(_ string) bool { return false }

func (m andMatcher) Match(b []byte) bool       { return m.lhs.Match(b) && m.rhs.Match(b) }
func (m andMatcher) MatchString(s string) bool { return m.lhs.MatchString(s) && m.rhs.MatchString(s) }

func (m orMatcher) Match(b []byte) bool       { return m.lhs.Match(b) || m.rhs.Match(b) }
func (m orMatcher) MatchString(s string) bool { return m.lhs.MatchString(s) || m.rhs.MatchString(s) }

func (m negMatcher) Match(b []byte) bool       { return !m.Matcher.Match(b) }
func (m negMatcher) MatchString(s string) bool { return !m.Matcher.MatchString(s) }
