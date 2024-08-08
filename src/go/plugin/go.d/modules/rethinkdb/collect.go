// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

type (
	// https://rethinkdb.com/docs/system-stats/
	serverStats struct {
		ID          []string `json:"id"`
		Server      string   `json:"server"`
		QueryEngine struct {
			ClientConnections int64 `json:"client_connections" stm:"client_connections"`
			ClientsActive     int64 `json:"clients_active" stm:"clients_active"`
			QueriesTotal      int64 `json:"queries_total" stm:"queries_total"`
			ReadDocsTotal     int64 `json:"read_docs_total" stm:"read_docs_total"`
			WrittenDocsTotal  int64 `json:"written_docs_total" stm:"written_docs_total"`
		} `json:"query_engine" stm:""`

		Error string `json:"error"`
	}
)

func (r *Rethinkdb) collect() (map[string]int64, error) {
	if r.rdb == nil {
		conn, err := r.newConn(r.Config)
		if err != nil {
			return nil, err
		}
		r.rdb = conn
	}

	mx := make(map[string]int64)

	if err := r.collectStats(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (r *Rethinkdb) collectStats(mx map[string]int64) error {
	resp, err := r.rdb.stats()
	if err != nil {
		return err
	}

	if len(resp) == 0 {
		return errors.New("empty stats response from server")
	}

	for _, v := range []string{
		"cluster_servers_stats_request_success",
		"cluster_servers_stats_request_timeout",
		"cluster_client_connections",
		"cluster_clients_active",
		"cluster_queries_total",
		"cluster_read_docs_total",
		"cluster_written_docs_total",
	} {
		mx[v] = 0
	}

	seen := make(map[string]bool)

	for _, bs := range resp[1:] { // skip cluster
		var srv serverStats

		if err := json.Unmarshal(bs, &srv); err != nil {
			return fmt.Errorf("invalid stats response: failed to unmarshal server data: %v", err)
		}
		if len(srv.ID[0]) == 0 {
			return errors.New("invalid stats response: empty id")
		}
		if srv.ID[0] != "server" {
			continue
		}
		if len(srv.ID) != 2 {
			return fmt.Errorf("invalid stats response: unexpected server id: '%v'", srv.ID)
		}

		srvUUID := srv.ID[1]

		seen[srvUUID] = true

		if !r.seenServers[srvUUID] {
			r.seenServers[srvUUID] = true
			r.addServerCharts(srvUUID, srv.Server)
		}

		px := fmt.Sprintf("server_%s_", srv.ID[1]) // uuid

		mx[px+"stats_request_status_success"] = 0
		mx[px+"stats_request_status_timeout"] = 0
		if srv.Error != "" {
			mx["cluster_servers_stats_request_timed_out"]++
			mx[px+"stats_request_status_timeout"] = 1
			continue
		}
		mx["cluster_servers_stats_request_success"]++
		mx[px+"stats_request_status_success"] = 1

		for k, v := range stm.ToMap(srv.QueryEngine) {
			mx["cluster_"+k] += v
			mx[px+k] = v
		}
	}

	for k := range r.seenServers {
		if !seen[k] {
			delete(r.seenServers, k)
			r.removeServerCharts(k)
		}
	}

	return nil
}
