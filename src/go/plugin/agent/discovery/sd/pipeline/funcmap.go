// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"regexp"
	"strconv"
	"text/template"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"

	"github.com/Masterminds/sprig/v3"
	"github.com/bmatcuk/doublestar/v4"
)

func newFuncMap() template.FuncMap {
	fm := sprig.TxtFuncMap()

	extra := map[string]any{
		"match": funcMatchAny,
		"glob": func(value, pattern string, patterns ...string) bool {
			return funcMatchAny("glob", value, pattern, patterns...)
		},
		"promPort": func(port string) string {
			v, _ := strconv.Atoi(port)
			return prometheusPortAllocations[v]
		},
	}

	for name, fn := range extra {
		fm[name] = fn
	}

	return fm
}

func funcMatchAny(typ, value, pattern string, patterns ...string) bool {
	switch len(patterns) {
	case 0:
		return funcMatch(typ, value, pattern)
	default:
		return funcMatch(typ, value, pattern) || funcMatchAny(typ, value, patterns[0], patterns[1:]...)
	}
}

func funcMatch(typ string, value, pattern string) bool {
	switch typ {
	case "glob", "":
		m, err := matcher.NewGlobMatcher(pattern)
		return err == nil && m.MatchString(value)
	case "sp":
		m, err := matcher.NewSimplePatternsMatcher(pattern)
		return err == nil && m.MatchString(value)
	case "re":
		ok, err := regexp.MatchString(pattern, value)
		return err == nil && ok
	case "dstar":
		ok, err := doublestar.Match(pattern, value)
		return err == nil && ok
	default:
		return false
	}
}
