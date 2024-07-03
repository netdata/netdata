// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import "fmt"

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

	var srs []Selector
	var allow Selector
	var deny Selector

	for _, item := range e.Allow {
		sr, err := Parse(item)
		if err != nil {
			return nil, fmt.Errorf("parse selector '%s': %v", item, err)
		}
		srs = append(srs, sr)
	}

	switch len(srs) {
	case 0:
		allow = trueSelector{}
	case 1:
		allow = srs[0]
	default:
		allow = Or(srs[0], srs[1], srs[2:]...)
	}

	srs = srs[:0]
	for _, item := range e.Deny {
		sr, err := Parse(item)
		if err != nil {
			return nil, fmt.Errorf("parse selector '%s': %v", item, err)
		}
		srs = append(srs, sr)
	}

	switch len(srs) {
	case 0:
		deny = falseSelector{}
	case 1:
		deny = srs[0]
	default:
		deny = Or(srs[0], srs[1], srs[2:]...)
	}

	return And(allow, Not(deny)), nil
}
