// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

func newCache() *cache {
	return &cache{
		nodes:  make(map[string]*nodeCacheItem),
		vhosts: make(map[string]*vhostCacheItem),
		queues: make(map[string]*queueCacheItem),
	}
}

type (
	cache struct {
		overview struct{ hasCharts bool }
		nodes    map[string]*nodeCacheItem
		vhosts   map[string]*vhostCacheItem
		queues   map[string]*queueCacheItem
	}
	nodeCacheItem struct {
		name      string
		seen      bool
		hasCharts bool
		peers     map[string]*peerCacheItem
	}
	peerCacheItem struct {
		name      string
		node      string
		seen      bool
		hasCharts bool
	}
	vhostCacheItem struct {
		name      string
		seen      bool
		hasCharts bool
	}
	queueCacheItem struct {
		name      string
		node      string
		vhost     string
		typ       string
		seen      bool
		hasCharts bool
	}
)

func (c *cache) resetSeen() {
	for _, v := range c.nodes {
		v.seen = false
		for _, v := range v.peers {
			v.seen = false
		}
	}
	for _, v := range c.vhosts {
		v.seen = false
	}
	for _, v := range c.queues {
		v.seen = false
	}
}

func (c *cache) getNode(node apiNodeResp) *nodeCacheItem {
	v, ok := c.nodes[node.Name]
	if !ok {
		v = &nodeCacheItem{name: node.Name, peers: make(map[string]*peerCacheItem)}
		c.nodes[node.Name] = v
	}
	return v
}

func (c *cache) getNodeClusterPeer(node apiNodeResp, peer apiClusterPeer) *peerCacheItem {
	n := c.getNode(node)
	v, ok := n.peers[peer.Name]
	if !ok {
		v = &peerCacheItem{node: node.Name, name: peer.Name}
		n.peers[peer.Name] = v
	}
	return v
}

func (c *cache) getQueue(q apiQueueResp) *queueCacheItem {
	key := q.Node + "_" + q.Vhost + "_" + q.Name
	v, ok := c.queues[key]
	if !ok {
		v = &queueCacheItem{node: q.Node, name: q.Name, vhost: q.Vhost, typ: q.Type}
		c.queues[key] = v
	}
	return v
}

func (c *cache) getVhost(vhost string) *vhostCacheItem {
	v, ok := c.vhosts[vhost]
	if !ok {
		v = &vhostCacheItem{name: vhost}
		c.vhosts[vhost] = v
	}
	return v
}
