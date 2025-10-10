// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const querySystemDisks = `
SELECT
    name,
    sum(free_space) as free_space,
    sum(total_space) as total_space 
FROM
    system.disks 
GROUP BY
    name FORMAT CSVWithNames
`

type diskStats struct {
	name       string
	totalBytes int64
	freeBytes  int64
}

func (c *Collector) collectSystemDisks(mx map[string]int64) error {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return err
	}
	req.URL.RawQuery = makeURLQuery(querySystemDisks)

	seen := make(map[string]*diskStats)

	getDisk := func(name string) *diskStats {
		s, ok := seen[name]
		if !ok {
			s = &diskStats{name: name}
			seen[name] = s
		}
		return s
	}

	var name string

	err = c.doHTTP(req, func(column, value string, lineEnd bool) {
		switch column {
		case "name":
			name = value
		case "free_space":
			v, _ := strconv.ParseInt(value, 10, 64)
			getDisk(name).freeBytes = v
		case "total_space":
			v, _ := strconv.ParseInt(value, 10, 64)
			getDisk(name).totalBytes = v
		}
	})
	if err != nil {
		return err
	}

	for _, disk := range seen {
		if _, ok := c.seenDisks[disk.name]; !ok {
			v := &seenDisk{disk: disk.name}
			c.seenDisks[disk.name] = v
			c.addDiskCharts(v)
		}

		px := "disk_" + disk.name + "_"

		mx[px+"free_space_bytes"] = disk.freeBytes
		mx[px+"used_space_bytes"] = disk.totalBytes - disk.freeBytes
	}

	for k, v := range c.seenDisks {
		if _, ok := seen[k]; !ok {
			delete(c.seenDisks, k)
			c.removeDiskCharts(v)
		}
	}

	return nil
}
