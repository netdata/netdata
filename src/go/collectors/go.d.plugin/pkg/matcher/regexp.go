// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import "regexp"

// NewRegExpMatcher create new matcher with RegExp format
func NewRegExpMatcher(expr string) (Matcher, error) {
	switch expr {
	case "", "^", "$":
		return TRUE(), nil
	case "^$", "$^":
		return NewStringMatcher("", true, true)
	}
	size := len(expr)
	chars := []rune(expr)
	var startWith, endWith bool
	startIdx := 0
	endIdx := size - 1
	if chars[startIdx] == '^' {
		startWith = true
		startIdx = 1
	}
	if chars[endIdx] == '$' {
		endWith = true
		endIdx--
	}

	unescapedExpr := make([]rune, 0, endIdx-startIdx+1)
	for i := startIdx; i <= endIdx; i++ {
		ch := chars[i]
		if ch == '\\' {
			if i == endIdx { // end with '\' => invalid format
				return regexp.Compile(expr)
			}
			nextCh := chars[i+1]
			if !isRegExpMeta(nextCh) { // '\' + mon-meta char => special meaning
				return regexp.Compile(expr)
			}
			unescapedExpr = append(unescapedExpr, nextCh)
			i++
		} else if isRegExpMeta(ch) {
			return regexp.Compile(expr)
		} else {
			unescapedExpr = append(unescapedExpr, ch)
		}
	}

	return NewStringMatcher(string(unescapedExpr), startWith, endWith)
}

// isRegExpMeta reports whether byte b needs to be escaped by QuoteMeta.
func isRegExpMeta(b rune) bool {
	switch b {
	case '\\', '.', '+', '*', '?', '(', ')', '|', '[', ']', '{', '}', '^', '$':
		return true
	default:
		return false
	}
}
