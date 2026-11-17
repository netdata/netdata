// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import "sync"

// transportCounters accumulates the job's history-writer and OTLP-exporter
// accounting. It describes the job's asynchronous transports, not measured
// traffic, so a vnode change that resets measurements leaves it intact. The
// history writer calls Add under the aggregator lock, so mu must stay a leaf lock.
type transportCounters struct {
	mu     sync.Mutex
	values map[string]uint64
}

func newTransportCounters() *transportCounters {
	return &transportCounters{
		values: map[string]uint64{},
	}
}

func (c *transportCounters) Add(counter string, n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[counter] += n
}

func (c *transportCounters) get(counter string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[counter]
}
