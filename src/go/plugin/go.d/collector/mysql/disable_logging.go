// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

const (
	queryShowSessionVariables = `
SHOW SESSION VARIABLES 
WHERE 
  Variable_name LIKE 'sql_log_off'
  OR Variable_name LIKE 'slow_query_log';`
)

const (
	queryDisableSessionQueryLog     = "SET SESSION sql_log_off='ON';"
	queryDisableSessionSlowQueryLog = "SET SESSION slow_query_log='OFF';"
)

func (m *MySQL) disableSessionQueryLog() {
	q := queryShowSessionVariables
	m.Debugf("executing query: '%s'", q)

	var sqlLogOff, slowQueryLog string
	var name string
	_, err := m.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			switch name {
			case "sql_log_off":
				sqlLogOff = value
			case "slow_query_log":
				slowQueryLog = value
			}
		}
	})
	if err != nil {
		m.Debug(err)
		return
	}

	if sqlLogOff == "OFF" && m.doDisableSessionQueryLog {
		// requires SUPER privileges
		q = queryDisableSessionQueryLog
		m.Debugf("executing query: '%s'", q)
		if _, err := m.collectQuery(q, func(_, _ string, _ bool) {}); err != nil {
			m.Infof("failed to disable session query log (sql_log_off): %v", err)
			m.doDisableSessionQueryLog = false
		}
	}
	if slowQueryLog == "ON" {
		q = queryDisableSessionSlowQueryLog
		m.Debugf("executing query: '%s'", q)
		if _, err := m.collectQuery(q, func(_, _ string, _ bool) {}); err != nil {
			m.Debugf("failed to disable session slow query log (slow_query_log): %v", err)
		}
	}
}
