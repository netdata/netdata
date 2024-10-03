// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const precision = 1000

func (o *OracleDB) collect() (map[string]int64, error) {
	if o.db == nil {
		if err := o.openConnection(); err != nil {
			return nil, fmt.Errorf("failed to open connection: %v", err)
		}
	}

	mx := make(map[string]int64)

	// TODO: https://www.oracle.com/technical-resources/articles/schumacher-analysis.html

	if err := o.collectSysMetrics(mx); err != nil {
		return nil, fmt.Errorf("failed to collect system metrics: %v", err)
	}
	if err := o.collectSysStat(mx); err != nil {
		return nil, fmt.Errorf("failed to collect activities: %v", err)
	}
	if err := o.collectWaitClass(mx); err != nil {
		return nil, fmt.Errorf("failed to collect wait time: %v", err)
	}
	if err := o.collectTablespace(mx); err != nil {
		return nil, fmt.Errorf("failed to collect tablespace: %v", err)
	}

	return mx, nil
}

func (o *OracleDB) doQuery(query string, assign func(column, value string, lineEnd bool) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), o.Timeout.Duration())
	defer cancel()

	rows, err := o.db.QueryContext(ctx, query)
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

func (o *OracleDB) openConnection() error {
	db, err := sql.Open("oracle", o.DSN)
	if err != nil {
		return fmt.Errorf("error on sql open: %v", err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), o.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging: %v", err)
	}

	o.db = db

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
