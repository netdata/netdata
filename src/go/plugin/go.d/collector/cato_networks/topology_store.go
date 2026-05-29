// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type topologyStore struct {
	data atomic.Pointer[topology.Data]
}

func (s *topologyStore) Publish(data *topology.Data) {
	s.data.Store(data)
}

func (s *topologyStore) CurrentTopology() (*topology.Data, bool) {
	data := s.data.Load()
	if data == nil {
		return nil, false
	}
	return data, true
}
