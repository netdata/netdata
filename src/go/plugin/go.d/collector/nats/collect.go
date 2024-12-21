// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectHealthz(mx); err != nil {
		return nil, err
	}
	if err := c.collectVarz(mx); err != nil {
		return mx, err
	}
	if err := c.collectAccstatz(mx); err != nil {
		return mx, err
	}

	return mx, nil
}

func (c *Collector) collectHealthz(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathHealthz)
	if err != nil {
		return err
	}

	switch c.HealthzCheck {
	case "js-enabled-only":
		req.URL.Scheme = urlQueryHealthzJsEnabledOnly
	case "js-server-only":
		req.URL.Scheme = urlQueryHealthzJsServerOnly
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

	mx["varz_srv_uptime"] = int64(resp.Now.Sub(resp.Start).Seconds())
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

	seen := make(map[string]bool)

	for _, acc := range resp.AccStats {
		if acc.Account == "" {
			continue
		}

		seen[acc.Account] = true

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

	for acc := range seen {
		if !c.seenAccounts[acc] {
			c.seenAccounts[acc] = true
			c.addAccountCharts(acc)
		}
	}
	for acc := range c.seenAccounts {
		if !seen[acc] {
			delete(c.seenAccounts, acc)
			c.removeAccountCharts(acc)
		}
	}

	return nil
}
