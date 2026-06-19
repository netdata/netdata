// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/blang/semver/v4"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
)

func (c *Collector) ensureVersionAndCapabilities(ctx context.Context) error {
	if c.version != nil {
		return nil
	}
	if err := c.collectVersion(ctx); err != nil {
		return fmt.Errorf("error on collecting version: %v", err)
	}
	// https://mariadb.com/kb/en/user-statistics/
	c.doUserStatistics = c.isPercona || c.isMariaDB && c.version.GTE(semver.Version{Major: 10, Minor: 1, Patch: 1})
	return nil
}

func (c *Collector) collect(ctx context.Context) error {
	if !c.dbReady() {
		if err := c.openConnection(ctx); err != nil {
			return err
		}
	}
	if err := c.ensureVersionAndCapabilities(ctx); err != nil {
		return err
	}

	c.disableSessionQueryLog(ctx)

	state := &collectRunState{}

	if err := c.collectGlobalStatus(ctx, state); err != nil {
		return fmt.Errorf("error on collecting global status: %v", err)
	}

	if err := c.collectEngineInnoDBStatus(ctx, state); err != nil {
		return fmt.Errorf("error on collecting engine innodb status: %v", err)
	}

	now := time.Now()
	if now.Sub(c.recheckGlobalVarsTime) > c.recheckGlobalVarsEvery {
		if err := c.collectGlobalVariables(ctx); err != nil {
			return fmt.Errorf("error on collecting global variables: %v", err)
		}
		c.recheckGlobalVarsTime = now
	}
	c.mx.set("innodb_log_file_size", c.varInnoDBLogFileSize)
	c.mx.set("innodb_log_files_in_group", c.varInnoDBLogFilesInGroup)

	logGroupCapacity := c.varInnoDBLogFileSize * c.varInnoDBLogFilesInGroup
	c.mx.set("innodb_log_group_capacity", logGroupCapacity)

	// https://mariadb.com/docs/server/server-usage/storage-engines/innodb/innodb-redo-log#determining-the-redo-log-occupancy
	if logGroupCapacity > 0 {
		c.mx.set("innodb_log_occupancy", 100*1000*state.innodbCheckpointAge/logGroupCapacity)
	} else {
		c.mx.set("innodb_log_occupancy", 0)
	}
	c.mx.set("max_connections", c.varMaxConns)
	c.mx.set("table_open_cache", c.varTableOpenCache)

	// TODO: perhaps make a decisions based on privileges? (SHOW GRANTS FOR CURRENT_USER();)
	if c.doSlaveStatus {
		if err := c.collectSlaveStatus(ctx); err != nil {
			c.Warningf("error on collecting slave status: %v", err)
			c.doSlaveStatus = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if c.doUserStatistics {
		if err := c.collectUserStatistics(ctx); err != nil {
			c.Warningf("error on collecting user statistics: %v", err)
			c.doUserStatistics = errors.Is(err, context.DeadlineExceeded)
		}
	}

	if err := c.collectProcessListStatistics(ctx); err != nil {
		c.Errorf("error on collecting process list statistics: %v", err)
	}

	c.mx.set("thread_cache_misses", calcThreadCacheMisses(state.threadsCreated, state.connections))
	return nil
}

func (c *Collector) check(ctx context.Context) error {
	if !c.dbReady() {
		if err := c.openConnection(ctx); err != nil {
			return err
		}
	}

	if err := c.ensureVersionAndCapabilities(ctx); err != nil {
		return err
	}

	if err := c.probeGlobalStatus(ctx); err != nil {
		return fmt.Errorf("error on collecting global status: %v", err)
	}

	if err := c.collectGlobalVariables(ctx); err != nil {
		return fmt.Errorf("error on collecting global variables: %v", err)
	}

	return nil
}

func (c *Collector) openConnection(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	db, err := sql.Open("mysql", c.DSN)
	if err != nil {
		return fmt.Errorf("error on opening a connection with the mysql database [%s]: %v", c.safeDSN, err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging the mysql database [%s]: %v", c.safeDSN, err)
	}

	c.setDB(db)
	return nil
}

func calcThreadCacheMisses(threads, cons int64) int64 {
	if threads == 0 || cons == 0 {
		return 0
	}
	return int64(float64(threads) / float64(cons) * 10000)
}

func (c *Collector) collectQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) (duration int64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	db, err := c.currentDB()
	if err != nil {
		return 0, err
	}

	queryDuration, err := sqlquery.QueryRows(ctx, db, query, assign)
	if err != nil {
		return 0, err
	}
	return queryDuration.Milliseconds(), nil
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
