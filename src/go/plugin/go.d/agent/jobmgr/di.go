// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type Vnodes interface {
	Lookup(key string) (*vnodes.VirtualNode, bool)
}

type FunctionRegistry interface {
	RegisterPrefix(name, prefix string, fn func(functions.Function))
	UnregisterPrefix(name string, prefix string)
}
