// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

const (
	queryShowGlobalVariables = `
SHOW GLOBAL VARIABLES 
WHERE 
  Variable_name LIKE 'max_connections' 
  OR Variable_name LIKE 'table_open_cache' 
  OR Variable_name LIKE 'disabled_storage_engines' 
  OR Variable_name LIKE 'log_bin'
  OR Variable_name LIKE 'performance_schema';`
)

func (c *Collector) collectGlobalVariables() error {
	// MariaDB: https://mariadb.com/kb/en/server-system-variables/
	// MySQL: https://dev.mysql.com/doc/refman/8.0/en/server-system-variable-reference.html
	q := queryShowGlobalVariables
	c.Debugf("executing query: '%s'", q)

	var name string
	_, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			switch name {
			case "disabled_storage_engines":
				c.varDisabledStorageEngine = value
			case "log_bin":
				c.varLogBin = value
			case "max_connections":
				c.varMaxConns = parseInt(value)
			case "performance_schema":
				c.varPerformanceSchema = value
			case "table_open_cache":
				c.varTableOpenCache = parseInt(value)
			}
		}
	})
	return err
}
