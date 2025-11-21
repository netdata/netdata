// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const querySystemParts = `
SELECT
    database,
    table,
    sum(bytes) as bytes,
    count() as parts,
    sum(rows) as rows 
FROM
    system.parts 
WHERE
    active = 1 
GROUP BY
    database,
    table FORMAT CSVWithNames
`

type tableStats struct {
	database string
	table    string
	bytes    int64
	parts    int64
	rows     int64
}

func (c *Collector) collectSystemParts(mx map[string]int64) error {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return err
	}
	req.URL.RawQuery = makeURLQuery(querySystemParts)

	seen := make(map[string]*tableStats)

	getTable := func(db, table string) *tableStats {
		k := table + db
		s, ok := seen[k]
		if !ok {
			s = &tableStats{database: db, table: table}
			seen[k] = s
		}
		return s
	}

	var database, table string

	err = c.doHTTP(req, func(column, value string, lineEnd bool) {
		switch column {
		case "database":
			database = value
		case "table":
			table = value
		case "bytes":
			v, _ := strconv.ParseInt(value, 10, 64)
			getTable(database, table).bytes = v
		case "parts":
			v, _ := strconv.ParseInt(value, 10, 64)
			getTable(database, table).parts = v
		case "rows":
			v, _ := strconv.ParseInt(value, 10, 64)
			getTable(database, table).rows = v
		}
	})
	if err != nil {
		return err
	}

	for _, table := range seen {
		k := table.table + table.database
		if _, ok := c.seenDbTables[k]; !ok {
			v := &seenTable{db: table.database, table: table.table}
			c.seenDbTables[k] = v
			c.addTableCharts(v)
		}

		px := fmt.Sprintf("table_%s_database_%s_", table.table, table.database)

		mx[px+"size_bytes"] = table.bytes
		mx[px+"parts"] = table.parts
		mx[px+"rows"] = table.rows
	}

	for k, v := range c.seenDbTables {
		if _, ok := seen[k]; !ok {
			delete(c.seenDbTables, k)
			c.removeTableCharts(v)
		}
	}

	return nil
}
