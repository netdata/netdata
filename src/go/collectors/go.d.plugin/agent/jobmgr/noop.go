// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/vnodes"
)

type noop struct{}

func (n noop) Lock(string) (bool, error)                     { return true, nil }
func (n noop) Unlock(string) error                           { return nil }
func (n noop) Save(confgroup.Config, string)                 {}
func (n noop) Remove(confgroup.Config)                       {}
func (n noop) Contains(confgroup.Config, ...string) bool     { return false }
func (n noop) Lookup(string) (*vnodes.VirtualNode, bool)     { return nil, false }
func (n noop) Register(confgroup.Config)                     { return }
func (n noop) Unregister(confgroup.Config)                   { return }
func (n noop) UpdateStatus(confgroup.Config, string, string) { return }
