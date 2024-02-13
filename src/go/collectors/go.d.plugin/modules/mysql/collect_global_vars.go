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

func (m *MySQL) collectGlobalVariables() error {
	// MariaDB: https://mariadb.com/kb/en/server-system-variables/
	// MySQL: https://dev.mysql.com/doc/refman/8.0/en/server-system-variable-reference.html
	q := queryShowGlobalVariables
	m.Debugf("executing query: '%s'", q)

	var name string
	_, err := m.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			switch name {
			case "disabled_storage_engines":
				m.varDisabledStorageEngine = value
			case "log_bin":
				m.varLogBin = value
			case "max_connections":
				m.varMaxConns = parseInt(value)
			case "performance_schema":
				m.varPerformanceSchema = value
			case "table_open_cache":
				m.varTableOpenCache = parseInt(value)
			}
		}
	})
	return err
}
