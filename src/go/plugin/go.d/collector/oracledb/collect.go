// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const precision = 1000

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, fmt.Errorf("failed to open connection: %v", err)
		}
	}

	mx := make(map[string]int64)

	// TODO: https://www.oracle.com/technical-resources/articles/schumacher-analysis.html

	if err := c.collectSysMetrics(mx); err != nil {
		return nil, fmt.Errorf("failed to collect system metrics: %v", err)
	}
	if err := c.collectSysStat(mx); err != nil {
		return nil, fmt.Errorf("failed to collect activities: %v", err)
	}
	if err := c.collectWaitClass(mx); err != nil {
		return nil, fmt.Errorf("failed to collect wait time: %v", err)
	}
	if err := c.collectTablespace(mx); err != nil {
		return nil, fmt.Errorf("failed to collect tablespace: %v", err)
	}

	return mx, nil
}

func (c *Collector) doQuery(query string, assign func(column, value string, lineEnd bool) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	vs := makeValues(len(columns))

	for rows.Next() {
		if err := rows.Scan(vs...); err != nil {
			return err
		}
		for i, l := 0, len(vs); i < l; i++ {
			if err := assign(columns[i], valueToString(vs[i]), i == l-1); err != nil {
				return err
			}
		}
	}

	return rows.Err()
}

func (c *Collector) openConnection() error {
	db, err := sql.Open("oracle", c.DSN)
	if err != nil {
		return fmt.Errorf("error on sql open: %v", err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging: %v", err)
	}

	c.db = db

	return nil
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
