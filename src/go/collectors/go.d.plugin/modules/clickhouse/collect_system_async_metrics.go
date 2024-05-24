// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"strconv"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

const querySystemAsyncMetrics = `
SELECT
    metric,
    value 
FROM
    system.asynchronous_metrics where metric like 'Uptime' FORMAT CSVWithNames
`

func (c *ClickHouse) collectSystemAsyncMetrics(mx map[string]int64) error {
	req, _ := web.NewHTTPRequest(c.Request)
	req.URL.RawQuery = makeURLQuery(querySystemAsyncMetrics)

	want := map[string]bool{
		"Uptime": true,
	}

	px := "async_metrics_"
	var metric string
	var n int

	err := c.doOKDecodeCSV(req, func(column, value string, lineEnd bool) {
		switch column {
		case "metric":
			metric = value
		case "value":
			if !want[metric] {
				return
			}
			n++
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				mx[px+metric] = int64(v)
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
