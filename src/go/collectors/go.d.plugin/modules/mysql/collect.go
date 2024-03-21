// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver/v4"
)

func (m *MySQL) collect() (map[string]int64, error) {
	if m.db == nil {
		if err := m.openConnection(); err != nil {
			return nil, err
		}
	}
	if m.version == nil {
		if err := m.collectVersion(); err != nil {
			return nil, fmt.Errorf("error on collecting version: %v", err)
		}
		// https://mariadb.com/kb/en/user-statistics/
		m.doUserStatistics = m.isPercona || m.isMariaDB && m.version.GTE(semver.Version{Major: 10, Minor: 1, Patch: 1})
	}

	m.disableSessionQueryLog()

	mx := make(map[string]int64)

	if err := m.collectGlobalStatus(mx); err != nil {
		return nil, fmt.Errorf("error on collecting global status: %v", err)
	}

	if hasInnodbOSLog(mx) {
		m.addInnoDBOSLogOnce.Do(m.addInnoDBOSLogCharts)
	}
	if hasInnodbDeadlocks(mx) {
		m.addInnodbDeadlocksOnce.Do(m.addInnodbDeadlocksChart)
	}
	if hasQCacheMetrics(mx) {
		m.addQCacheOnce.Do(m.addQCacheCharts)
	}
	if hasGaleraMetrics(mx) {
		m.addGaleraOnce.Do(m.addGaleraCharts)
	}
	if hasTableOpenCacheOverflowsMetrics(mx) {
		m.addTableOpenCacheOverflowsOnce.Do(m.addTableOpenCacheOverflowChart)
	}

	now := time.Now()
	if now.Sub(m.recheckGlobalVarsTime) > m.recheckGlobalVarsEvery {
		if err := m.collectGlobalVariables(); err != nil {
			return nil, fmt.Errorf("error on collecting global variables: %v", err)
		}
	}
	mx["max_connections"] = m.varMaxConns
	mx["table_open_cache"] = m.varTableOpenCache

	if m.isMariaDB || !strings.Contains(m.varDisabledStorageEngine, "MyISAM") {
		m.addMyISAMOnce.Do(m.addMyISAMCharts)
	}
	if m.varLogBin != "OFF" {
		m.addBinlogOnce.Do(m.addBinlogCharts)
	}

	// TODO: perhaps make a decisions based on privileges? (SHOW GRANTS FOR CURRENT_USER();)
	if m.doSlaveStatus {
		if err := m.collectSlaveStatus(mx); err != nil {
			m.Warningf("error on collecting slave status: %v", err)
			m.doSlaveStatus = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if m.doUserStatistics {
		if err := m.collectUserStatistics(mx); err != nil {
			m.Warningf("error on collecting user statistics: %v", err)
			m.doUserStatistics = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if err := m.collectProcessListStatistics(mx); err != nil {
		m.Errorf("error on collecting process list statistics: %v", err)
	}

	calcThreadCacheMisses(mx)
	return mx, nil
}

func (m *MySQL) openConnection() error {
	db, err := sql.Open("mysql", m.DSN)
	if err != nil {
		return fmt.Errorf("error on opening a connection with the mysql database [%s]: %v", m.safeDSN, err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging the mysql database [%s]: %v", m.safeDSN, err)
	}

	m.db = db
	return nil
}

func calcThreadCacheMisses(collected map[string]int64) {
	threads, cons := collected["threads_created"], collected["connections"]
	if threads == 0 || cons == 0 {
		collected["thread_cache_misses"] = 0
	} else {
		collected["thread_cache_misses"] = int64(float64(threads) / float64(cons) * 10000)
	}
}

func hasInnodbOSLog(collected map[string]int64) bool {
	// removed in MariaDB 10.8 (https://mariadb.com/kb/en/innodb-status-variables/#innodb_os_log_fsyncs)
	_, ok := collected["innodb_os_log_fsyncs"]
	return ok
}

func hasInnodbDeadlocks(collected map[string]int64) bool {
	_, ok := collected["innodb_deadlocks"]
	return ok
}

func hasGaleraMetrics(collected map[string]int64) bool {
	_, ok := collected["wsrep_received"]
	return ok
}

func hasQCacheMetrics(collected map[string]int64) bool {
	_, ok := collected["qcache_hits"]
	return ok
}

func hasTableOpenCacheOverflowsMetrics(collected map[string]int64) bool {
	_, ok := collected["table_open_cache_overflows"]
	return ok
}

func (m *MySQL) collectQuery(query string, assign func(column, value string, lineEnd bool)) (duration int64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout.Duration())
	defer cancel()

	s := time.Now()
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	duration = time.Since(s).Milliseconds()
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return duration, err
	}

	vs := makeValues(len(columns))
	for rows.Next() {
		if err := rows.Scan(vs...); err != nil {
			return duration, err
		}
		for i, l := 0, len(vs); i < l; i++ {
			assign(columns[i], valueToString(vs[i]), i == l-1)
		}
	}
	return duration, rows.Err()
}

func makeValues(size int) []any {
	vs := make([]any, size)
	for i := range vs {
		vs[i] = &sql.NullString{}
	}
	return vs
}

func valueToString(value any) string {
	v, ok := value.(*sql.NullString)
	if !ok || !v.Valid {
		return ""
	}
	return v.String
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
