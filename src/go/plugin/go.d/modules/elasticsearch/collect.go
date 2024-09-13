// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathLocalNodeStats = "/_nodes/_local/stats"
	urlPathNodesStats     = "/_nodes/stats"
	urlPathIndicesStats   = "/_cat/indices"
	urlPathClusterHealth  = "/_cluster/health"
	urlPathClusterStats   = "/_cluster/stats"
)

func (es *Elasticsearch) collect() (map[string]int64, error) {
	if es.clusterName == "" {
		name, err := es.getClusterName()
		if err != nil {
			return nil, err
		}
		es.clusterName = name
	}

	ms := es.scrapeElasticsearch()
	if ms.empty() {
		return nil, nil
	}

	mx := make(map[string]int64)

	es.collectNodesStats(mx, ms)
	es.collectClusterHealth(mx, ms)
	es.collectClusterStats(mx, ms)
	es.collectLocalIndicesStats(mx, ms)

	return mx, nil
}

func (es *Elasticsearch) collectNodesStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasNodesStats() {
		return
	}

	seen := make(map[string]bool)

	for nodeID, node := range ms.NodesStats.Nodes {
		seen[nodeID] = true

		if !es.nodes[nodeID] {
			es.nodes[nodeID] = true
			es.addNodeCharts(nodeID, node)
		}

		merge(mx, stm.ToMap(node), "node_"+nodeID)
	}

	for nodeID := range es.nodes {
		if !seen[nodeID] {
			delete(es.nodes, nodeID)
			es.removeNodeCharts(nodeID)
		}
	}
}

func (es *Elasticsearch) collectClusterHealth(mx map[string]int64, ms *esMetrics) {
	if !ms.hasClusterHealth() {
		return
	}

	es.addClusterHealthChartsOnce.Do(es.addClusterHealthCharts)

	merge(mx, stm.ToMap(ms.ClusterHealth), "cluster")

	mx["cluster_status_green"] = boolToInt(ms.ClusterHealth.Status == "green")
	mx["cluster_status_yellow"] = boolToInt(ms.ClusterHealth.Status == "yellow")
	mx["cluster_status_red"] = boolToInt(ms.ClusterHealth.Status == "red")
}

func (es *Elasticsearch) collectClusterStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasClusterStats() {
		return
	}

	es.addClusterStatsChartsOnce.Do(es.addClusterStatsCharts)

	merge(mx, stm.ToMap(ms.ClusterStats), "cluster")
}

func (es *Elasticsearch) collectLocalIndicesStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasLocalIndicesStats() {
		return
	}

	seen := make(map[string]bool)

	for _, v := range ms.LocalIndicesStats {
		seen[v.Index] = true

		if !es.indices[v.Index] {
			es.indices[v.Index] = true
			es.addIndexCharts(v.Index)
		}

		px := fmt.Sprintf("node_index_%s_stats_", v.Index)

		mx[px+"health_green"] = boolToInt(v.Health == "green")
		mx[px+"health_yellow"] = boolToInt(v.Health == "yellow")
		mx[px+"health_red"] = boolToInt(v.Health == "red")
		mx[px+"shards_count"] = strToInt(v.Rep)
		mx[px+"docs_count"] = strToInt(v.DocsCount)
		mx[px+"store_size_in_bytes"] = convertIndexStoreSizeToBytes(v.StoreSize)
	}

	for index := range es.indices {
		if !seen[index] {
			delete(es.indices, index)
			es.removeIndexCharts(index)
		}
	}
}

func (es *Elasticsearch) scrapeElasticsearch() *esMetrics {
	ms := &esMetrics{}
	wg := &sync.WaitGroup{}

	if es.DoNodeStats {
		wg.Add(1)
		go func() { defer wg.Done(); es.scrapeNodesStats(ms) }()
	}
	if es.DoClusterHealth {
		wg.Add(1)
		go func() { defer wg.Done(); es.scrapeClusterHealth(ms) }()
	}
	if es.DoClusterStats {
		wg.Add(1)
		go func() { defer wg.Done(); es.scrapeClusterStats(ms) }()
	}
	if !es.ClusterMode && es.DoIndicesStats {
		wg.Add(1)
		go func() { defer wg.Done(); es.scrapeLocalIndicesStats(ms) }()
	}
	wg.Wait()

	return ms
}

func (es *Elasticsearch) scrapeNodesStats(ms *esMetrics) {
	var p string
	if es.ClusterMode {
		p = urlPathNodesStats
	} else {
		p = urlPathLocalNodeStats
	}

	req, _ := web.NewHTTPRequestWithPath(es.RequestConfig, p)

	var stats esNodesStats
	if err := es.doOKDecode(req, &stats); err != nil {
		es.Warning(err)
		return
	}

	ms.NodesStats = &stats
}

func (es *Elasticsearch) scrapeClusterHealth(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(es.RequestConfig, urlPathClusterHealth)

	var health esClusterHealth
	if err := es.doOKDecode(req, &health); err != nil {
		es.Warning(err)
		return
	}

	ms.ClusterHealth = &health
}

func (es *Elasticsearch) scrapeClusterStats(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(es.RequestConfig, urlPathClusterStats)

	var stats esClusterStats
	if err := es.doOKDecode(req, &stats); err != nil {
		es.Warning(err)
		return
	}

	ms.ClusterStats = &stats
}

func (es *Elasticsearch) scrapeLocalIndicesStats(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(es.RequestConfig, urlPathIndicesStats)
	req.URL.RawQuery = "local=true&format=json"

	var stats []esIndexStats
	if err := es.doOKDecode(req, &stats); err != nil {
		es.Warning(err)
		return
	}

	ms.LocalIndicesStats = removeSystemIndices(stats)
}

func (es *Elasticsearch) getClusterName() (string, error) {
	req, _ := web.NewHTTPRequest(es.RequestConfig)

	var info struct {
		ClusterName string `json:"cluster_name"`
	}

	if err := es.doOKDecode(req, &info); err != nil {
		return "", err
	}

	if info.ClusterName == "" {
		return "", errors.New("empty cluster name")
	}

	return info.ClusterName, nil
}

func (es *Elasticsearch) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := es.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTPConfig request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}

func convertIndexStoreSizeToBytes(size string) int64 {
	var num float64
	switch {
	case strings.HasSuffix(size, "kb"):
		num, _ = strconv.ParseFloat(size[:len(size)-2], 64)
		num *= math.Pow(1024, 1)
	case strings.HasSuffix(size, "mb"):
		num, _ = strconv.ParseFloat(size[:len(size)-2], 64)
		num *= math.Pow(1024, 2)
	case strings.HasSuffix(size, "gb"):
		num, _ = strconv.ParseFloat(size[:len(size)-2], 64)
		num *= math.Pow(1024, 3)
	case strings.HasSuffix(size, "tb"):
		num, _ = strconv.ParseFloat(size[:len(size)-2], 64)
		num *= math.Pow(1024, 4)
	case strings.HasSuffix(size, "b"):
		num, _ = strconv.ParseFloat(size[:len(size)-1], 64)
	}
	return int64(num)
}

func strToInt(s string) int64 {
	v, _ := strconv.Atoi(s)
	return int64(v)
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func removeSystemIndices(indices []esIndexStats) []esIndexStats {
	var i int
	for _, index := range indices {
		if strings.HasPrefix(index.Index, ".") {
			continue
		}
		indices[i] = index
		i++
	}
	return indices[:i]
}

func merge(dst, src map[string]int64, prefix string) {
	for k, v := range src {
		dst[prefix+"_"+k] = v
	}
}
