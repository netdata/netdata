// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

type selector interface {
	matches(model.Tags) bool
}

type (
	exactSelector string
	trueSelector  struct{}
	negSelector   struct{ selector }
	orSelector    struct{ lhs, rhs selector }
	andSelector   struct{ lhs, rhs selector }
)

func (s exactSelector) matches(tags model.Tags) bool { _, ok := tags[string(s)]; return ok }
func (s trueSelector) matches(model.Tags) bool       { return true }
func (s negSelector) matches(tags model.Tags) bool   { return !s.selector.matches(tags) }
func (s orSelector) matches(tags model.Tags) bool    { return s.lhs.matches(tags) || s.rhs.matches(tags) }
func (s andSelector) matches(tags model.Tags) bool   { return s.lhs.matches(tags) && s.rhs.matches(tags) }

func (s exactSelector) String() string { return "{" + string(s) + "}" }
func (s negSelector) String() string   { return "{!" + stringify(s.selector) + "}" }
func (s trueSelector) String() string  { return "{*}" }
func (s orSelector) String() string    { return "{" + stringify(s.lhs) + "|" + stringify(s.rhs) + "}" }
func (s andSelector) String() string   { return "{" + stringify(s.lhs) + ", " + stringify(s.rhs) + "}" }
func stringify(sr selector) string     { return strings.Trim(fmt.Sprintf("%s", sr), "{}") }

func parseSelector(line string) (sr selector, err error) {
	words := strings.Fields(line)
	if len(words) == 0 {
		return trueSelector{}, nil
	}

	var srs []selector
	for _, word := range words {
		if idx := strings.IndexByte(word, '|'); idx > 0 {
			sr, err = parseOrSelectorWord(word)
		} else {
			sr, err = parseSingleSelectorWord(word)
		}
		if err != nil {
			return nil, fmt.Errorf("selector '%s' contains selector '%s' with forbidden symbol", line, word)
		}
		srs = append(srs, sr)
	}

	switch len(srs) {
	case 0:
		return trueSelector{}, nil
	case 1:
		return srs[0], nil
	default:
		return newAndSelector(srs[0], srs[1], srs[2:]...), nil
	}
}

func parseOrSelectorWord(orWord string) (sr selector, err error) {
	var srs []selector
	for _, word := range strings.Split(orWord, "|") {
		if sr, err = parseSingleSelectorWord(word); err != nil {
			return nil, err
		}
		srs = append(srs, sr)
	}
	switch len(srs) {
	case 0:
		return trueSelector{}, nil
	case 1:
		return srs[0], nil
	default:
		return newOrSelector(srs[0], srs[1], srs[2:]...), nil
	}
}

func parseSingleSelectorWord(word string) (selector, error) {
	if len(word) == 0 {
		return nil, errors.New("empty word")
	}
	neg := word[0] == '!'
	if neg {
		word = word[1:]
	}
	if len(word) == 0 {
		return nil, errors.New("empty word")
	}
	if word != "*" && !isSelectorWordValid(word) {
		return nil, errors.New("forbidden symbol")
	}

	var sr selector
	switch word {
	case "*":
		sr = trueSelector{}
	default:
		sr = exactSelector(word)
	}
	if neg {
		return negSelector{sr}, nil
	}
	return sr, nil
}

func newAndSelector(lhs, rhs selector, others ...selector) selector {
	m := andSelector{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newAndSelector(m, others[0], others[1:]...)
	}
}

func newOrSelector(lhs, rhs selector, others ...selector) selector {
	m := orSelector{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newOrSelector(m, others[0], others[1:]...)
	}
}

func isSelectorWordValid(word string) bool {
	// valid:
	// *
	// ^[a-zA-Z][a-zA-Z0-9=_.]*$
	if len(word) == 0 {
		return false
	}
	if word == "*" {
		return true
	}
	for i, b := range word {
		switch {
		case b >= 'a' && b <= 'z':
		case b >= 'A' && b <= 'Z':
		case b >= '0' && b <= '9' && i > 0:
		case (b == '=' || b == '_' || b == '.') && i > 0:
		default:
			return false
		}
	}
	return true
}
