// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type Vnodes interface {
	Lookup(key string) (*vnodes.VirtualNode, bool)
}

// FunctionRegistry is an alias to functions.Registry for backward compatibility.
type FunctionRegistry = functions.Registry
