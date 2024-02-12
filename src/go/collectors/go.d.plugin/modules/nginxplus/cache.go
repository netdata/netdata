// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

func newCache() *cache {
	return &cache{
		httpCaches:            make(map[string]*cacheHTTPCacheEntry),
		httpServerZones:       make(map[string]*cacheZoneEntry),
		httpLocationZones:     make(map[string]*cacheZoneEntry),
		httpUpstreams:         make(map[string]*cacheUpstreamEntry),
		httpUpstreamServers:   make(map[string]*cacheUpstreamServerEntry),
		streamServerZones:     make(map[string]*cacheZoneEntry),
		streamUpstreams:       make(map[string]*cacheUpstreamEntry),
		streamUpstreamServers: make(map[string]*cacheUpstreamServerEntry),
		resolvers:             make(map[string]*cacheResolverEntry),
	}
}

type (
	cache struct {
		httpCaches            map[string]*cacheHTTPCacheEntry
		httpServerZones       map[string]*cacheZoneEntry
		httpLocationZones     map[string]*cacheZoneEntry
		httpUpstreams         map[string]*cacheUpstreamEntry
		httpUpstreamServers   map[string]*cacheUpstreamServerEntry
		streamServerZones     map[string]*cacheZoneEntry
		streamUpstreams       map[string]*cacheUpstreamEntry
		streamUpstreamServers map[string]*cacheUpstreamServerEntry
		resolvers             map[string]*cacheResolverEntry
	}
	cacheEntry struct {
		hasCharts    bool
		updated      bool
		notSeenTimes int
	}
	cacheHTTPCacheEntry struct {
		name string
		cacheEntry
	}
	cacheResolverEntry struct {
		zone string
		cacheEntry
	}
	cacheZoneEntry struct {
		zone string
		cacheEntry
	}
	cacheUpstreamEntry struct {
		name string
		zone string
		cacheEntry
	}
	cacheUpstreamServerEntry struct {
		name       string
		zone       string
		serverAddr string
		serverName string
		cacheEntry
	}
)

func (c *cache) resetUpdated() {
	for _, v := range c.httpCaches {
		v.updated = false
	}
	for _, v := range c.httpServerZones {
		v.updated = false
	}
	for _, v := range c.httpLocationZones {
		v.updated = false
	}
	for _, v := range c.httpUpstreams {
		v.updated = false
	}
	for _, v := range c.httpUpstreamServers {
		v.updated = false
	}
	for _, v := range c.streamServerZones {
		v.updated = false
	}
	for _, v := range c.streamUpstreams {
		v.updated = false
	}
	for _, v := range c.streamUpstreamServers {
		v.updated = false
	}
	for _, v := range c.resolvers {
		v.updated = false
	}
}

func (c *cache) putHTTPCache(cache string) {
	v, ok := c.httpCaches[cache]
	if !ok {
		v = &cacheHTTPCacheEntry{name: cache}
		c.httpCaches[cache] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putHTTPServerZone(zone string) {
	v, ok := c.httpServerZones[zone]
	if !ok {
		v = &cacheZoneEntry{zone: zone}
		c.httpServerZones[zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putHTTPLocationZone(zone string) {
	v, ok := c.httpLocationZones[zone]
	if !ok {
		v = &cacheZoneEntry{zone: zone}
		c.httpLocationZones[zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putHTTPUpstream(name, zone string) {
	v, ok := c.httpUpstreams[name+"_"+zone]
	if !ok {
		v = &cacheUpstreamEntry{name: name, zone: zone}
		c.httpUpstreams[name+"_"+zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putHTTPUpstreamServer(name, serverAddr, serverName, zone string) {
	v, ok := c.httpUpstreamServers[name+"_"+serverAddr+"_"+zone]
	if !ok {
		v = &cacheUpstreamServerEntry{name: name, zone: zone, serverAddr: serverAddr, serverName: serverName}
		c.httpUpstreamServers[name+"_"+serverAddr+"_"+zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putStreamServerZone(zone string) {
	v, ok := c.streamServerZones[zone]
	if !ok {
		v = &cacheZoneEntry{zone: zone}
		c.streamServerZones[zone] = v
	}
	v.updated, v.notSeenTimes = true, 0

}

func (c *cache) putStreamUpstream(name, zone string) {
	v, ok := c.streamUpstreams[name+"_"+zone]
	if !ok {
		v = &cacheUpstreamEntry{name: name, zone: zone}
		c.streamUpstreams[name+"_"+zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putStreamUpstreamServer(name, serverAddr, serverName, zone string) {
	v, ok := c.streamUpstreamServers[name+"_"+serverAddr+"_"+zone]
	if !ok {
		v = &cacheUpstreamServerEntry{name: name, zone: zone, serverAddr: serverAddr, serverName: serverName}
		c.streamUpstreamServers[name+"_"+serverAddr+"_"+zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}

func (c *cache) putResolver(zone string) {
	v, ok := c.resolvers[zone]
	if !ok {
		v = &cacheResolverEntry{zone: zone}
		c.resolvers[zone] = v
	}
	v.updated, v.notSeenTimes = true, 0
}
