// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"sync/atomic"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

type topologyStore struct {
	data atomic.Pointer[topologyv1.Data]
}

func (s *topologyStore) Publish(data *topologyv1.Data) {
	// Published topology snapshots are immutable; Function handlers load and
	// marshal the pointer concurrently with later collection cycles.
	s.data.Store(data)
}

func (s *topologyStore) CurrentTopology() (*topologyv1.Data, bool) {
	data := s.data.Load()
	if data == nil {
		return nil, false
	}
	return data, true
}
