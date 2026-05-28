// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import "fmt"

// Expr is a selector expression with allow/deny selector lists.
type Expr struct {
	Allow []string `yaml:"allow,omitempty" json:"allow"`
	Deny  []string `yaml:"deny,omitempty" json:"deny"`
}

func (e Expr) Empty() bool {
	return len(e.Allow) == 0 && len(e.Deny) == 0
}

func (e Expr) Parse() (Selector, error) {
	if e.Empty() {
		return nil, nil
	}

	var (
		allow Selector = trueSelector{}
		deny  Selector = falseSelector{}
		err   error
	)

	if allow, err = parseSelectorList(e.Allow, trueSelector{}); err != nil {
		return nil, err
	}
	if deny, err = parseSelectorList(e.Deny, falseSelector{}); err != nil {
		return nil, err
	}
	return And(allow, Not(deny)), nil
}

func parseSelectorList(items []string, fallback Selector) (Selector, error) {
	srs := make([]Selector, 0, len(items))
	for _, item := range items {
		sr, err := Parse(item)
		if err != nil {
			return nil, fmt.Errorf("parse selector %q: %w", item, err)
		}
		if sr == nil {
			continue
		}
		srs = append(srs, sr)
	}
	switch len(srs) {
	case 0:
		return fallback, nil
	case 1:
		return srs[0], nil
	default:
		return Or(srs[0], srs[1], srs[2:]...), nil
	}
}
