// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type FileLocker interface {
	Lock(name string) (bool, error)
	Unlock(name string)
	UnlockAll()
}

type FileStatus interface {
	Save(cfg confgroup.Config, state string)
	Remove(cfg confgroup.Config)
}

type FileStatusStore interface {
	Contains(cfg confgroup.Config, states ...string) bool
}

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
