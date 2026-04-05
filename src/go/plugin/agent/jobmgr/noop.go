// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type noop struct{}

func (n noop) Register(name string, fn func(functions.Function))                {}
func (n noop) Unregister(name string)                                           {}
func (n noop) RegisterPrefix(name, prefix string, reg func(functions.Function)) {}
func (n noop) UnregisterPrefix(name, prefix string)                             {}
