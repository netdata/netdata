// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/topology/graph"

type builtAdjacencyLink struct {
	adj      Adjacency
	protocol string
	link     graph.Link
}

type pairedLinkAccumulator struct {
	all []*builtAdjacencyLink
}

type projectedLinks struct {
	links               []graph.Link
	lldp                int
	cdp                 int
	bidirectionalCount  int
	unidirectionalCount int
}

type bridgePortRef struct {
	deviceID   string
	ifIndex    int
	ifName     string
	bridgePort string
	vlanID     string
}
