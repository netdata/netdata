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

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	urlPathActiveTasks   = "/_active_tasks"
	urlPathOverviewStats = "/_node/%s/_stats"
	urlPathSystemStats   = "/_node/%s/_system"
	urlPathDatabases     = "/_dbs_info"

	httpStatusCodePrefix    = "couchdb_httpd_status_codes_"
	httpStatusCodePrefixLen = len(httpStatusCodePrefix)
)

func (c *Collector) collect() (map[string]int64, error) {
	ms := c.scrapeCouchDB()
	if ms.empty() {
		return nil, nil
	}

	collected := make(map[string]int64)
	c.collectNodeStats(collected, ms)
	c.collectSystemStats(collected, ms)
	c.collectActiveTasks(collected, ms)
	c.collectDBStats(collected, ms)

	return collected, nil
}

func (c *Collector) collectNodeStats(collected map[string]int64, ms *cdbMetrics) {
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

func (c *Collector) collectSystemStats(collected map[string]int64, ms *cdbMetrics) {
	if !ms.hasNodeSystem() {
		return
	}

	for metric, value := range stm.ToMap(ms.NodeSystem) {
		collected[metric] = value
	}

	collected["peak_msg_queue"] = findMaxMQSize(ms.NodeSystem.MessageQueues)
}

func (c *Collector) collectActiveTasks(collected map[string]int64, ms *cdbMetrics) {
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

func (c *Collector) collectDBStats(collected map[string]int64, ms *cdbMetrics) {
	if !ms.hasDBStats() {
		return
	}

	for _, dbStats := range ms.DBStats {
		if dbStats.Error != "" {
			c.Warning("database '", dbStats.Key, "' doesn't exist")
			continue
		}
		merge(collected, stm.ToMap(dbStats.Info), "db_"+dbStats.Key)
	}
}

func (c *Collector) scrapeCouchDB() *cdbMetrics {
	ms := &cdbMetrics{}
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() { defer wg.Done(); c.scrapeNodeStats(ms) }()

	wg.Add(1)
	go func() { defer wg.Done(); c.scrapeSystemStats(ms) }()

	wg.Add(1)
	go func() { defer wg.Done(); c.scrapeActiveTasks(ms) }()

	if len(c.databases) > 0 {
		wg.Add(1)
		go func() { defer wg.Done(); c.scrapeDBStats(ms) }()
	}

	wg.Wait()
	return ms
}

func (c *Collector) scrapeNodeStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathOverviewStats, c.Config.Node))

	var stats cdbNodeStats

	if err := c.client().RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.NodeStats = &stats
}

func (c *Collector) scrapeSystemStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, fmt.Sprintf(urlPathSystemStats, c.Config.Node))

	var stats cdbNodeSystem

	if err := c.client().RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.NodeSystem = &stats
}

func (c *Collector) scrapeActiveTasks(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathActiveTasks)

	var stats []cdbActiveTask

	if err := c.client().RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.ActiveTasks = stats
}

func (c *Collector) scrapeDBStats(ms *cdbMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathDatabases)
	req.Method = http.MethodPost
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	var q struct {
		Keys []string `json:"keys"`
	}
	q.Keys = c.databases
	body, err := json.Marshal(q)
	if err != nil {
		c.Error(err)
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	var stats []cdbDBStats

	if err := c.client().RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.DBStats = stats
}

func findMaxMQSize(MessageQueues map[string]any) int64 {
	var maxSize float64
	for _, mq := range MessageQueues {
		switch mqSize := mq.(type) {
		case float64:
			maxSize = math.Max(maxSize, mqSize)
		case map[string]any:
			if v, ok := mqSize["count"].(float64); ok {
				maxSize = math.Max(maxSize, v)
			}
		}
	}
	return int64(maxSize)
}

func (c *Collector) pingCouchDB() error {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return err
	}

	var info struct{ Couchdb string }

	if err := c.client().RequestJSON(req, &info); err != nil {
		return err
	}

	if info.Couchdb != "Welcome" {
		return errors.New("not a CouchDB endpoint")
	}

	return nil
}

func (c *Collector) client() *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
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
