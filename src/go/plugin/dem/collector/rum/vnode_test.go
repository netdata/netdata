// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
)

func TestConfiguredVnodeCanonicalIdentityAndMetadataRefresh(t *testing.T) {
	const guid = "a1111111-1111-4111-8111-111111111111"
	c, _ := measurementCollector(t)
	c.Vnode = "authored-reference"
	c.SetConfiguredVnode(vnodes.VirtualNode{
		GUID:     "A1111111111141118111111111111111",
		Hostname: "first",
	})
	assert.Equal(t, guid, c.aggregator.Activity().VnodeGUID)
	c.aggregator.Ingest(
		&beacon.Beacon{
			Site:         c.Name,
			ExperienceID: "document",
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		},
	)
	before := c.aggregator.Snapshot()
	c.SetConfiguredVnode(
		vnodes.VirtualNode{
			GUID:     guid,
			Hostname: "renamed",
			Labels:   map[string]string{"location": "new"},
		},
	)
	assert.Equal(t, before, c.aggregator.Snapshot())
	assert.Equal(t, "authored-reference", c.Configuration().(Config).Vnode)
	c.SetConfiguredVnode(vnodes.VirtualNode{})
	assert.Empty(t, c.aggregator.Activity().VnodeGUID)
	assert.Zero(t, c.aggregator.Snapshot().Counters["document_views"])
}

// Transport accounting is job-wide: a vnode change that resets measurements
// keeps it, and collection publishes it under the rum.history dimensions.
func TestTransportCountersSurviveVnodeChange(t *testing.T) {
	c, _ := measurementCollector(t)
	c.SetConfiguredVnode(vnodes.VirtualNode{
		GUID: "a1111111-1111-4111-8111-111111111111",
	})
	c.transport.Add(history.CounterWritten, 4)
	c.transport.Add(history.CounterDropped, 1)
	c.SetConfiguredVnode(vnodes.VirtualNode{
		GUID: "b2222222-2222-4222-8222-222222222222",
	})
	c.transport.Add(history.CounterWritten, 2)
	got := measurementValues(collectMeasurements(t, c), history.CounterWritten, history.CounterDropped)
	assert.Equal(t, map[string]float64{"history_written": 6, "history_dropped": 1}, got)
}
