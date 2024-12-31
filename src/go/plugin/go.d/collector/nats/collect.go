// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.srvMeta.id == "" {
		if err := c.getServerMeta(); err != nil {
			return nil, err
		}
	}

	mx := make(map[string]int64)

	c.cache.resetUpdated()

	if err := c.collectHealthz(mx); err != nil {
		return nil, err
	}
	if err := c.collectVarz(mx); err != nil {
		return mx, err
	}
	if err := c.collectAccstatz(mx); err != nil {
		return mx, err
	}
	if err := c.collectRoutez(mx); err != nil {
		return mx, err
	}
	if err := c.collectGatewayz(mx); err != nil {
		return mx, err
	}
	if err := c.collectLeafz(mx); err != nil {
		return mx, err
	}
	if err := c.collectJsz(mx); err != nil {
		return mx, err
	}

	c.updateCharts()

	return mx, nil
}

func (c *Collector) getServerMeta() error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathVarz)
	if err != nil {
		return err
	}

	var resp struct {
		ID      string `json:"server_id"`
		Name    string `json:"server_name"`
		Cluster struct {
			Name string `json:"name"`
		} `json:"cluster"`
	}

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	c.srvMeta.id = resp.ID
	c.srvMeta.name = resp.Name
	c.srvMeta.clusterName = resp.Cluster.Name

	return nil
}

func (c *Collector) collectHealthz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathHealthz)
	if err != nil {
		return err
	}

	switch c.HealthzCheck {
	case "js-enabled-only":
		req.URL.RawQuery = urlQueryHealthzJsEnabledOnly
	case "js-server-only":
		req.URL.RawQuery = urlQueryHealthzJsServerOnly
	}

	var resp healthzResponse
	client := web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) { return true, nil })
	if err := client.RequestJSON(req, &resp); err != nil {
		return err
	}
	if resp.Status == nil {
		return fmt.Errorf("healthz response missing status")
	}

	mx["varz_srv_healthz_status_ok"] = metrix.Bool(*resp.Status == "ok")
	mx["varz_srv_healthz_status_error"] = metrix.Bool(*resp.Status != "ok")

	return nil
}

func (c *Collector) collectVarz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathVarz)
	if err != nil {
		return err
	}

	var resp varzResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	uptime, _ := parseUptime(resp.Uptime)
	mx["varz_srv_uptime"] = int64(uptime.Seconds())
	mx["varz_srv_in_msgs"] = resp.InMsgs
	mx["varz_srv_out_msgs"] = resp.OutMsgs
	mx["varz_srv_in_bytes"] = resp.InBytes
	mx["varz_srv_out_bytes"] = resp.OutBytes
	mx["varz_srv_slow_consumers"] = resp.SlowConsumers
	mx["varz_srv_subscriptions"] = int64(resp.Subscriptions)
	mx["varz_srv_connections"] = int64(resp.Connections)
	mx["varz_srv_total_connections"] = int64(resp.TotalConnections)
	mx["varz_srv_routes"] = int64(resp.Routes)
	mx["varz_srv_remotes"] = int64(resp.Remotes)
	mx["varz_srv_cpu"] = int64(resp.CPU)
	mx["varz_srv_mem"] = resp.Mem

	for _, path := range httpEndpoints {
		v := resp.HTTPReqStats[path]
		mx[fmt.Sprintf("varz_http_endpoint_%s_req", path)] = int64(v)
	}

	return nil
}

func (c *Collector) collectAccstatz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAccstatz)
	if err != nil {
		return err
	}

	req.URL.RawQuery = urlQueryAccstatz

	var resp accstatzResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, acc := range resp.AccStats {
		c.cache.accounts.put(acc)

		px := fmt.Sprintf("accstatz_acc_%s_", acc.Account)

		mx[px+"conns"] = int64(acc.Conns)
		mx[px+"total_conns"] = int64(acc.TotalConns)
		mx[px+"num_subs"] = int64(acc.NumSubs)
		mx[px+"leaf_nodes"] = int64(acc.LeafNodes)
		mx[px+"slow_consumers"] = acc.SlowConsumers
		mx[px+"received_bytes"] = acc.Received.Bytes
		mx[px+"received_msgs"] = acc.Received.Msgs
		mx[px+"sent_bytes"] = acc.Sent.Bytes
		mx[px+"sent_msgs"] = acc.Sent.Msgs
	}

	return nil
}

func (c *Collector) collectRoutez(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathRoutez)
	if err != nil {
		return err
	}

	var resp routezResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, route := range resp.Routes {
		c.cache.routes.put(route)

		px := fmt.Sprintf("routez_route_id_%d_", route.Rid)

		mx[px+"in_bytes"] = route.InBytes
		mx[px+"out_bytes"] = route.OutBytes
		mx[px+"in_msgs"] = route.InMsgs
		mx[px+"out_msgs"] = route.OutMsgs
		mx[px+"num_subs"] = int64(route.NumSubs)
	}

	return nil
}

func (c *Collector) collectGatewayz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathGatewayz)
	if err != nil {
		return err
	}

	var resp gatewayzResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	for name, ogw := range resp.OutboundGateways {
		c.cache.outGateways.put(resp.Name, name, ogw)

		px := fmt.Sprintf("gatewayz_outbound_gw_%s_cid_%d_", name, ogw.Connection.Cid)

		mx[px+"in_bytes"] = ogw.Connection.InBytes
		mx[px+"out_bytes"] = ogw.Connection.OutBytes
		mx[px+"in_msgs"] = ogw.Connection.InMsgs
		mx[px+"out_msgs"] = ogw.Connection.OutMsgs
		mx[px+"num_subs"] = int64(ogw.Connection.NumSubs)
		uptime, _ := parseUptime(ogw.Connection.Uptime)
		mx[px+"uptime"] = int64(uptime.Seconds())
	}

	for name, igws := range resp.InboundGateways {
		for _, igw := range igws {
			c.cache.inGateways.put(resp.Name, name, igw)

			px := fmt.Sprintf("gatewayz_inbound_gw_%s_cid_%d_", name, igw.Connection.Cid)

			mx[px+"in_bytes"] = igw.Connection.InBytes
			mx[px+"out_bytes"] = igw.Connection.OutBytes
			mx[px+"in_msgs"] = igw.Connection.InMsgs
			mx[px+"out_msgs"] = igw.Connection.OutMsgs
			mx[px+"num_subs"] = int64(igw.Connection.NumSubs)
			uptime, _ := parseUptime(igw.Connection.Uptime)
			mx[px+"uptime"] = int64(uptime.Seconds())
		}
	}

	return nil
}

func (c *Collector) collectLeafz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathLeafz)
	if err != nil {
		return err
	}

	var resp leafzResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, leaf := range resp.Leafs {
		c.cache.leafs.put(leaf)
		px := fmt.Sprintf("leafz_leaf_%s_%s_%s_%d_", leaf.Name, leaf.Account, leaf.IP, leaf.Port)

		mx[px+"in_bytes"] = leaf.InBytes
		mx[px+"out_bytes"] = leaf.OutBytes
		mx[px+"in_msgs"] = leaf.InMsgs
		mx[px+"out_msgs"] = leaf.OutMsgs
		mx[px+"num_subs"] = int64(leaf.NumSubs)
		rtt, _ := time.ParseDuration(leaf.RTT)
		mx[px+"rtt"] = rtt.Microseconds()
	}

	return nil
}

func (c *Collector) collectJsz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathJsz)
	if err != nil {
		return err
	}

	var resp jszResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	mx["jsz_disabled"] = metrix.Bool(resp.Disabled)
	mx["jsz_enabled"] = metrix.Bool(!resp.Disabled)
	mx["jsz_streams"] = int64(resp.Streams)
	mx["jsz_consumers"] = int64(resp.Consumers)
	mx["jsz_bytes"] = int64(resp.Bytes)
	mx["jsz_messages"] = int64(resp.Messages)
	mx["jsz_memory_used"] = int64(resp.Memory)
	mx["jsz_store_used"] = int64(resp.Store)
	mx["jsz_api_total"] = int64(resp.Api.Total)
	mx["jsz_api_errors"] = int64(resp.Api.Errors)
	mx["jsz_api_inflight"] = int64(resp.Api.Inflight)

	return nil
}

func parseUptime(uptime string) (time.Duration, error) {
	// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L1354

	var duration time.Duration
	var num strings.Builder

	for i := 0; i < len(uptime); i++ {
		ch := uptime[i]
		if ch >= '0' && ch <= '9' {
			num.WriteByte(ch)
			continue
		}

		if num.Len() == 0 {
			return 0, fmt.Errorf("invalid format: unit '%c' without number", ch)
		}

		n, err := strconv.Atoi(num.String())
		if err != nil {
			return 0, fmt.Errorf("invalid number in duration: %s", num.String())
		}

		switch ch {
		case 'y':
			duration += time.Duration(n) * 365 * 24 * time.Hour
		case 'd':
			duration += time.Duration(n) * 24 * time.Hour
		case 'h':
			duration += time.Duration(n) * time.Hour
		case 'm':
			duration += time.Duration(n) * time.Minute
		case 's':
			duration += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("invalid unit in duration: %c", ch)
		}
		num.Reset()
	}

	if num.Len() > 0 {
		return 0, fmt.Errorf("invalid format: number without unit at end")
	}

	return duration, nil
}
