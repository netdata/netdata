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

	clusterStats struct {
		ID          []string `json:"id"`
		QueryEngine struct {
			ClientConnections int64   `json:"client_connections" stm:"client_connections""`
			ClientsActive     int64   `json:"clients_active" stm:"client_active"`
			QueriesPerSec     float64 `json:"queries_per_sec" stm:"queries_per_sec"`
			ReadDocsPerSec    float64 `json:"read_docs_per_sec" stm:"read_docs_per_sec"`
			WrittenDocsPerSec float64 `json:"written_docs_per_sec" stm:"written_docs_per_sec"`
		} `json:"query_engine" stm:""`
	}

	serverStats struct {
		ID          []string `json:"id"`
		Server      string   `json:"server"`
		QueryEngine struct {
			ClientConnections int64   `json:"client_connections" stm:"client_connections"`
			ClientsActive     int64   `json:"clients_active" stm:"client_active"`
			QueriesPerSec     float64 `json:"queries_per_sec" stm:"queries_per_sec"`
			QueriesTotal      int64   `json:"queries_total" stm:"queries_total"`
			ReadDocsPerSec    float64 `json:"read_docs_per_sec" stm:"read_docs_per_sec"`
			ReadDocsTotal     int64   `json:"read_docs_total" stm:"read_docs_total"`
			WrittenDocsPerSec float64 `json:"written_docs_per_sec" stm:"written_docs_per_sec"`
			WrittenDocsTotal  int64   `json:"written_docs_total" stm:"written_docs_total"`
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

	for k, v := range mx {
		fmt.Println(k, v)
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

	var cs clusterStats
	if err := json.Unmarshal(resp[0], &cs); err != nil {
		return err
	}

	if len(cs.ID) != 1 || cs.ID[0] != "cluster" {
		return fmt.Errorf("invalid stats response from server: invalid cluster id: '%v'", cs.ID)
	}

	for k, v := range stm.ToMap(cs) {
		mx["cluster_"+k] = v
	}

	for _, bs := range resp[1:] {
		var srv serverStats

		if err := json.Unmarshal(bs, &srv); err != nil {
			return err
		}

		if len(srv.ID) != 2 {
			return fmt.Errorf("invalid stats response from server: invalid server id: '%v'", srv.ID)
		}

		if srv.ID[0] != "server" {
			continue
		}

		if srv.Error != "" {
			continue
		}

		px := fmt.Sprintf("server_%s_", srv.ID[1]) // uuid

		for k, v := range stm.ToMap(srv) {
			mx[px+k] = v
		}
	}

	return nil
}
