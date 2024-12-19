// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathVarz    = "/varz"
	urlPathHealthz = "/healthz"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectVarz(mx); err != nil {
		return nil, err
	}
	if err := c.collectHealthz(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectVarz(mx map[string]int64) error {
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#general-information
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathVarz)
	if err != nil {
		return err
	}

	var resp varzResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	mx["uptime"] = int64(resp.Now.Sub(resp.Start).Seconds())
	mx["in_msgs"] = resp.InMsgs
	mx["out_msgs"] = resp.OutMsgs
	mx["in_bytes"] = resp.InBytes
	mx["out_bytes"] = resp.OutBytes
	mx["slow_consumers"] = resp.SlowConsumers
	mx["subscriptions"] = int64(resp.Subscriptions)
	mx["connections"] = int64(resp.Connections)
	mx["total_connections"] = int64(resp.TotalConnections)
	mx["routes"] = int64(resp.Routes)
	mx["remotes"] = int64(resp.Remotes)
	mx["cpu"] = int64(resp.CPU)
	mx["mem"] = resp.Mem

	for _, path := range httpEndpoints {
		v := resp.HTTPReqStats[path]
		mx[fmt.Sprintf("http_endpoint_%s_req", path)] = int64(v)
	}

	return nil
}

func (c *Collector) collectHealthz(mx map[string]int64) error {
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#health
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathHealthz)
	if err != nil {
		return err
	}

	var resp healthzResponse
	client := web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) { return true, nil })
	if err := client.RequestJSON(req, &resp); err != nil {
		return err
	}
	if resp.Status == nil {
		return fmt.Errorf("healthz response missing status")
	}

	mx["healthz_status_ok"] = metrix.Bool(*resp.Status == "ok")
	mx["healthz_status_error"] = metrix.Bool(*resp.Status != "ok")

	return nil
}
