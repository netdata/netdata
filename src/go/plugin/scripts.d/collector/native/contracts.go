// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type retainedContract struct {
	definition metricContract
	lastCommit uint64
}

// Reconcile only at collection boundaries: JobV2 commits after Collect returns.
// A failed commit establishes nothing. The clock and eviction window are the
// store's, including descriptor grace; this is not a lifetime type registry.
func (c *Collector) reconcileContracts() {
	retention, ok := c.store.(metrix.DescriptorRetention)
	if !ok {
		// Without an acknowledgement clock, retain staged contracts conservatively.
		c.acceptContracts(0)
		return
	}
	success := retention.SuccessfulCommits()
	if success > c.contractCommit {
		c.acceptContracts(success)
	}
	c.pendingContracts = nil
	c.contractCommit = success
	window := retention.DescriptorRetentionWindow()
	if window == metrix.DescriptorRetentionUnbounded {
		return
	}
	for name, contract := range c.contracts {
		if success >= contract.lastCommit && success-contract.lastCommit >= window {
			delete(c.contracts, name)
		}
	}
}

func (c *Collector) acceptContracts(commit uint64) {
	if len(c.pendingContracts) > 0 && c.contracts == nil {
		c.contracts = make(map[string]retainedContract)
	}
	for name, definition := range c.pendingContracts {
		c.contracts[name] = retainedContract{
			definition: definition,
			lastCommit: commit,
		}
	}
	c.pendingContracts = nil
}

func (c *Collector) validateContracts(families []metricFamily) error {
	for _, family := range families {
		if previous, ok := c.contracts[family.Name]; ok && !previous.definition.equal(family.metricContract) {
			return errors.New("metric contract changed; use a new metric name for different type or meaning")
		}
	}
	return nil
}

func (a metricContract) equal(b metricContract) bool {
	return a.Type == b.Type && a.Unit == b.Unit && a.Mode == b.Mode && slices.Equal(a.States, b.States) &&
		a.ChartMeta.Title == b.ChartMeta.Title && a.ChartMeta.Family == b.ChartMeta.Family &&
		(a.ChartMeta.Priority == b.ChartMeta.Priority ||
			(a.ChartMeta.Priority != nil && b.ChartMeta.Priority != nil && *a.ChartMeta.Priority == *b.ChartMeta.Priority))
}

func (c *Collector) stageContracts(families []metricFamily) {
	for _, family := range families {
		// Declaring an empty family neither registers nor refreshes a descriptor.
		if len(family.Samples) == 0 {
			continue
		}
		if c.pendingContracts == nil {
			c.pendingContracts = make(map[string]metricContract)
		}
		c.pendingContracts[family.Name] = family.metricContract
	}
}
