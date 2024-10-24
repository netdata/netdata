// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathMaxscale        = "/maxscale"
	urlPathMaxscaleThreads = "/maxscale/threads"
	urlPathServers         = "/servers"
)

func (m *MaxScale) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := m.collectMaxScaleGlobal(mx); err != nil {
		return nil, err
	}
	if err := m.collectMaxScaleThreads(mx); err != nil {
		return nil, err
	}
	if err := m.collectServers(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (m *MaxScale) collectMaxScaleGlobal(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(m.RequestConfig, urlPathMaxscale)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp maxscaleGlobalResponse

	if err := web.DoHTTP(m.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	if resp.Data == nil {
		return fmt.Errorf("invalid response from '%s': missing expected MaxScale data", req.URL)
	}

	mx["uptime"] = resp.Data.Attrs.Uptime

	return nil
}

func (m *MaxScale) collectMaxScaleThreads(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(m.RequestConfig, urlPathMaxscaleThreads)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp maxscaleThreadsResponse

	if err := web.DoHTTP(m.httpClient).RequestJSON(req, &resp); err != nil {
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

func (m *MaxScale) collectServers(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(m.RequestConfig, urlPathServers)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var resp serversResponse

	if err := web.DoHTTP(m.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, r := range resp.Data {
		if r.ID == "" {
			continue
		}

		seen[r.ID] = true

		if !m.seenServers[r.ID] {
			m.seenServers[r.ID] = true
			m.addServerCharts(r.ID, r.Attrs.Params.Address)
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

	for id := range m.seenServers {
		if !seen[id] {
			delete(m.seenServers, id)
			m.removeServerCharts(id)
		}
	}

	return nil
}
