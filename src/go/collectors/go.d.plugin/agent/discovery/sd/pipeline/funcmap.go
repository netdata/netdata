// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"regexp"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/bmatcuk/doublestar/v4"
)

func newFuncMap() template.FuncMap {
	custom := map[string]interface{}{
		"glob": globAny,
		"re":   regexpAny,
	}

	fm := sprig.HermeticTxtFuncMap()
	for name, fn := range custom {
		fm[name] = fn
	}

	return fm
}

func globAny(value, pattern string, rest ...string) bool {
	switch len(rest) {
	case 0:
		return globOnce(value, pattern)
	default:
		return globOnce(value, pattern) || globAny(value, rest[0], rest[1:]...)
	}
}

func regexpAny(value, pattern string, rest ...string) bool {
	switch len(rest) {
	case 0:
		return regexpOnce(value, pattern)
	default:
		return regexpOnce(value, pattern) || regexpAny(value, rest[0], rest[1:]...)
	}
}

func globOnce(value, pattern string) bool {
	ok, err := doublestar.Match(pattern, value)
	return err == nil && ok
}

func regexpOnce(value, pattern string) bool {
	ok, err := regexp.MatchString(pattern, value)
	return err == nil && ok
}
