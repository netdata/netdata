// SPDX-License-Identifier: GPL-3.0-or-later

package nats

func newCache() *cache {
	return &cache{
		accounts:    make(accCache),
		routes:      make(routeCache),
		inGateways:  make(gwCache),
		outGateways: make(gwCache),
	}
}

type cache struct {
	accounts    accCache
	routes      routeCache
	inGateways  gwCache
	outGateways gwCache
}

func (c *cache) resetUpdated() {
	for _, gw := range c.inGateways {
		gw.updated = false
		for _, conn := range gw.conns {
			conn.updated = false
		}
	}
	for _, gw := range c.outGateways {
		gw.updated = false
		for _, conn := range gw.conns {
			conn.updated = false
		}
	}
}

type (
	accCache      map[string]*accCacheEntry
	accCacheEntry struct {
		accName   string
		hasCharts bool
		updated   bool
	}
)

func (c *accCache) put(name string) {
	acc, ok := (*c)[name]
	if !ok {
		acc = &accCacheEntry{accName: name}
		(*c)[name] = acc
	}
	acc.updated = true
}

type (
	routeCache      map[uint64]*routeCacheEntry
	routeCacheEntry struct {
		rid       uint64
		remoteId  string
		hasCharts bool
		updated   bool
	}
)

func (c *routeCache) put(rid uint64, remoteId string) {
	route, ok := (*c)[rid]
	if !ok {
		route = &routeCacheEntry{rid: rid, remoteId: remoteId}
		(*c)[rid] = route
	}
	route.updated = true
}

type (
	gwCache      map[string]*gwCacheEntry
	gwCacheEntry struct {
		gwName    string
		rgwName   string
		hasCharts bool
		updated   bool
		conns     map[uint64]*gwConnCacheEntry
	}
	gwConnCacheEntry struct {
		gwName    string
		rgwName   string
		cid       uint64
		hasCharts bool
		updated   bool
	}
)

func (c *gwCache) put(gwName, rgwName string) {
	gw, ok := (*c)[gwName]
	if !ok {
		gw = &gwCacheEntry{gwName: gwName, rgwName: rgwName, conns: make(map[uint64]*gwConnCacheEntry)}
		(*c)[gwName] = gw
	}
	gw.updated = true
}

func (c *gwCache) putConn(gwName, rgwName string, cid uint64) {
	c.put(gwName, rgwName)
	conn, ok := (*c)[gwName].conns[cid]
	if !ok {
		conn = &gwConnCacheEntry{gwName: gwName, rgwName: rgwName, cid: cid}
		(*c)[gwName].conns[cid] = conn
	}
	conn.updated = true
}
