// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

type builtAdjacencyLink struct {
	adj      Adjacency
	protocol string
	link     Link
}

type pairedLinkAccumulator struct {
	all []*builtAdjacencyLink
}

type projectedLinks struct {
	links               []Link
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
