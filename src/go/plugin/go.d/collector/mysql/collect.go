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

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}
	if c.version == nil {
		if err := c.collectVersion(); err != nil {
			return nil, fmt.Errorf("error on collecting version: %v", err)
		}
		// https://mariadb.com/kb/en/user-statistics/
		c.doUserStatistics = c.isPercona || c.isMariaDB && c.version.GTE(semver.Version{Major: 10, Minor: 1, Patch: 1})
	}

	c.disableSessionQueryLog()

	mx := make(map[string]int64)

	if err := c.collectGlobalStatus(mx); err != nil {
		return nil, fmt.Errorf("error on collecting global status: %v", err)
	}

	if hasInnodbOSLog(mx) {
		c.addInnoDBOSLogOnce.Do(c.addInnoDBOSLogCharts)
	}
	if hasInnodbDeadlocks(mx) {
		c.addInnodbDeadlocksOnce.Do(c.addInnodbDeadlocksChart)
	}
	if hasQCacheMetrics(mx) {
		c.addQCacheOnce.Do(c.addQCacheCharts)
	}
	if hasGaleraMetrics(mx) {
		c.addGaleraOnce.Do(c.addGaleraCharts)
	}
	if hasTableOpenCacheOverflowsMetrics(mx) {
		c.addTableOpenCacheOverflowsOnce.Do(c.addTableOpenCacheOverflowChart)
	}

	now := time.Now()
	if now.Sub(c.recheckGlobalVarsTime) > c.recheckGlobalVarsEvery {
		if err := c.collectGlobalVariables(); err != nil {
			return nil, fmt.Errorf("error on collecting global variables: %v", err)
		}
		c.recheckGlobalVarsTime = now
	}
	mx["max_connections"] = c.varMaxConns
	mx["table_open_cache"] = c.varTableOpenCache

	if c.isMariaDB || !strings.Contains(c.varDisabledStorageEngine, "MyISAM") {
		c.addMyISAMOnce.Do(c.addMyISAMCharts)
	}
	if c.varLogBin != "OFF" {
		c.addBinlogOnce.Do(c.addBinlogCharts)
	}

	// TODO: perhaps make a decisions based on privileges? (SHOW GRANTS FOR CURRENT_USER();)
	if c.doSlaveStatus {
		if err := c.collectSlaveStatus(mx); err != nil {
			c.Warningf("error on collecting slave status: %v", err)
			c.doSlaveStatus = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if c.doUserStatistics {
		if err := c.collectUserStatistics(mx); err != nil {
			c.Warningf("error on collecting user statistics: %v", err)
			c.doUserStatistics = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if err := c.collectProcessListStatistics(mx); err != nil {
		c.Errorf("error on collecting process list statistics: %v", err)
	}

	calcThreadCacheMisses(mx)
	return mx, nil
}

func (c *Collector) openConnection() error {
	db, err := sql.Open("mysql", c.DSN)
	if err != nil {
		return fmt.Errorf("error on opening a connection with the mysql database [%s]: %v", c.safeDSN, err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging the mysql database [%s]: %v", c.safeDSN, err)
	}

	c.db = db
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

func (c *Collector) collectQuery(query string, assign func(column, value string, lineEnd bool)) (duration int64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	s := time.Now()
	rows, err := c.db.QueryContext(ctx, query)
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
