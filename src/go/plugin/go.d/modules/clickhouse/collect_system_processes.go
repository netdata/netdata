// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const queryLongestQueryTime = `
SELECT
    toString(max(elapsed)) as value 
FROM
    system.processes FORMAT CSVWithNames
`

func (c *ClickHouse) collectLongestRunningQueryTime(mx map[string]int64) error {
	req, _ := web.NewHTTPRequest(c.Request)
	req.URL.RawQuery = makeURLQuery(queryLongestQueryTime)

	return c.doOKDecodeCSV(req, func(column, value string, lineEnd bool) {
		if column == "value" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				mx["LongestRunningQueryTime"] = int64(v * precision)
			}
		}
	})
}
