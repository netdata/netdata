// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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

func (n *NginxPlus) queryAPIVersion() (int64, error) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, urlPathAPIVersions)

	var versions nginxAPIVersions
	if err := n.doWithDecode(&versions, req); err != nil {
		return 0, err
	}

	if len(versions) == 0 {
		return 0, fmt.Errorf("'%s' returned no data", req.URL)
	}

	return versions[len(versions)-1], nil
}

func (n *NginxPlus) queryAvailableEndpoints() error {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsRoot, n.apiVersion))

	var endpoints []string
	if err := n.doWithDecode(&endpoints, req); err != nil {
		return err
	}

	n.Debugf("discovered root endpoints: %v", endpoints)
	var hasHTTP, hasStream bool
	for _, v := range endpoints {
		switch v {
		case "nginx":
			n.endpoints.nginx = true
		case "connections":
			n.endpoints.connections = true
		case "ssl":
			n.endpoints.ssl = true
		case "resolvers":
			n.endpoints.resolvers = true
		case "http":
			hasHTTP = true
		case "stream":
			hasStream = true
		}
	}

	if hasHTTP {
		endpoints = endpoints[:0]
		req, _ = web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsHTTP, n.apiVersion))

		if err := n.doWithDecode(&endpoints, req); err != nil {
			return err
		}

		n.Debugf("discovered http endpoints: %v", endpoints)
		for _, v := range endpoints {
			switch v {
			case "requests":
				n.endpoints.httpRequest = true
			case "server_zones":
				n.endpoints.httpServerZones = true
			case "location_zones":
				n.endpoints.httpLocationZones = true
			case "caches":
				n.endpoints.httpCaches = true
			case "upstreams":
				n.endpoints.httpUpstreams = true
			}
		}
	}

	if hasStream {
		endpoints = endpoints[:0]
		req, _ = web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIEndpointsStream, n.apiVersion))

		if err := n.doWithDecode(&endpoints, req); err != nil {
			return err
		}

		n.Debugf("discovered stream endpoints: %v", endpoints)
		for _, v := range endpoints {
			switch v {
			case "server_zones":
				n.endpoints.streamServerZones = true
			case "upstreams":
				n.endpoints.streamUpstreams = true
			}
		}
	}

	return nil
}

func (n *NginxPlus) queryMetrics() *nginxMetrics {
	ms := &nginxMetrics{}
	wg := &sync.WaitGroup{}

	for _, task := range []struct {
		do bool
		fn func(*nginxMetrics)
	}{
		{do: n.endpoints.nginx, fn: n.queryNginxInfo},
		{do: n.endpoints.connections, fn: n.queryConnections},
		{do: n.endpoints.ssl, fn: n.querySSL},
		{do: n.endpoints.httpRequest, fn: n.queryHTTPRequests},
		{do: n.endpoints.httpServerZones, fn: n.queryHTTPServerZones},
		{do: n.endpoints.httpLocationZones, fn: n.queryHTTPLocationZones},
		{do: n.endpoints.httpUpstreams, fn: n.queryHTTPUpstreams},
		{do: n.endpoints.httpCaches, fn: n.queryHTTPCaches},
		{do: n.endpoints.streamServerZones, fn: n.queryStreamServerZones},
		{do: n.endpoints.streamUpstreams, fn: n.queryStreamUpstreams},
		{do: n.endpoints.resolvers, fn: n.queryResolvers},
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

func (n *NginxPlus) queryNginxInfo(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPINginx, n.apiVersion))

	var v nginxInfo

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.nginx = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.info = &v
}

func (n *NginxPlus) queryConnections(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIConnections, n.apiVersion))

	var v nginxConnections

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.connections = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.connections = &v
}

func (n *NginxPlus) querySSL(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPISSL, n.apiVersion))

	var v nginxSSL

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.ssl = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.ssl = &v
}

func (n *NginxPlus) queryHTTPRequests(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIHTTPRequests, n.apiVersion))

	var v nginxHTTPRequests

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.httpRequest = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.httpRequests = &v
}

func (n *NginxPlus) queryHTTPServerZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIHTTPServerZones, n.apiVersion))

	var v nginxHTTPServerZones

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.httpServerZones = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.httpServerZones = &v
}

func (n *NginxPlus) queryHTTPLocationZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIHTTPLocationZones, n.apiVersion))

	var v nginxHTTPLocationZones

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.httpLocationZones = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.httpLocationZones = &v
}

func (n *NginxPlus) queryHTTPUpstreams(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIHTTPUpstreams, n.apiVersion))

	var v nginxHTTPUpstreams

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.httpUpstreams = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.httpUpstreams = &v
}

func (n *NginxPlus) queryHTTPCaches(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIHTTPCaches, n.apiVersion))

	var v nginxHTTPCaches

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.httpCaches = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.httpCaches = &v
}

func (n *NginxPlus) queryStreamServerZones(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIStreamServerZones, n.apiVersion))

	var v nginxStreamServerZones

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.streamServerZones = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.streamServerZones = &v
}

func (n *NginxPlus) queryStreamUpstreams(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIStreamUpstreams, n.apiVersion))

	var v nginxStreamUpstreams

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.streamUpstreams = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.streamUpstreams = &v
}

func (n *NginxPlus) queryResolvers(ms *nginxMetrics) {
	req, _ := web.NewHTTPRequestWithPath(n.RequestConfig, fmt.Sprintf(urlPathAPIResolvers, n.apiVersion))

	var v nginxResolvers

	if err := n.doWithDecode(&v, req); err != nil {
		n.endpoints.resolvers = !errors.Is(err, errPathNotFound)
		n.Warning(err)
		return
	}

	ms.resolvers = &v
}

var (
	errPathNotFound = errors.New("path not found")
)

func (n *NginxPlus) doWithDecode(dst interface{}, req *http.Request) error {
	n.Debugf("executing %s '%s'", req.Method, req.URL)
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer web.CloseBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s returned %d status code (%w)", req.URL, resp.StatusCode, errPathNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d status code (%s)", req.URL, resp.StatusCode, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading response from %s : %v", req.URL, err)
	}

	if err := json.Unmarshal(content, dst); err != nil {
		return fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return nil
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
