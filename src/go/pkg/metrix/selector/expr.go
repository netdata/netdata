// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import "github.com/netdata/netdata/go/plugins/pkg/selectorcore"

// Expr is a selector expression with allow/deny selector lists.
type Expr struct {
	Allow []string `yaml:"allow,omitempty" json:"allow"`
	Deny  []string `yaml:"deny,omitempty" json:"deny"`
}

func (e Expr) Empty() bool {
	return len(e.Allow) == 0 && len(e.Deny) == 0
}

func (e Expr) Parse() (Selector, error) {
	sel, err := selectorcore.Expr{
		Allow: append([]string(nil), e.Allow...),
		Deny:  append([]string(nil), e.Deny...),
	}.Parse()
	if err != nil {
		return nil, err
	}
	return wrapCoreSelector(sel), nil
}
