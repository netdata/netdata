// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import "github.com/netdata/netdata/go/plugins/plugin/framework/functions"

// WrapHandler adapts a dyncfg function handler to functions.Registry handler type.
func WrapHandler(handler func(Function)) func(functions.Function) {
	return func(fn functions.Function) {
		handler(NewFunction(fn))
	}
}
