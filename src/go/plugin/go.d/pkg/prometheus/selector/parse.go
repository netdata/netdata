// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

var (
	reLV = regexp.MustCompile(`^(?P<label_name>[a-zA-Z0-9_]+)(?P<op>=~|!~|=\*|!\*|=|!=)"(?P<pattern>.+)"$`)
)

func Parse(expr string) (Selector, error) {
	var srs []Selector
	lvs := strings.Split(unsugarExpr(expr), ",")

	for _, lv := range lvs {
		sr, err := parseSelector(lv)
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

	sr := labelSelector{
		name: name,
		m:    m,
	}

	if neg := strings.HasPrefix(op, "!"); neg {
		return Not(sr), nil
	}
	return sr, nil
}

func unsugarExpr(expr string) string {
	// name                => __name__=*"name"
	// name{label="value"} => __name__=*"name",label="value"
	// {label="value"}     => label="value"
	expr = strings.TrimSpace(expr)

	switch idx := strings.IndexByte(expr, '{'); true {
	case idx == -1:
		expr = fmt.Sprintf(`__name__%s"%s"`,
			OpSimplePatterns,
			strings.TrimSpace(expr),
		)
	case idx == 0:
		expr = strings.Trim(expr, "{}")
	default:
		expr = fmt.Sprintf(`__name__%s"%s",%s`,
			OpSimplePatterns,
			strings.TrimSpace(expr[:idx]),
			strings.Trim(expr[idx:], "{}"),
		)
	}
	return expr
}
