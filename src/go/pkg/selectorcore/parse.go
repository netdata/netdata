// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

var (
	reLV = regexp.MustCompile(`^(?P<label_name>[a-zA-Z0-9_]+)(?P<op>=~|!~|=\*|!\*|=|!=)"(?P<pattern>.+)"$`)
)

// Parse parses one selector expression.
func Parse(expr string) (Selector, error) {
	terms, err := splitSelectorTerms(unsugarExpr(expr))
	if err != nil {
		return nil, err
	}

	srs := make([]Selector, 0, len(terms))
	for _, term := range terms {
		sr, err := parseSelector(term)
		if err != nil {
			return nil, err
		}
		srs = append(srs, sr)
	}

	switch len(srs) {
	case 0:
		return nil, nil
	case 1:
		return srs[0], nil
	default:
		return And(srs[0], srs[1], srs[2:]...), nil
	}
}

func parseSelector(line string) (Selector, error) {
	sub := reLV.FindStringSubmatch(strings.TrimSpace(line))
	if sub == nil {
		return nil, fmt.Errorf("invalid selector syntax: '%s'", line)
	}

	name, op, pattern := sub[1], sub[2], strings.Trim(sub[3], "\"")

	var m matcher.Matcher
	var err error

	switch op {
	case OpEqual, OpNegEqual:
		m, err = matcher.NewStringMatcher(pattern, true, true)
	case OpRegexp, OpNegRegexp:
		m, err = matcher.NewRegExpMatcher(pattern)
	case OpSimplePatterns, OpNegSimplePatterns:
		m, err = matcher.NewSimplePatternsMatcher(pattern)
	default:
		err = fmt.Errorf("unknown matching operator: %s", op)
	}
	if err != nil {
		return nil, err
	}

	sr := labelSelector{name: name, m: m}
	if strings.HasPrefix(op, "!") {
		return Not(sr), nil
	}
	return sr, nil
}

func unsugarExpr(expr string) string {
	// name                => __name__=*"name"
	// name{label="value"} => __name__=*"name",label="value"
	// {label="value"}     => label="value"
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}

	switch idx := strings.IndexByte(expr, '{'); {
	case idx == -1:
		expr = fmt.Sprintf(`%s%s"%s"`,
			MetricNameLabel,
			OpSimplePatterns,
			strings.TrimSpace(expr),
		)
	case idx == 0:
		expr = strings.Trim(expr, "{}")
	default:
		expr = fmt.Sprintf(`%s%s"%s",%s`,
			MetricNameLabel,
			OpSimplePatterns,
			strings.TrimSpace(expr[:idx]),
			strings.Trim(expr[idx:], "{}"),
		)
	}
	return expr
}

func splitSelectorTerms(expr string) ([]string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}

	var (
		terms         []string
		buf           strings.Builder
		inQuotes      bool
		escapePending bool
		seenComma     bool
	)

	for _, r := range expr {
		switch {
		case escapePending:
			escapePending = false
			buf.WriteRune(r)
		case r == '\\':
			escapePending = true
			buf.WriteRune(r)
		case r == '"':
			inQuotes = !inQuotes
			buf.WriteRune(r)
		case r == ',' && !inQuotes:
			seenComma = true
			part := strings.TrimSpace(buf.String())
			if part == "" {
				return nil, fmt.Errorf("invalid selector syntax: empty matcher around comma")
			}
			terms = append(terms, part)
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}

	if inQuotes {
		return nil, fmt.Errorf("invalid selector syntax: unterminated quote")
	}

	last := strings.TrimSpace(buf.String())
	if last == "" {
		if seenComma {
			return nil, fmt.Errorf("invalid selector syntax: trailing comma")
		}
		return nil, nil
	}
	terms = append(terms, last)
	return terms, nil
}
