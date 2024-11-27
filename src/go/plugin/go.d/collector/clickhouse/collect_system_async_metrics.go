// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const querySystemAsyncMetrics = `
SELECT
    metric,
    value 
FROM
    system.asynchronous_metrics 
where
    metric LIKE 'Uptime' 
    OR metric LIKE 'MaxPartCountForPartition' 
    OR metric LIKE 'ReplicasMaxAbsoluteDelay' FORMAT CSVWithNames
`

func (c *Collector) collectSystemAsyncMetrics(mx map[string]int64) error {
	req, _ := web.NewHTTPRequest(c.RequestConfig)
	req.URL.RawQuery = makeURLQuery(querySystemAsyncMetrics)

	want := map[string]float64{
		"Uptime":                   1,
		"MaxPartCountForPartition": 1,
		"ReplicasMaxAbsoluteDelay": precision,
	}

	px := "async_metrics_"
	var metric string
	var n int

	err := c.doHTTP(req, func(column, value string, lineEnd bool) {
		switch column {
		case "metric":
			metric = value
		case "value":
			mul, ok := want[metric]
			if !ok {
				return
			}
			n++
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				mx[px+metric] = int64(v * mul)
			}
		}
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no system async metrics data returned")
	}

	return nil
}
