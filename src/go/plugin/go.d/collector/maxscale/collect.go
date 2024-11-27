// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathMaxscale        = "/maxscale"
	urlPathMaxscaleThreads = "/maxscale/threads"
	urlPathServers         = "/servers"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectMaxScaleGlobal(mx); err != nil {
		return nil, err
	}
	if err := c.collectMaxScaleThreads(mx); err != nil {
		return nil, err
	}
	if err := c.collectServers(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectMaxScaleGlobal(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathMaxscale)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp maxscaleGlobalResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	if resp.Data == nil {
		return fmt.Errorf("invalid response from '%s': missing expected MaxScale data", req.URL)
	}

	mx["uptime"] = resp.Data.Attrs.Uptime

	return nil
}

func (c *Collector) collectMaxScaleThreads(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathMaxscaleThreads)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp maxscaleThreadsResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, r := range resp.Data {
		st := r.Attrs.Stats
		mx["threads_reads"] += st.Reads
		mx["threads_writes"] += st.Writes
		mx["threads_errors"] += st.Errors
		mx["threads_hangups"] += st.Hangups
		mx["threads_accepts"] += st.Accepts
		mx["threads_sessions"] += st.Sessions
		mx["threads_zombies"] += st.Zombies
		mx["threads_current_fds"] += st.CurrentDescriptors
		mx["threads_total_fds"] += st.TotalDescriptors
		mx["threads_qc_cache_inserts"] += st.QCCache.Inserts
		mx["threads_qc_cache_evictions"] += st.QCCache.Evictions
		mx["threads_qc_cache_hits"] += st.QCCache.Hits
		mx["threads_qc_cache_misses"] += st.QCCache.Misses
		for _, v := range threadStates {
			mx["threads_state_"+v] = 0
		}
		mx["threads_state_"+st.State]++
	}

	return nil
}

func (c *Collector) collectServers(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathServers)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp serversResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, r := range resp.Data {
		if r.ID == "" {
			continue
		}

		seen[r.ID] = true

		if !c.seenServers[r.ID] {
			c.seenServers[r.ID] = true
			addr := net.JoinHostPort(r.Attrs.Params.Address, strconv.Itoa(int(r.Attrs.Params.Port)))
			c.addServerCharts(r.ID, addr)
		}

		px := fmt.Sprintf("server_%s_", r.ID)

		mx[px+"connections"] = r.Attrs.Statistics.Connections

		for _, v := range serverStates {
			mx[px+"state_"+v] = 0
		}
		for _, v := range strings.FieldsFunc(r.Attrs.State, unicode.IsSpace) {
			mx[px+"state_"+v] = 1
		}
	}

	for id := range c.seenServers {
		if !seen[id] {
			delete(c.seenServers, id)
			c.removeServerCharts(id)
		}
	}

	return nil
}
