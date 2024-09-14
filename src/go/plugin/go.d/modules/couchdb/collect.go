// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathActiveTasks   = "/_active_tasks"
	urlPathOverviewStats = "/_node/%s/_stats"
	urlPathSystemStats   = "/_node/%s/_system"
	urlPathDatabases     = "/_dbs_info"

	httpStatusCodePrefix    = "couchdb_httpd_status_codes_"
	httpStatusCodePrefixLen = len(httpStatusCodePrefix)
)

func (cdb *CouchDB) collect() (map[string]int64, error) {
	ms := cdb.scrapeCouchDB()
	if ms.empty() {
		return nil, nil
	}

	collected := make(map[string]int64)
	cdb.collectNodeStats(collected, ms)
	cdb.collectSystemStats(collected, ms)
	cdb.collectActiveTasks(collected, ms)
	cdb.collectDBStats(collected, ms)

	return collected, nil
}

func (cdb *CouchDB) collectNodeStats(collected map[string]int64, ms *cdbMetrics) {
	if !ms.hasNodeStats() {
		return
	}

	for metric, value := range stm.ToMap(ms.NodeStats) {
		collected[metric] = value
		if strings.HasPrefix(metric, httpStatusCodePrefix) {
			code := metric[httpStatusCodePrefixLen:]
			collected["couchdb_httpd_status_codes_"+string(code[0])+"xx"] += value
		}
	}
}

func (cdb *CouchDB) collectSystemStats(collected map[string]int64, ms *cdbMetrics) {
	if !ms.hasNodeSystem() {
		return
	}

	for metric, value := range stm.ToMap(ms.NodeSystem) {
		collected[metric] = value
	}

	collected["peak_msg_queue"] = findMaxMQSize(ms.NodeSystem.MessageQueues)
}

func (cdb *CouchDB) collectActiveTasks(collected map[string]int64, ms *cdbMetrics) {
	collected["active_tasks_indexer"] = 0
	collected["active_tasks_database_compaction"] = 0
	collected["active_tasks_replication"] = 0
	collected["active_tasks_view_compaction"] = 0

	if !ms.hasActiveTasks() {
		return
	}

	for _, task := range ms.ActiveTasks {
		collected["active_tasks_"+task.Type]++
	}
}

func (cdb *CouchDB) collectDBStats(collected map[string]int64, ms *cdbMetrics) {
	if !ms.hasDBStats() {
		return
	}

	for _, dbStats := range ms.DBStats {
		if dbStats.Error != "" {
			cdb.Warning("database '", dbStats.Key, "' doesn't exist")
			continue
		}
		merge(collected, stm.ToMap(dbStats.Info), "db_"+dbStats.Key)
	}
}

func (cdb *CouchDB) scrapeCouchDB() *cdbMetrics {
	ms := &cdbMetrics{}
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() { defer wg.Done(); cdb.scrapeNodeStats(ms) }()

	wg.Add(1)
	go func() { defer wg.Done(); cdb.scrapeSystemStats(ms) }()

	wg.Add(1)
	go func() { defer wg.Done(); cdb.scrapeActiveTasks(ms) }()

	if len(cdb.databases) > 0 {
		wg.Add(1)
		go func() { defer wg.Done(); cdb.scrapeDBStats(ms) }()
	}

	wg.Wait()
	return ms
}

func (cdb *CouchDB) scrapeNodeStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(cdb.RequestConfig, fmt.Sprintf(urlPathOverviewStats, cdb.Config.Node))

	var stats cdbNodeStats

	if err := cdb.client().RequestJSON(req, &stats); err != nil {
		cdb.Warning(err)
		return
	}

	ms.NodeStats = &stats
}

func (cdb *CouchDB) scrapeSystemStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(cdb.RequestConfig, fmt.Sprintf(urlPathSystemStats, cdb.Config.Node))

	var stats cdbNodeSystem

	if err := cdb.client().RequestJSON(req, &stats); err != nil {
		cdb.Warning(err)
		return
	}

	ms.NodeSystem = &stats
}

func (cdb *CouchDB) scrapeActiveTasks(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(cdb.RequestConfig, urlPathActiveTasks)

	var stats []cdbActiveTask

	if err := cdb.client().RequestJSON(req, &stats); err != nil {
		cdb.Warning(err)
		return
	}

	ms.ActiveTasks = stats
}

func (cdb *CouchDB) scrapeDBStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(cdb.RequestConfig, urlPathDatabases)
	req.Method = http.MethodPost
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	var q struct {
		Keys []string `json:"keys"`
	}
	q.Keys = cdb.databases
	body, err := json.Marshal(q)
	if err != nil {
		cdb.Error(err)
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	var stats []cdbDBStats

	if err := cdb.client().RequestJSON(req, &stats); err != nil {
		cdb.Warning(err)
		return
	}

	ms.DBStats = stats
}

func findMaxMQSize(MessageQueues map[string]interface{}) int64 {
	var maxSize float64
	for _, mq := range MessageQueues {
		switch mqSize := mq.(type) {
		case float64:
			maxSize = math.Max(maxSize, mqSize)
		case map[string]interface{}:
			if v, ok := mqSize["count"].(float64); ok {
				maxSize = math.Max(maxSize, v)
			}
		}
	}
	return int64(maxSize)
}

func (cdb *CouchDB) pingCouchDB() error {
	req, _ := web.NewHTTPRequest(cdb.RequestConfig)

	var info struct{ Couchdb string }

	if err := cdb.client().RequestJSON(req, &info); err != nil {
		return err
	}

	if info.Couchdb != "Welcome" {
		return errors.New("not a CouchDB endpoint")
	}

	return nil
}

func (cdb *CouchDB) client() *web.Client {
	return web.DoHTTP(cdb.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		var msg struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msg); err == nil && msg.Error != "" {
			return false, fmt.Errorf("error '%s', reason '%s'", msg.Error, msg.Reason)
		}
		return false, nil
	})
}

func merge(dst, src map[string]int64, prefix string) {
	for k, v := range src {
		dst[prefix+"_"+k] = v
	}
}
