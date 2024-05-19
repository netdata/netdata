// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"bytes"
	"strings"
)

type (
	// stringFullMatcher implements Matcher, it uses "==" to match.
	stringFullMatcher string

	// stringPartialMatcher implements Matcher, it uses strings.Contains to match.
	stringPartialMatcher string

	// stringPrefixMatcher implements Matcher, it uses strings.HasPrefix to match.
	stringPrefixMatcher string

	// stringSuffixMatcher implements Matcher, it uses strings.HasSuffix to match.
	stringSuffixMatcher string
)

// NewStringMatcher create a new matcher with string format
func NewStringMatcher(s string, startWith, endWith bool) (Matcher, error) {
	if startWith {
		if endWith {
			return stringFullMatcher(s), nil
		}
		return stringPrefixMatcher(s), nil
	}
	if endWith {
		return stringSuffixMatcher(s), nil
	}
	return stringPartialMatcher(s), nil
}

func (m stringFullMatcher) Match(b []byte) bool          { return string(m) == string(b) }
func (m stringFullMatcher) MatchString(line string) bool { return string(m) == line }

func (m stringPartialMatcher) Match(b []byte) bool          { return bytes.Contains(b, []byte(m)) }
func (m stringPartialMatcher) MatchString(line string) bool { return strings.Contains(line, string(m)) }

func (m stringPrefixMatcher) Match(b []byte) bool          { return bytes.HasPrefix(b, []byte(m)) }
func (m stringPrefixMatcher) MatchString(line string) bool { return strings.HasPrefix(line, string(m)) }

func (m stringSuffixMatcher) Match(b []byte) bool          { return bytes.HasSuffix(b, []byte(m)) }
func (m stringSuffixMatcher) MatchString(line string) bool { return strings.HasSuffix(line, string(m)) }
