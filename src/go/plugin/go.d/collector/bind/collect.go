// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	s, err := c.serverStats()
	if err != nil {
		return nil, err
	}
	c.collectServerStats(mx, s)

	return mx, nil
}

func (c *Collector) collectServerStats(metrics map[string]int64, stats *serverStats) {
	var chart *Chart

	for k, v := range stats.NSStats {
		var (
			algo    = module.Incremental
			dimName = k
			chartID string
		)
		switch {
		default:
			continue
		case k == "RecursClients":
			dimName = "clients"
			chartID = keyRecursiveClients
			algo = module.Absolute
		case k == "Requestv4":
			dimName = "IPv4"
			chartID = keyReceivedRequests
		case k == "Requestv6":
			dimName = "IPv6"
			chartID = keyReceivedRequests
		case k == "QryFailure":
			dimName = "failures"
			chartID = keyQueryFailures
		case k == "QryUDP":
			dimName = "UDP"
			chartID = keyProtocolsQueries
		case k == "QryTCP":
			dimName = "TCP"
			chartID = keyProtocolsQueries
		case k == "QrySuccess":
			dimName = "queries"
			chartID = keyQueriesSuccess
		case strings.HasSuffix(k, "QryRej"):
			chartID = keyQueryFailuresDetail
		case strings.HasPrefix(k, "Qry"):
			chartID = keyQueriesAnalysis
		case strings.HasPrefix(k, "Update"):
			chartID = keyReceivedUpdates
		}

		if !c.charts.Has(chartID) {
			_ = c.charts.Add(charts[chartID].Copy())
		}

		chart = c.charts.Get(chartID)

		if !chart.HasDim(k) {
			_ = chart.AddDim(&Dim{ID: k, Name: dimName, Algo: algo})
			chart.MarkNotCreated()
		}

		delete(stats.NSStats, k)
		metrics[k] = v
	}

	for _, v := range []struct {
		item    map[string]int64
		chartID string
	}{
		{item: stats.NSStats, chartID: keyNSStats},
		{item: stats.OpCodes, chartID: keyInOpCodes},
		{item: stats.QTypes, chartID: keyInQTypes},
		{item: stats.SockStats, chartID: keyInSockStats},
	} {
		if len(v.item) == 0 {
			continue
		}

		if !c.charts.Has(v.chartID) {
			_ = c.charts.Add(charts[v.chartID].Copy())
		}

		chart = c.charts.Get(v.chartID)

		for key, val := range v.item {
			if !chart.HasDim(key) {
				_ = chart.AddDim(&Dim{ID: key, Algo: module.Incremental})
				chart.MarkNotCreated()
			}

			metrics[key] = val
		}
	}

	if !(c.permitView != nil && len(stats.Views) > 0) {
		return
	}

	for name, view := range stats.Views {
		if !c.permitView.MatchString(name) {
			continue
		}
		r := view.Resolver

		delete(r.Stats, "BucketSize")

		for key, val := range r.Stats {
			var (
				algo     = module.Incremental
				dimName  = key
				chartKey string
			)

			switch {
			default:
				chartKey = keyResolverStats
			case key == "NumFetch":
				chartKey = keyResolverNumFetch
				dimName = "queries"
				algo = module.Absolute
			case strings.HasPrefix(key, "QryRTT"):
				// TODO: not ordered
				chartKey = keyResolverRTT
			}

			chartID := fmt.Sprintf(chartKey, name)

			if !c.charts.Has(chartID) {
				chart = charts[chartKey].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = c.charts.Add(chart)
			}

			chart = c.charts.Get(chartID)
			dimID := fmt.Sprintf("%s_%s", name, key)

			if !chart.HasDim(dimID) {
				_ = chart.AddDim(&Dim{ID: dimID, Name: dimName, Algo: algo})
				chart.MarkNotCreated()
			}

			metrics[dimID] = val
		}

		if len(r.QTypes) > 0 {
			chartID := fmt.Sprintf(keyResolverInQTypes, name)

			if !c.charts.Has(chartID) {
				chart = charts[keyResolverInQTypes].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = c.charts.Add(chart)
			}

			chart = c.charts.Get(chartID)

			for key, val := range r.QTypes {
				dimID := fmt.Sprintf("%s_%s", name, key)
				if !chart.HasDim(dimID) {
					_ = chart.AddDim(&Dim{ID: dimID, Name: key, Algo: module.Incremental})
					chart.MarkNotCreated()
				}
				metrics[dimID] = val
			}
		}

		if len(r.CacheStats) > 0 {
			chartID := fmt.Sprintf(keyResolverCacheHits, name)

			if !c.charts.Has(chartID) {
				chart = charts[keyResolverCacheHits].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = c.charts.Add(chart)
				for _, dim := range chart.Dims {
					dim.ID = fmt.Sprintf(dim.ID, name)
				}
			}

			metrics[name+"_CacheHits"] = r.CacheStats["CacheHits"]
			metrics[name+"_CacheMisses"] = r.CacheStats["CacheMisses"]
		}
	}
}
