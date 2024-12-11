// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strconv"
)

const querySysMetrics = `
SELECT
    METRIC_NAME,
    VALUE
FROM
    v$sysmetric
WHERE
    METRIC_NAME IN (
        'Session Count',
        'Session Limit %',
        'Average Active Sessions',
        'Buffer Cache Hit Ratio',
        'Cursor Cache Hit Ratio',
        'Library Cache Hit Ratio',
        'Row Cache Hit Ratio',
        'Global Cache Blocks Corrupted',
        'Global Cache Blocks Lost',
        'Database Wait Time Ratio',
        'SQL Service Response Time'
    )
    AND
    intsize_csec
    = (SELECT max(intsize_csec) FROM sys.v_$sysmetric)
`

func (c *Collector) collectSysMetrics(mx map[string]int64) error {
	q := querySysMetrics
	c.Debugf("executing query: %s", q)

	var name, val string

	return c.doQuery(q, func(column, value string, lineEnd bool) error {
		switch column {
		case "METRIC_NAME":
			name = value
		case "VALUE":
			val = value
		}
		if lineEnd {
			v, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return fmt.Errorf("could not parse metric '%s' value '%s': %w", name, val, err)
			}
			mx[name] = int64(v * precision)

		}
		return nil
	})
}
