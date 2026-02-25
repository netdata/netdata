// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import "github.com/netdata/netdata/go/plugins/pkg/selectorcore"

// Parse parses one selector expression.
func Parse(expr string) (Selector, error) {
	sel, err := selectorcore.Parse(expr)
	if err != nil {
		return nil, err
	}
	return wrapCoreSelector(sel), nil
}
