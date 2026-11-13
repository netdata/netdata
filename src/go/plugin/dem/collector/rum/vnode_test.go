// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfiguredVnodeCanonicalIdentityAndMetadataRefresh(t *testing.T) {
	const guid = "a1111111-1111-4111-8111-111111111111"
	c, _ := measurementCollector(t)
	c.Vnode = "authored-reference"
	c.SetConfiguredVnode(vnodes.VirtualNode{GUID: "A1111111111141118111111111111111", Hostname: "first"})
	assert.Equal(t, guid, c.aggregator.Activity().VnodeGUID)
	c.aggregator.Ingest(&beacon.Beacon{Site: c.Name, ExperienceID: "document", Events: []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}}})
	before := c.aggregator.Snapshot()
	c.SetConfiguredVnode(vnodes.VirtualNode{GUID: guid, Hostname: "renamed", Labels: map[string]string{"location": "new"}})
	assert.Equal(t, before, c.aggregator.Snapshot())
	assert.Equal(t, "authored-reference", c.Configuration().(Config).Vnode)
	c.SetConfiguredVnode(vnodes.VirtualNode{})
	assert.Empty(t, c.aggregator.Activity().VnodeGUID)
	assert.Zero(t, c.aggregator.Snapshot().Counters["document_views"])
}
