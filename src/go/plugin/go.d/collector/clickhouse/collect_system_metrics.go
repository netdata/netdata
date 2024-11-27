// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const querySystemMetrics = `
SELECT
    metric,
    value 
FROM
    system.metrics FORMAT CSVWithNames
`

func (c *Collector) collectSystemMetrics(mx map[string]int64) error {
	req, _ := web.NewHTTPRequest(c.RequestConfig)
	req.URL.RawQuery = makeURLQuery(querySystemMetrics)

	px := "metrics_"
	var metric string
	var n int

	err := c.doHTTP(req, func(column, value string, lineEnd bool) {
		switch column {
		case "metric":
			metric = value
		case "value":
			if !wantSystemMetrics[metric] {
				return
			}
			n++
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				mx[px+metric] = v
			}
		}
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no system metrics data returned")
	}

	return nil
}

var wantSystemMetrics = map[string]bool{
	"Query":                    true,
	"TCPConnection":            true,
	"HTTPConnection":           true,
	"MySQLConnection":          true,
	"PostgreSQLConnection":     true,
	"InterserverConnection":    true,
	"MemoryTracking":           true,
	"QueryPreempted":           true,
	"ReplicatedFetch":          true,
	"ReplicatedSend":           true,
	"ReplicatedChecks":         true,
	"ReadonlyReplica":          true,
	"PartsTemporary":           true,
	"PartsPreActive":           true,
	"PartsActive":              true,
	"PartsDeleting":            true,
	"PartsDeleteOnDestroy":     true,
	"PartsOutdated":            true,
	"PartsWide":                true,
	"PartsCompact":             true,
	"DistributedSend":          true,
	"DistributedFilesToInsert": true,
}
