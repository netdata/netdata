// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	srvTypeMaster  = "master"
	srvTypeTServer = "tserver"
	srvTypeCQL     = "ycql"
	srvTypeSQL     = "ysql"
)

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if c.srvType == "" {
		if c.srvType = c.getSrvType(mfs); c.srvType == "" {
			return nil, fmt.Errorf("could not determine server type")
		}
		c.addBaseCharts()
	}

	mx := make(map[string]int64)

	c.collectMetrics(mx, mfs)

	return mx, nil
}

func (c *Collector) getSrvType(mfs prometheus.MetricFamilies) string {
	for _, mf := range mfs {
		for _, m := range mf.Metrics() {
			switch m.Labels().Get("metric_id") {
			case "yb.master":
				return srvTypeMaster
			case "yb.tabletserver":
				return srvTypeTServer
			case "yb.cqlserver":
				return srvTypeCQL
			case "yb.ysqlserver":
				return srvTypeSQL
			}
		}
	}
	return ""
}

func (c *Collector) collectMetrics(mx map[string]int64, mfs prometheus.MetricFamilies) {
	var maxConn float64
	var usedConn float64

	for _, mf := range mfs {
		if len(mf.Metrics()) > 1 {
			continue
		}

		if v, ok := strings.CutSuffix(mf.Name(), "_count"); ok {
			if v, ok = strings.CutPrefix(v, metricPxMasterLatencyMasterClient+"_"); ok {
				if !c.cacheHasp("MasterClient", v) {
					c.addMasterClientOpCharts(v)
				}
			} else if v, ok = strings.CutPrefix(v, metricPxMasterLatencyMasterDdl+"_"); ok {
				if !c.cacheHasp("MasterDDL", v) {
					c.addMasterDDLOpCharts(v)
				}
			} else if v, ok = strings.CutPrefix(v, metricPxServerLatencyConsensusService+"_"); ok {
				if !c.cacheHasp("ConsensusService", v) {
					c.addConsensusServiceOpCharts(v)
				}
			} else if v, ok = strings.CutPrefix(v, metricPxTserverHandlerLatency+"_"); ok {
				if svc, op, ok := strings.Cut(v, "_"); ok {
					if !c.cacheHasp(svc, op) {
						c.addServiceOpCharts(svc, op)
					}
				}
			} else if v, ok = strings.CutPrefix(v, metricPxYCQLLatencySQLProcessor+"_"); ok {
				if strings.HasSuffix(v, "Stmt") || strings.HasSuffix(v, "Stmts") {
					if !c.cacheHasp("SQLProcessor", v) {
						c.addCQLStatementCharts(v)
					}
				}
			} else if v, ok = strings.CutPrefix(v, metricPxYSQLLatencySQLProcessor+"_"); ok {
				if strings.HasSuffix(v, "Stmt") || strings.HasSuffix(v, "Stmts") {
					if !c.cacheHasp("SQLProcessor", v) {
						c.addSQLStatementCharts(v)
					}
				}
			}
		}

		m := mf.Metrics()[0]

		switch {
		case mf.Name() == metricSqlConnTotal && m.Gauge() != nil:
			usedConn = m.Gauge().Value()
		case mf.Name() == metricSqlMaxConnTotal && m.Gauge() != nil:
			maxConn = m.Gauge().Value()
		}

		if m.Counter() != nil {
			mx[mf.Name()] += int64(m.Counter().Value())
		}
		if m.Gauge() != nil {
			mx[mf.Name()] += int64(m.Gauge().Value())
		}
	}

	if maxConn > 0 {
		mx["yb_ysqlserver_connection_available"] = int64(maxConn - usedConn)
	}
}

func (c *Collector) cacheHasp(key, subkey string) bool {
	if _, ok := c.cache[key]; !ok {
		c.cache[key] = make(map[string]bool)
	}
	_, ok := c.cache[key][subkey]
	if !ok {
		c.cache[key][subkey] = true
	}
	return ok
}
