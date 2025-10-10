// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	urlPathAPIVersions          = "/api/"
	urlPathAPIEndpointsRoot     = "/api/%d"
	urlPathAPINginx             = "/api/%d/nginx"
	urlPathAPIEndpointsHTTP     = "/api/%d/http"
	urlPathAPIEndpointsStream   = "/api/%d/stream"
	urlPathAPIConnections       = "/api/%d/connections"
	urlPathAPISSL               = "/api/%d/ssl"
	urlPathAPIResolvers         = "/api/%d/resolvers"
	urlPathAPIHTTPRequests      = "/api/%d/http/requests"
	urlPathAPIHTTPServerZones   = "/api/%d/http/server_zones"
	urlPathAPIHTTPLocationZones = "/api/%d/http/location_zones"
	urlPathAPIHTTPUpstreams     = "/api/%d/http/upstreams"
	urlPathAPIHTTPCaches        = "/api/%d/http/caches"
	urlPathAPIStreamServerZones = "/api/%d/stream/server_zones"
	urlPathAPIStreamUpstreams   = "/api/%d/stream/upstreams"
)

type nginxMetrics struct {
	info              *nginxInfo
	connections       *nginxConnections
	ssl               *nginxSSL
	httpRequests      *nginxHTTPRequests
	httpServerZones   *nginxHTTPServerZones
	httpLocationZones *nginxHTTPLocationZones
	httpUpstreams     *nginxHTTPUpstreams
	httpCaches        *nginxHTTPCaches
	streamServerZones *nginxStreamServerZones
	streamUpstreams   *nginxStreamUpstreams
	resolvers         *nginxResolvers
}

func (c *Collector) queryAPIVersion() (int64, error) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIVersions)

	var versions nginxAPIVersions
	if err := c.doHTTP(req, &versions); err != nil {
		return 0, err
	}

	if len(versions) == 0 {
		return 0, fmt.Errorf("'%s' returned no data", req.URL)
	}

	return versions[len(versions)-1], nil
}

func (c *Collector) queryAvailableEndpoints() error {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsRoot, c.apiVersion))

	var endpoints []string
	if err := c.doHTTP(req, &endpoints); err != nil {
		return err
	}

	c.Debugf("discovered root endpoints: %v", endpoints)
	var hasHTTP, hasStream bool
	for _, v := range endpoints {
		switch v {
		case "nginx":
			c.endpoints.nginx = true
		case "connections":
			c.endpoints.connections = true
		case "ssl":
			c.endpoints.ssl = true
		case "resolvers":
			c.endpoints.resolvers = true
		case "http":
			hasHTTP = true
		case "stream":
			hasStream = true
		}
	}

	if hasHTTP {
		endpoints = endpoints[:0]
		req, _ = web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsHTTP, c.apiVersion))

		if err := c.doHTTP(req, &endpoints); err != nil {
			return err
		}

		c.Debugf("discovered http endpoints: %v", endpoints)
		for _, v := range endpoints {
			switch v {
			case "requests":
				c.endpoints.httpRequest = true
			case "server_zones":
				c.endpoints.httpServerZones = true
			case "location_zones":
				c.endpoints.httpLocationZones = true
			case "caches":
				c.endpoints.httpCaches = true
			case "upstreams":
				c.endpoints.httpUpstreams = true
			}
		}
	}

	if hasStream {
		endpoints = endpoints[:0]
		req, _ = web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsStream, c.apiVersion))

		if err := c.doHTTP(req, &endpoints); err != nil {
			return err
		}

		c.Debugf("discovered stream endpoints: %v", endpoints)
		for _, v := range endpoints {
			switch v {
			case "server_zones":
				c.endpoints.streamServerZones = true
			case "upstreams":
				c.endpoints.streamUpstreams = true
			}
		}
	}

	return nil
}

func (c *Collector) queryMetrics() *nginxMetrics {
	ms := &nginxMetrics{}
	wg := &sync.WaitGroup{}

	for _, task := range []struct {
		do bool
		fn func(*nginxMetrics)
	}{
		{do: c.endpoints.nginx, fn: c.queryNginxInfo},
		{do: c.endpoints.connections, fn: c.queryConnections},
		{do: c.endpoints.ssl, fn: c.querySSL},
		{do: c.endpoints.httpRequest, fn: c.queryHTTPRequests},
		{do: c.endpoints.httpServerZones, fn: c.queryHTTPServerZones},
		{do: c.endpoints.httpLocationZones, fn: c.queryHTTPLocationZones},
		{do: c.endpoints.httpUpstreams, fn: c.queryHTTPUpstreams},
		{do: c.endpoints.httpCaches, fn: c.queryHTTPCaches},
		{do: c.endpoints.streamServerZones, fn: c.queryStreamServerZones},
		{do: c.endpoints.streamUpstreams, fn: c.queryStreamUpstreams},
		{do: c.endpoints.resolvers, fn: c.queryResolvers},
	} {
		task := task
		if task.do {
			wg.Add(1)
			go func() { task.fn(ms); wg.Done() }()
		}
	}

	wg.Wait()

	return ms
}

func (c *Collector) queryNginxInfo(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPINginx, c.apiVersion))

	var v nginxInfo

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.nginx = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.info = &v
}

func (c *Collector) queryConnections(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIConnections, c.apiVersion))

	var v nginxConnections

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.connections = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.connections = &v
}

func (c *Collector) querySSL(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPISSL, c.apiVersion))

	var v nginxSSL

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.ssl = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.ssl = &v
}

func (c *Collector) queryHTTPRequests(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIHTTPRequests, c.apiVersion))

	var v nginxHTTPRequests

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.httpRequest = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.httpRequests = &v
}

func (c *Collector) queryHTTPServerZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIHTTPServerZones, c.apiVersion))

	var v nginxHTTPServerZones

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.httpServerZones = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.httpServerZones = &v
}

func (c *Collector) queryHTTPLocationZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIHTTPLocationZones, c.apiVersion))

	var v nginxHTTPLocationZones

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.httpLocationZones = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.httpLocationZones = &v
}

func (c *Collector) queryHTTPUpstreams(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIHTTPUpstreams, c.apiVersion))

	var v nginxHTTPUpstreams

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.httpUpstreams = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.httpUpstreams = &v
}

func (c *Collector) queryHTTPCaches(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIHTTPCaches, c.apiVersion))

	var v nginxHTTPCaches

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.httpCaches = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.httpCaches = &v
}

func (c *Collector) queryStreamServerZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIStreamServerZones, c.apiVersion))

	var v nginxStreamServerZones

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.streamServerZones = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.streamServerZones = &v
}

func (c *Collector) queryStreamUpstreams(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIStreamUpstreams, c.apiVersion))

	var v nginxStreamUpstreams

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.streamUpstreams = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.streamUpstreams = &v
}

func (c *Collector) queryResolvers(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathAPIResolvers, c.apiVersion))

	var v nginxResolvers

	if err := c.doHTTP(req, &v); err != nil {
		c.endpoints.resolvers = !errors.Is(err, errPathNotFound)
		c.Warning(err)
		return
	}

	ms.resolvers = &v
}

var (
	errPathNotFound = errors.New("path not found")
)

func (c *Collector) doHTTP(req *http.Request, dst any) error {
	c.Debugf("executing %s '%s'", req.Method, req.URL)

	cl := web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		if resp.StatusCode == http.StatusNotFound {
			return false, errPathNotFound
		}
		return false, nil
	})

	return cl.RequestJSON(req, dst)
}

func (n *nginxMetrics) empty() bool {
	return n.info != nil &&
		n.connections == nil &&
		n.ssl == nil &&
		n.httpRequests == nil &&
		n.httpServerZones == nil &&
		n.httpLocationZones == nil &&
		n.httpUpstreams == nil &&
		n.httpCaches == nil &&
		n.streamServerZones == nil &&
		n.streamUpstreams == nil &&
		n.resolvers != nil
}
