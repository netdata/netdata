// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "sync/atomic"

type topologyFunctionAvailability struct {
	ready atomic.Bool
}

func newTopologyFunctionAvailability() *topologyFunctionAvailability {
	return &topologyFunctionAvailability{}
}

func (a *topologyFunctionAvailability) Available() bool {
	return a != nil && a.ready.Load()
}

func (a *topologyFunctionAvailability) Reset() {
	if a != nil {
		a.ready.Store(false)
	}
}

func (a *topologyFunctionAvailability) MarkAvailable() {
	if a != nil {
		a.ready.Store(true)
	}
}
