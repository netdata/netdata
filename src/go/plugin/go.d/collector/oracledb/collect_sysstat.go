// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strconv"
)

const querySysStat = `
SELECT
    name,
    value
FROM
    v$sysstat
WHERE
    name IN (
        'enqueue timeouts',
        'table scans (long tables)',
        'table scans (short tables)',
        'sorts (disk)',
        'sorts (memory)',
        'physical write bytes',
        'physical read bytes',
        'physical writes',
        'physical reads',
        'logons cumulative',
        'logons current',
        'parse count (total)',
        'execute count',
        'user commits',
        'user rollbacks'
    )
`

func (c *Collector) collectSysStat(mx map[string]int64) error {
	q := querySysStat
	c.Debugf("executing query: %s", q)

	var name, val string

	return c.doQuery(q, func(column, value string, lineEnd bool) error {
		switch column {
		case "NAME":
			name = value
		case "VALUE":
			val = value
		}
		if lineEnd {
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("could not parse activity '%s' value '%s': %w", name, val, err)
			}
			mx[name] = v
		}
		return nil
	})
}
