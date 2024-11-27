// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strconv"
)

const queryWaitClass = `
SELECT
    n.wait_class AS wait_class,
    round(m.time_waited / m.intsize_csec, 3) AS wait_time
FROM
    v$waitclassmetric m,
    v$system_wait_class n
WHERE
    m.wait_class_id = n.wait_class_id
    AND n.wait_class != 'Idle'
`

func (c *Collector) collectWaitClass(mx map[string]int64) error {
	q := queryWaitClass
	c.Debugf("executing query: %s", q)

	seen := make(map[string]bool)
	var wclass, wtime string

	err := c.doQuery(q, func(column, value string, lineEnd bool) error {
		switch column {
		case "WAIT_CLASS":
			wclass = value
		case "WAIT_TIME":
			wtime = value
		}
		if lineEnd {
			seen[wclass] = true

			v, err := strconv.ParseFloat(wtime, 64)
			if err != nil {
				return fmt.Errorf("could not parse class '%s' value '%s': %w", wclass, wtime, err)
			}
			mx["wait_class_"+wclass+"_wait_time"] = int64(v * precision)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for name := range seen {
		if !c.seenWaitClasses[name] {
			c.seenWaitClasses[name] = true
			c.addWaitClassCharts(name)
		}
	}

	return nil
}
