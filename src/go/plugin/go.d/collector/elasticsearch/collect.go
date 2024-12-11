// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"errors"
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"math"
	"slices"
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

func (c *Collector) collect() (map[string]int64, error) {
	if c.clusterName == "" {
		name, err := c.getClusterName()
		if err != nil {
			return nil, err
		}
		c.clusterName = name
	}

	ms := c.scrapeElasticsearch()
	if ms.empty() {
		return nil, nil
	}

	mx := make(map[string]int64)

	c.collectNodesStats(mx, ms)
	c.collectClusterHealth(mx, ms)
	c.collectClusterStats(mx, ms)
	c.collectLocalIndicesStats(mx, ms)

	return mx, nil
}

func (c *Collector) collectNodesStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasNodesStats() {
		return
	}

	seen := make(map[string]bool)

	for nodeID, node := range ms.NodesStats.Nodes {
		seen[nodeID] = true

		if !c.nodes[nodeID] {
			c.nodes[nodeID] = true
			c.addNodeCharts(nodeID, node)
		}

		merge(mx, stm.ToMap(node), "node_"+nodeID)
	}

	for nodeID := range c.nodes {
		if !seen[nodeID] {
			delete(c.nodes, nodeID)
			c.removeNodeCharts(nodeID)
		}
	}
}

func (c *Collector) collectClusterHealth(mx map[string]int64, ms *esMetrics) {
	if !ms.hasClusterHealth() {
		return
	}

	c.addClusterHealthChartsOnce.Do(c.addClusterHealthCharts)

	merge(mx, stm.ToMap(ms.ClusterHealth), "cluster")

	mx["cluster_status_green"] = metrix.Bool(ms.ClusterHealth.Status == "green")
	mx["cluster_status_yellow"] = metrix.Bool(ms.ClusterHealth.Status == "yellow")
	mx["cluster_status_red"] = metrix.Bool(ms.ClusterHealth.Status == "red")
}

func (c *Collector) collectClusterStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasClusterStats() {
		return
	}

	c.addClusterStatsChartsOnce.Do(c.addClusterStatsCharts)

	merge(mx, stm.ToMap(ms.ClusterStats), "cluster")
}

func (c *Collector) collectLocalIndicesStats(mx map[string]int64, ms *esMetrics) {
	if !ms.hasLocalIndicesStats() {
		return
	}

	seen := make(map[string]bool)

	for _, v := range ms.LocalIndicesStats {
		seen[v.Index] = true

		if !c.indices[v.Index] {
			c.indices[v.Index] = true
			c.addIndexCharts(v.Index)
		}

		px := fmt.Sprintf("node_index_%s_stats_", v.Index)

		mx[px+"health_green"] = metrix.Bool(v.Health == "green")
		mx[px+"health_yellow"] = metrix.Bool(v.Health == "yellow")
		mx[px+"health_red"] = metrix.Bool(v.Health == "red")
		mx[px+"shards_count"] = strToInt(v.Rep)
		mx[px+"docs_count"] = strToInt(v.DocsCount)
		mx[px+"store_size_in_bytes"] = convertIndexStoreSizeToBytes(v.StoreSize)
	}

	for index := range c.indices {
		if !seen[index] {
			delete(c.indices, index)
			c.removeIndexCharts(index)
		}
	}
}

func (c *Collector) scrapeElasticsearch() *esMetrics {
	ms := &esMetrics{}
	wg := &sync.WaitGroup{}

	if c.DoNodeStats {
		wg.Add(1)
		go func() { defer wg.Done(); c.scrapeNodesStats(ms) }()
	}
	if c.DoClusterHealth {
		wg.Add(1)
		go func() { defer wg.Done(); c.scrapeClusterHealth(ms) }()
	}
	if c.DoClusterStats {
		wg.Add(1)
		go func() { defer wg.Done(); c.scrapeClusterStats(ms) }()
	}
	if !c.ClusterMode && c.DoIndicesStats {
		wg.Add(1)
		go func() { defer wg.Done(); c.scrapeLocalIndicesStats(ms) }()
	}
	wg.Wait()

	return ms
}

func (c *Collector) scrapeNodesStats(ms *esMetrics) {
	var p string
	if c.ClusterMode {
		p = urlPathNodesStats
	} else {
		p = urlPathLocalNodeStats
	}

	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, p)

	var stats esNodesStats
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.NodesStats = &stats
}

func (c *Collector) scrapeClusterHealth(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathClusterHealth)

	var health esClusterHealth
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &health); err != nil {
		c.Warning(err)
		return
	}

	ms.ClusterHealth = &health
}

func (c *Collector) scrapeClusterStats(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathClusterStats)

	var stats esClusterStats
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.ClusterStats = &stats
}

func (c *Collector) scrapeLocalIndicesStats(ms *esMetrics) {
	req, _ := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathIndicesStats)
	req.URL.RawQuery = "local=true&format=json"

	var stats []esIndexStats
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		c.Warning(err)
		return
	}

	ms.LocalIndicesStats = removeSystemIndices(stats)
}

func (c *Collector) getClusterName() (string, error) {
	req, _ := web.NewHTTPRequest(c.RequestConfig)

	var info struct {
		ClusterName string `json:"cluster_name"`
	}
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &info); err != nil {
		return "", err
	}

	if info.ClusterName == "" {
		return "", errors.New("empty cluster name")
	}

	return info.ClusterName, nil
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

func removeSystemIndices(indices []esIndexStats) []esIndexStats {
	return slices.DeleteFunc(indices, func(stats esIndexStats) bool {
		return strings.HasPrefix(stats.Index, ".")
	})
}

func merge(dst, src map[string]int64, prefix string) {
	for k, v := range src {
		dst[prefix+"_"+k] = v
	}
}
