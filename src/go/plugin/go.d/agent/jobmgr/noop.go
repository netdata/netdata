// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type noop struct{}

func (n noop) Lock(string) (bool, error)                                        { return true, nil }
func (n noop) Unlock(string)                                                    {}
func (n noop) UnlockAll()                                                       {}
func (n noop) Lookup(string) (*vnodes.VirtualNode, bool)                        { return nil, false }
func (n noop) RegisterPrefix(name, prefix string, reg func(functions.Function)) {}
func (n noop) UnregisterPrefix(name, prefix string)                             {}
