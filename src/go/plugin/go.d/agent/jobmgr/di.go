// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type Vnodes interface {
	Lookup(key string) (*vnodes.VirtualNode, bool)
}

type FunctionRegistry interface {
	Register(name string, reg func(functions.Function))
	Unregister(name string)
}

type dyncfgAPI interface {
	CONFIGCREATE(opts netdataapi.ConfigOpts)
	CONFIGDELETE(id string)
	CONFIGSTATUS(id, status string)
	FUNCRESULT(result netdataapi.FunctionResult)
}
