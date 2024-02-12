// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/vnodes"
)

type FileLocker interface {
	Lock(name string) (bool, error)
	Unlock(name string) error
}

type Vnodes interface {
	Lookup(key string) (*vnodes.VirtualNode, bool)
}

type StatusSaver interface {
	Save(cfg confgroup.Config, state string)
	Remove(cfg confgroup.Config)
}

type StatusStore interface {
	Contains(cfg confgroup.Config, states ...string) bool
}

type Dyncfg interface {
	Register(cfg confgroup.Config)
	Unregister(cfg confgroup.Config)
	UpdateStatus(cfg confgroup.Config, status, payload string)
}
