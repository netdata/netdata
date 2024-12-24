// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"fmt"
)

func newCache() *cache {
	return &cache{
		accounts:    make(accCache),
		routes:      make(routeCache),
		inGateways:  make(gwCache),
		outGateways: make(gwCache),
		leafs:       make(leafCache),
	}
}

type cache struct {
	accounts    accCache
	routes      routeCache
	inGateways  gwCache
	outGateways gwCache
	leafs       leafCache
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
		hasCharts bool
		updated   bool

		accName string
	}
)

func (c *accCache) put(ai accountInfo) {
	acc, ok := (*c)[ai.Account]
	if !ok {
		acc = &accCacheEntry{accName: ai.Account}
		(*c)[ai.Account] = acc
	}
	acc.updated = true
}

type (
	routeCache      map[uint64]*routeCacheEntry
	routeCacheEntry struct {
		hasCharts bool
		updated   bool

		rid      uint64
		remoteId string
	}
)

func (c *routeCache) put(ri routeInfo) {
	route, ok := (*c)[ri.Rid]
	if !ok {
		route = &routeCacheEntry{rid: ri.Rid, remoteId: ri.RemoteID}
		(*c)[ri.Rid] = route
	}
	route.updated = true
}

type (
	gwCache      map[string]*gwCacheEntry
	gwCacheEntry struct {
		hasCharts bool
		updated   bool

		gwName  string
		rgwName string
		conns   map[uint64]*gwConnCacheEntry
	}
	gwConnCacheEntry struct {
		hasCharts bool
		updated   bool

		gwName  string
		rgwName string
		cid     uint64
	}
)

func (c *gwCache) put(gwName, rgwName string, rgi *remoteGatewayInfo) {
	gw, ok := (*c)[gwName]
	if !ok {
		gw = &gwCacheEntry{gwName: gwName, rgwName: rgwName, conns: make(map[uint64]*gwConnCacheEntry)}
		(*c)[gwName] = gw
	}
	gw.updated = true

	conn, ok := gw.conns[rgi.Connection.Cid]
	if !ok {
		conn = &gwConnCacheEntry{gwName: gwName, rgwName: rgwName, cid: rgi.Connection.Cid}
		gw.conns[rgi.Connection.Cid] = conn
	}
	conn.updated = true
}

type (
	leafCache      map[string]*leafCacheEntry
	leafCacheEntry struct {
		hasCharts bool
		updated   bool

		leafName string
		account  string
		ip       string
		port     int
	}
)

func (c *leafCache) put(li leafInfo) {
	key := fmt.Sprintf("%s_%s_%s_%d", li.Name, li.Account, li.IP, li.Port)
	leaf, ok := (*c)[key]
	if !ok {
		leaf = &leafCacheEntry{leafName: li.Name, account: li.Account, ip: li.IP, port: li.Port}
		(*c)[key] = leaf
	}
	leaf.updated = true
}
